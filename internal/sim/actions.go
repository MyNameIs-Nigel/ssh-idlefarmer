package sim

import (
	"errors"

	"github.com/mynameis-nigel/ssh-idlefarmer/internal/content"
)

// Sentinel errors let the UI explain exactly why an intent was refused. The
// engine is the sole authority on legality; the UI must never pre-compute an
// outcome it can't get from here.
var (
	ErrUnknownPlot   = errors.New("no such plot")
	ErrPlotOccupied  = errors.New("that plot is already planted")
	ErrPlotEmpty     = errors.New("nothing is planted there")
	ErrNotMature     = errors.New("that crop is still growing")
	ErrUnknownCrop   = errors.New("no such crop")
	ErrUnknownItem   = errors.New("no such item")
	ErrLocked        = errors.New("not unlocked yet")
	ErrCantAfford    = errors.New("not enough coins")
	ErrCantAffordPP  = errors.New("not enough prestige")
	ErrMaxed         = errors.New("already at maximum")
	ErrAlreadyOwned  = errors.New("already owned")
	ErrRebirthTooSoon = errors.New("this run has not earned enough to rebirth")
)

// Plant spends the seed cost and plants cropID on empty plot i at time now.
func Plant(s *State, c *content.Content, i int, cropID string, now int64) error {
	if i < 0 || i >= len(s.Plots) {
		return ErrUnknownPlot
	}
	if s.Plots[i].Crop != "" {
		return ErrPlotOccupied
	}
	crop := c.Crop(cropID)
	if crop == nil {
		return ErrUnknownCrop
	}
	if !s.Unlocked(crop.Unlock) {
		return ErrLocked
	}
	if s.Coins < crop.SeedCost {
		return ErrCantAfford
	}
	s.Coins -= crop.SeedCost
	s.Plots[i] = Plot{Crop: cropID, PlantedAt: now}
	return nil
}

// HarvestResult reports what one manual harvest yielded.
type HarvestResult struct {
	CropID    string
	Payout    int64
	Discovery int64 // bonus coins from a lucky find, 0 if none
}

// Harvest gathers mature plot i at time now, crediting the payout.
// Harvesting an immature plot is rejected outright (no partial yield).
func Harvest(s *State, c *content.Content, i int, now int64) (HarvestResult, error) {
	if i < 0 || i >= len(s.Plots) {
		return HarvestResult{}, ErrUnknownPlot
	}
	plot := s.Plots[i]
	if plot.Crop == "" {
		return HarvestResult{}, ErrPlotEmpty
	}
	crop := c.Crop(plot.Crop)
	if crop == nil {
		return HarvestResult{}, ErrUnknownCrop
	}
	if now < plot.PlantedAt+s.GrowSeconds(c, crop) {
		return HarvestResult{}, ErrNotMature
	}

	payout, discovery := s.harvestPayout(c, crop)
	s.credit(payout)
	if discovery > 0 {
		s.credit(discovery)
	}
	s.LifetimeHarvests = satAdd(s.LifetimeHarvests, 1)
	s.Plots[i] = Plot{}
	return HarvestResult{CropID: crop.ID, Payout: payout, Discovery: discovery}, nil
}

// BuyPlot purchases the next plot at the scaling price.
func BuyPlot(s *State, c *content.Content) (int64, error) {
	cost := s.NextPlotCost(c)
	if cost < 0 {
		return 0, ErrMaxed
	}
	if s.Coins < cost {
		return 0, ErrCantAfford
	}
	s.Coins -= cost
	s.PurchasedPlots++
	s.Plots = append(s.Plots, Plot{})
	return cost, nil
}

// BuyTool purchases a run-scoped automation tool.
func BuyTool(s *State, c *content.Content, id string) error {
	tool := c.ToolByID(id)
	if tool == nil {
		return ErrUnknownItem
	}
	if s.Tools[id] {
		return ErrAlreadyOwned
	}
	if !s.Unlocked(tool.Unlock) {
		return ErrLocked
	}
	if s.Coins < tool.Cost {
		return ErrCantAfford
	}
	s.Coins -= tool.Cost
	s.Tools[id] = true
	return nil
}

// BuyZone purchases a run-scoped zone, adding its plots immediately.
func BuyZone(s *State, c *content.Content, id string) error {
	zone := c.ZoneByID(id)
	if zone == nil {
		return ErrUnknownItem
	}
	if s.Zones[id] {
		return ErrAlreadyOwned
	}
	if !s.Unlocked(zone.Unlock) {
		return ErrLocked
	}
	if s.Coins < zone.Cost {
		return ErrCantAfford
	}
	s.Coins -= zone.Cost
	s.Zones[id] = true
	for i := 0; i < zone.ExtraPlots; i++ {
		s.Plots = append(s.Plots, Plot{})
	}
	return nil
}

// BuyUpgrade spends prestige currency on the next level of a permanent bonus.
func BuyUpgrade(s *State, c *content.Content, id string) error {
	u := c.UpgradeByID(id)
	if u == nil {
		return ErrUnknownItem
	}
	cost := s.UpgradeCost(u)
	if cost < 0 {
		return ErrMaxed
	}
	if s.PrestigeCurrency < cost {
		return ErrCantAffordPP
	}
	s.PrestigeCurrency -= cost
	s.Upgrades[id]++
	return nil
}

// Rebirth resets the current run in exchange for prestige currency. The
// wallet, plots, crops, tools, zones, and run earnings are sacrificed;
// prestige currency, upgrades, achievements, and lifetime stats persist.
// It must only be called after an explicit, confirmed player choice.
func Rebirth(s *State, c *content.Content, now int64) (int64, error) {
	if !s.CanRebirth(c) {
		return 0, ErrRebirthTooSoon
	}
	gain := s.PrestigeGain(c)
	s.PrestigeCurrency = satAdd(s.PrestigeCurrency, gain)
	s.Rebirths++

	s.Coins = startCoins(c, s.Upgrades)
	s.Plots = make([]Plot, c.Start.Plots)
	s.PurchasedPlots = 0
	s.Tools = map[string]bool{}
	s.Zones = map[string]bool{}
	s.RunEarnings = 0
	s.UpdatedAt = now
	return gain, nil
}

// SetFlavor toggles the ambient-discovery flavor for this save.
func SetFlavor(s *State, enabled bool) { s.FlavorEnabled = enabled }
