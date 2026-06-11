package sim

import (
	"github.com/mynameis-nigel/ssh-idlefarmer/internal/content"
)

// autoCycleCap bounds per-plot harvest/replant cycles simulated individually
// during one Advance.
const autoCycleCap = 50_000

// onlineTickThreshold: advances at or below this elapsed are "live" ticks.
const onlineTickThreshold = 2

// Events describes what happened during an Advance.
type Events struct {
	Elapsed         int64
	Matured         map[string]int
	AutoHarvested   map[string]int
	AutoCoins       int64
	Discoveries     int
	DiscoveryCoins  int64
	Achievements    []string
	FailedHarvests  map[string]int
	GoldenHarvests  int
	GiftArrived     bool
	EventStarted    string
	EventEnded      string
	CritterVisits   []string
	AwayVignettes   []string
}

// Empty reports whether nothing noteworthy happened.
func (e *Events) Empty() bool {
	return len(e.Matured) == 0 && len(e.AutoHarvested) == 0 &&
		e.Discoveries == 0 && len(e.Achievements) == 0 &&
		len(e.FailedHarvests) == 0 && e.GoldenHarvests == 0 &&
		!e.GiftArrived && e.EventStarted == "" && e.EventEnded == "" &&
		len(e.CritterVisits) == 0
}

// Advance simulates the state from its UpdatedAt to the given timestamp.
func Advance(s *State, c *content.Content, to int64) Events {
	ev := Events{
		Matured:        map[string]int{},
		AutoHarvested:  map[string]int{},
		FailedHarvests: map[string]int{},
	}
	if to <= s.UpdatedAt {
		return ev
	}
	from := s.UpdatedAt
	elapsed := to - from
	ev.Elapsed = elapsed

	online := elapsed <= onlineTickThreshold

	// Expire finished events.
	if s.EventID != "" && to >= s.EventEndsAt {
		ev.EventEnded = s.EventID
		s.EventID = ""
		s.EventEndsAt = 0
	}

	// Roll gifts and events before plot simulation.
	if online {
		s.rollOnlineEvent(c, to, &ev)
	}
	s.rollGiftArrival(c, elapsed, online, to, &ev)

	for i := range s.Plots {
		plot := &s.Plots[i]
		if plot.Crop == "" {
			if online {
				s.maybeCritterVisit(c, plot, &ev)
			}
			continue
		}
		crop := c.Crop(plot.Crop)
		if crop == nil {
			continue
		}
		grow := s.GrowSeconds(c, crop)
		matureAt := plot.PlantedAt + grow

		if !plot.AutoHarvest {
			if matureAt > from && matureAt <= to {
				ev.Matured[crop.ID]++
			}
			continue
		}

		cycles := 0
		for matureAt <= to {
			if cycles >= autoCycleCap && plot.AutoSow && s.Coins >= s.SeedCost(c, crop) {
				gross := expectedPayout(s, c, crop)
				seedCost := s.SeedCost(c, crop)
				if gross > seedCost {
					remaining := (to - matureAt) / grow
					n := remaining + 1
					totalGross := satMul(gross, n)
					totalSeeds := satMul(seedCost, n)
					s.credit(totalGross)
					if totalSeeds > s.Coins {
						totalSeeds = s.Coins
					}
					s.Coins -= totalSeeds
					ev.AutoCoins = satAdd(ev.AutoCoins, totalGross-totalSeeds)
					ev.AutoHarvested[crop.ID] += int(n)
					s.LifetimeHarvests = satAdd(s.LifetimeHarvests, n)
					plot.PlantedAt = matureAt + remaining*grow
					matureAt = plot.PlantedAt + grow
					continue
				}
			}
			payout, discovery, failed, golden := s.harvestPayout(c, crop)
			s.credit(payout)
			ev.AutoCoins += payout
			if failed {
				ev.FailedHarvests[crop.ID]++
			}
			if golden {
				ev.GoldenHarvests++
			}
			if discovery > 0 {
				s.credit(discovery)
				ev.Discoveries++
				ev.DiscoveryCoins += discovery
			}
			s.LifetimeHarvests = satAdd(s.LifetimeHarvests, 1)
			ev.AutoHarvested[crop.ID]++

			if plot.AutoSow && s.Coins >= s.SeedCost(c, crop) && s.Unlocked(crop.Unlock) {
				s.Coins -= s.SeedCost(c, crop)
				ev.AutoCoins -= s.SeedCost(c, crop)
				plot.PlantedAt = matureAt
				matureAt += grow
				cycles++
				continue
			}
			plot.Crop = ""
			plot.PlantedAt = 0
			break
		}
	}

	s.UpdatedAt = to
	ev.Achievements = s.CheckAchievements(c, to)

	if !online && elapsed > 60 {
		ev.AwayVignettes = awayVignettes(s, c, &ev)
	}
	return ev
}

func awayVignettes(s *State, c *content.Content, ev *Events) []string {
	var lines []string
	if totalCount(ev.Matured) > 0 {
		lines = append(lines, "The fields kept growing while you were gone.")
	}
	if totalCount(ev.AutoHarvested) > 0 {
		lines = append(lines, "Your automated plots worked without rest.")
	}
	for id, n := range ev.FailedHarvests {
		name := id
		if crop := c.Crop(id); crop != nil {
			name = crop.Name
		}
		if n == 1 {
			lines = append(lines, "A "+name+" crop failed in the night.")
		} else {
			lines = append(lines, "Several "+name+" crops failed in the night.")
		}
	}
	if ev.GiftArrived {
		lines = append(lines, "A parcel arrived at the gate while you were away.")
	}
	if len(lines) == 0 {
		vignettes := []string{
			"A fox watched your turnips. It did not help.",
			"The scarecrow held its post admirably.",
			"Dew settled on empty furrows like silver coins.",
			"A distant owl hooted approval at your patience.",
		}
		idx := int(s.UpdatedAt % int64(len(vignettes)))
		lines = append(lines, vignettes[idx])
	}
	return lines
}

func totalCount(m map[string]int) int {
	t := 0
	for _, n := range m {
		t += n
	}
	return t
}

func (s *State) rollGiftArrival(c *content.Content, elapsed int64, online bool, to int64, ev *Events) {
	if s.GiftPending {
		return
	}
	interval := c.Gifts.OfflineIntervalSec
	if online {
		interval = c.Gifts.OnlineIntervalSec
		bonus := s.upgradeEffectSum(c, "gift_rate_pct")
		if bonus > 0 {
			interval = interval * 100 / (100 + bonus)
		}
	}
	if interval < 1 {
		return
	}
	chance := elapsed * 100 / interval
	if chance > 100 {
		chance = 100
	}
	if chance > 0 && s.roll100() < chance {
		s.GiftPending = true
		s.GiftArrivedAt = to
		ev.GiftArrived = true
	}
}

func (s *State) rollOnlineEvent(c *content.Content, to int64, ev *Events) {
	if len(c.Events) == 0 {
		return
	}
	if s.EventActive(to) {
		return
	}
	ec := c.EventsConfig
	minI, maxI := ec.MinIntervalSec, ec.MaxIntervalSec
	bonus := s.upgradeEffectSum(c, "event_rate_pct")
	if bonus > 0 {
		minI = minI * 100 / (100 + bonus)
		maxI = maxI * 100 / (100 + bonus)
	}
	if minI < 1 {
		minI = 1
	}
	if maxI < minI {
		maxI = minI
	}
	span := maxI - minI
	interval := minI
	if span > 0 {
		interval += s.rollRange(0, span)
	}
	// Per-second chance for a ~interval-second mean wait.
	chance := int64(100) / interval
	if chance < 1 {
		chance = 1
	}
	if s.roll100() >= chance {
		return
	}
	idx := int(s.rollRange(0, int64(len(c.Events)-1)))
	evDef := c.Events[idx]
	s.EventID = evDef.ID
	durMin, durMax := ec.MinDurationSec, ec.MaxDurationSec
	durBonus := s.upgradeEffectSum(c, "event_duration_pct") + int64(s.UpgradeLevel("stargazer"))*10
	if durBonus > 0 {
		durMin = durMin * (100 + durBonus) / 100
		durMax = durMax * (100 + durBonus) / 100
	}
	dur := durMin
	if durMax > durMin {
		dur += s.rollRange(0, durMax-durMin)
	}
	s.EventEndsAt = to + dur
	ev.EventStarted = evDef.ID
}

func (s *State) maybeCritterVisit(c *content.Content, plot *Plot, ev *Events) {
	if plot.Critter != "" || len(c.Critters.Kinds) == 0 {
		return
	}
	if c.Critters.VisitChancePct <= 0 {
		return
	}
	if s.roll100() >= c.Critters.VisitChancePct {
		return
	}
	idx := int(s.rollRange(0, int64(len(c.Critters.Kinds)-1)))
	plot.Critter = c.Critters.Kinds[idx]
	ev.CritterVisits = append(ev.CritterVisits, plot.Critter)
}

// harvestPayout rolls one harvest of crop.
func (s *State) harvestPayout(c *content.Content, crop *content.Crop) (payout, discovery int64, failed, golden bool) {
	base := crop.SellValue
	if crop.Archetype == "risky" {
		if s.roll100() < crop.FailChancePct {
			base = crop.SellValue * SalvageNumerator(s.SeedUpgradeLevel(crop.ID)) / 8
			failed = true
		}
	}
	payout = s.sellMultiplied(c, base, crop.ID)

	if c.GoldenHarvest.ChancePct > 0 && s.roll100() < c.GoldenHarvest.ChancePct {
		mult := c.GoldenHarvest.Multiplier
		if mult < 10 {
			mult = 10
		}
		payout = payout * mult / 10
		golden = true
	}

	if c.Flavor.Enabled && s.FlavorEnabled && c.Flavor.DiscoveryChancePct > 0 {
		if s.roll100() < c.Flavor.DiscoveryChancePct {
			discovery = s.rollRange(c.Flavor.DiscoveryMin, c.Flavor.DiscoveryMax)
		}
	}
	return payout, discovery, failed, golden
}

// expectedPayout is the integer expected value of one harvest for batched catch-up.
func expectedPayout(s *State, c *content.Content, crop *content.Crop) int64 {
	if crop.Archetype != "risky" {
		return s.sellMultiplied(c, crop.SellValue, crop.ID)
	}
	failPct := crop.FailChancePct
	salvage := crop.SellValue * SalvageNumerator(s.SeedUpgradeLevel(crop.ID)) / 8
	successPct := 100 - failPct
	base := (failPct*salvage + successPct*crop.SellValue) / 100
	return s.sellMultiplied(c, base, crop.ID)
}

// CheckAchievements records any newly satisfied achievement conditions.
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
