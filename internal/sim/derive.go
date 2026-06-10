package sim

import (
	"math"

	"github.com/mynameis-nigel/ssh-idlefarmer/internal/content"
)

// GrowSeconds returns the crop's effective grow time for this save, after
// permanent grow-speed upgrades. Integer math; never below 1 second.
func (s *State) GrowSeconds(c *content.Content, crop *content.Crop) int64 {
	reduction := int64(0)
	for _, u := range c.Upgrades {
		if u.Effect == "grow_speed_pct" {
			reduction += u.EffectValue * int64(s.UpgradeLevel(u.ID))
		}
	}
	if reduction >= 100 {
		reduction = 99 // content validation prevents this, but stay safe
	}
	g := crop.GrowSeconds * (100 - reduction) / 100
	if g < 1 {
		g = 1
	}
	return g
}

// sellMultiplied applies permanent sell-bonus upgrades to a base payout.
func (s *State) sellMultiplied(c *content.Content, base int64) int64 {
	bonus := int64(0)
	for _, u := range c.Upgrades {
		if u.Effect == "sell_bonus_pct" {
			bonus += u.EffectValue * int64(s.UpgradeLevel(u.ID))
		}
	}
	if bonus == 0 {
		return base
	}
	if base > math.MaxInt64/(100+bonus) {
		return math.MaxInt64
	}
	return base * (100 + bonus) / 100
}

// startCoins returns the wallet a new run begins with, including the
// permanent start-coins upgrades.
func startCoins(c *content.Content, upgrades map[string]int) int64 {
	coins := c.Start.Coins
	for _, u := range c.Upgrades {
		if u.Effect == "start_coins" {
			coins = satAdd(coins, u.EffectValue*int64(upgrades[u.ID]))
		}
	}
	return coins
}

// NextPlotCost returns the price of the next plot, or -1 when the farm is at
// the purchasable cap. Geometric growth with integer math, discounted by
// plot-discount upgrades.
func (s *State) NextPlotCost(c *content.Content) int64 {
	if c.Start.Plots+s.PurchasedPlots >= c.Land.MaxPlots {
		return -1
	}
	cost := c.Land.BasePlotCost
	for i := 0; i < s.PurchasedPlots; i++ {
		if cost > math.MaxInt64/c.Land.GrowthPct {
			cost = math.MaxInt64 / 100
			break
		}
		cost = cost * c.Land.GrowthPct / 100
	}
	discount := int64(0)
	for _, u := range c.Upgrades {
		if u.Effect == "plot_discount_pct" {
			discount += u.EffectValue * int64(s.UpgradeLevel(u.ID))
		}
	}
	if discount >= 100 {
		discount = 99
	}
	cost = cost * (100 - discount) / 100
	if cost < 1 {
		cost = 1
	}
	return cost
}

// UpgradeCost returns the prestige price of the next level of an upgrade, or
// -1 when it is already at max level.
func (s *State) UpgradeCost(u *content.Upgrade) int64 {
	level := s.UpgradeLevel(u.ID)
	if level >= u.MaxLevel {
		return -1
	}
	cost := u.Cost
	for i := 0; i < level; i++ {
		if cost > math.MaxInt64/u.CostGrowthPct {
			return math.MaxInt64 / 100
		}
		cost = cost * u.CostGrowthPct / 100
	}
	return cost
}

// PrestigeGain returns the prestige currency a rebirth would award right now:
// isqrt(run earnings / divisor).
func (s *State) PrestigeGain(c *content.Content) int64 {
	return isqrt(s.RunEarnings / c.Prestige.Divisor)
}

// CanRebirth reports whether the run has earned enough to rebirth.
func (s *State) CanRebirth(c *content.Content) bool {
	return s.RunEarnings >= c.Prestige.MinEarnings && s.PrestigeGain(c) >= 1
}

// isqrt is the integer square root (floor) for non-negative n. The
// correction loops compare via division so they cannot overflow even for n
// near MaxInt64 (saturated lifetime earnings).
func isqrt(n int64) int64 {
	if n <= 0 {
		return 0
	}
	r := int64(math.Sqrt(float64(n)))
	for r > 0 && r > n/r {
		r--
	}
	for r+1 <= n/(r+1) {
		r++
	}
	return r
}

// Unlocked reports whether an unlock gate is satisfied for this save.
// "earnings" and "prestige" gates are permanent (lifetime values), so crops a
// player has unlocked stay unlocked across rebirths; "zone" gates depend on
// the run's owned zones.
func (s *State) Unlocked(u content.Unlock) bool {
	switch u.Kind {
	case "", "start":
		return true
	case "earnings":
		return s.LifetimeEarnings >= u.Value
	case "prestige":
		return s.Rebirths >= u.Value
	case "zone":
		return s.Zones[u.Zone]
	default:
		return false
	}
}

// PlotReady reports whether plot i is planted and mature at now.
func (s *State) PlotReady(c *content.Content, i int, now int64) bool {
	if i < 0 || i >= len(s.Plots) || s.Plots[i].Crop == "" {
		return false
	}
	crop := c.Crop(s.Plots[i].Crop)
	if crop == nil {
		return false
	}
	return now >= s.Plots[i].PlantedAt+s.GrowSeconds(c, crop)
}

// PlotProgressPct returns growth progress 0–100 for a planted plot.
func (s *State) PlotProgressPct(c *content.Content, i int, now int64) int {
	if i < 0 || i >= len(s.Plots) || s.Plots[i].Crop == "" {
		return 0
	}
	crop := c.Crop(s.Plots[i].Crop)
	if crop == nil {
		return 0
	}
	grow := s.GrowSeconds(c, crop)
	elapsed := now - s.Plots[i].PlantedAt
	if elapsed <= 0 {
		return 0
	}
	if elapsed >= grow {
		return 100
	}
	return int(elapsed * 100 / grow)
}
