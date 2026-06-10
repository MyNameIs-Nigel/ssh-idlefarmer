package sim

import (
	"github.com/mynameis-nigel/ssh-idlefarmer/internal/content"
)

// autoCycleCap bounds per-plot harvest/replant cycles simulated individually
// during one Advance. Beyond it (months of auto-farming in one gap) the
// remaining whole cycles are settled at expected value in a single batch so
// catch-up stays O(1) however long the player was away. The result is still
// deterministic — the same inputs always take the same path.
const autoCycleCap = 50_000

// Events describes what happened during an Advance, for the
// "while you were away" summary and live notifications.
type Events struct {
	Elapsed        int64            // seconds simulated
	Matured        map[string]int   // crop id -> plots that became ready (awaiting manual harvest)
	AutoHarvested  map[string]int   // crop id -> crops gathered by the auto-harvester
	AutoCoins      int64            // coins earned by auto-harvesting (net of replant seeds)
	Discoveries    int              // lucky finds
	DiscoveryCoins int64            // coins from lucky finds
	Achievements   []string         // achievement ids earned during this advance
}

// Empty reports whether nothing noteworthy happened.
func (e *Events) Empty() bool {
	return len(e.Matured) == 0 && len(e.AutoHarvested) == 0 &&
		e.Discoveries == 0 && len(e.Achievements) == 0
}

// Advance simulates the state from its UpdatedAt to the given timestamp.
// It is a pure function of (state, to): it never reads the clock, and all
// randomness comes from the state's seeded RNG. Offline catch-up and the
// live one-second tick both go through here. A `to` in the past is a no-op,
// so time can never be applied twice or run backwards.
func Advance(s *State, c *content.Content, to int64) Events {
	ev := Events{
		Matured:       map[string]int{},
		AutoHarvested: map[string]int{},
	}
	if to <= s.UpdatedAt {
		return ev
	}
	from := s.UpdatedAt
	ev.Elapsed = to - from

	autoHarvest := s.Tools["auto_harvester"]
	autoSow := s.Tools["auto_sower"]

	for i := range s.Plots {
		plot := &s.Plots[i]
		if plot.Crop == "" {
			continue
		}
		crop := c.Crop(plot.Crop)
		if crop == nil {
			// Crop removed from content; leave the plot untouched so a
			// restored content file resumes it.
			continue
		}
		grow := s.GrowSeconds(c, crop)
		matureAt := plot.PlantedAt + grow

		if !autoHarvest {
			if matureAt > from && matureAt <= to {
				ev.Matured[crop.ID]++
			}
			continue
		}

		// Auto-harvester: gather every cycle that completed in the window,
		// replanting after each gather when the auto-sower can afford it.
		cycles := 0
		for matureAt <= to {
			if cycles >= autoCycleCap && autoSow && s.Coins >= crop.SeedCost {
				gross := expectedPayout(s, c, crop)
				if gross > crop.SeedCost {
					// Settle all remaining whole cycles at expected value,
					// with the same accounting as the per-cycle path: gross
					// payouts count as earnings, seeds come out of the wallet.
					remaining := (to - matureAt) / grow // full cycles after this one
					n := remaining + 1
					totalGross := satMul(gross, n)
					totalSeeds := satMul(crop.SeedCost, n)
					s.credit(totalGross)
					if totalSeeds > s.Coins {
						totalSeeds = s.Coins // only when totals saturated
					}
					s.Coins -= totalSeeds
					ev.AutoCoins = satAdd(ev.AutoCoins, totalGross-totalSeeds)
					ev.AutoHarvested[crop.ID] += int(n)
					s.LifetimeHarvests = satAdd(s.LifetimeHarvests, n)
					plot.PlantedAt = matureAt + remaining*grow
					matureAt = plot.PlantedAt + grow
					continue
				}
				// Loss-making crop: keep simulating per cycle so the wallet
				// drains and replanting stops exactly where it should.
			}
			payout, discovery := s.harvestPayout(c, crop)
			s.credit(payout)
			ev.AutoCoins += payout
			if discovery > 0 {
				s.credit(discovery)
				ev.Discoveries++
				ev.DiscoveryCoins += discovery
			}
			s.LifetimeHarvests = satAdd(s.LifetimeHarvests, 1)
			ev.AutoHarvested[crop.ID]++

			if autoSow && s.Coins >= crop.SeedCost && s.Unlocked(crop.Unlock) {
				s.Coins -= crop.SeedCost
				ev.AutoCoins -= crop.SeedCost
				plot.PlantedAt = matureAt
				matureAt += grow
				cycles++
				continue
			}
			*plot = Plot{}
			break
		}
	}

	s.UpdatedAt = to
	ev.Achievements = s.CheckAchievements(c, to)
	return ev
}

// harvestPayout rolls one harvest of crop: the risky-archetype variance roll
// first, then the flavor discovery roll. The roll order is fixed so a replay
// from the same RNG state reproduces identical results.
func (s *State) harvestPayout(c *content.Content, crop *content.Crop) (payout, discovery int64) {
	base := crop.SellValue
	if crop.Archetype == "risky" {
		r := s.roll100()
		switch {
		case r < crop.FailChancePct:
			base = crop.FailValue
		case r < crop.FailChancePct+crop.BonusChancePct:
			base = crop.BonusValue
		}
	}
	payout = s.sellMultiplied(c, base)
	if c.Flavor.Enabled && s.FlavorEnabled && c.Flavor.DiscoveryChancePct > 0 {
		if s.roll100() < c.Flavor.DiscoveryChancePct {
			discovery = s.rollRange(c.Flavor.DiscoveryMin, c.Flavor.DiscoveryMax)
		}
	}
	return payout, discovery
}

// expectedPayout is the integer expected value of one harvest, used only for
// the batched tail of very long auto-farm catch-ups (no discovery rolls).
func expectedPayout(s *State, c *content.Content, crop *content.Crop) int64 {
	base := crop.SellValue
	if crop.Archetype == "risky" {
		rest := 100 - crop.FailChancePct - crop.BonusChancePct
		base = (crop.FailChancePct*crop.FailValue +
			crop.BonusChancePct*crop.BonusValue +
			rest*crop.SellValue) / 100
	}
	return s.sellMultiplied(c, base)
}

// CheckAchievements records any newly satisfied achievement conditions at
// time now and returns their ids. Achievements are positive-only and never
// un-earn.
func (s *State) CheckAchievements(c *content.Content, now int64) []string {
	var newly []string
	for _, a := range c.Achievements {
		if _, earned := s.Achievements[a.ID]; earned {
			continue
		}
		var met bool
		switch a.Condition.Kind {
		case "harvests":
			met = s.LifetimeHarvests >= a.Condition.Value
		case "earnings":
			met = s.LifetimeEarnings >= a.Condition.Value
		case "plots":
			met = int64(len(s.Plots)) >= a.Condition.Value
		case "rebirths":
			met = s.Rebirths >= a.Condition.Value
		case "balance":
			met = s.Coins >= a.Condition.Value
		}
		if met {
			s.Achievements[a.ID] = now
			newly = append(newly, a.ID)
		}
	}
	return newly
}
