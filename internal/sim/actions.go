package sim

import (
	"errors"
	"strings"
	"unicode/utf8"

	"github.com/mynameis-nigel/ssh-idlefarmer/internal/content"
)

// Sentinel errors let the UI explain exactly why an intent was refused.
var (
	ErrUnknownPlot    = errors.New("no such plot")
	ErrPlotOccupied   = errors.New("that plot is already planted")
	ErrPlotEmpty      = errors.New("nothing is planted there")
	ErrNotMature      = errors.New("that crop is still growing")
	ErrUnknownCrop    = errors.New("no such crop")
	ErrUnknownItem    = errors.New("no such item")
	ErrLocked         = errors.New("not unlocked yet")
	ErrCantAfford     = errors.New("not enough coins")
	ErrCantAffordPP   = errors.New("not enough Starseeds")
	ErrMaxed          = errors.New("already at maximum")
	ErrAlreadyOwned   = errors.New("already owned")
	ErrRebirthTooSoon = errors.New("this run has not earned enough to rebirth")
	ErrNoGift         = errors.New("no parcel is waiting")
	ErrNoCritter      = errors.New("no critter to shoo")
	ErrNameTooLong    = errors.New("farm name is too long")
	ErrNameEmpty      = errors.New("farm name cannot be empty")
)

// Plant spends the seed cost and plants cropID on empty plot i at time now.
// Mercy planting is free when broke with all plots empty.
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
	cost := s.SeedCost(c, crop)
	mercy := s.MercyPlantEligible(c, cropID)
	if !mercy && s.Coins < cost {
		return ErrCantAfford
	}
	if !mercy {
		s.Coins -= cost
	}
	s.Plots[i] = Plot{Crop: cropID, PlantedAt: now, AutoHarvest: s.Plots[i].AutoHarvest, AutoSow: s.Plots[i].AutoSow}
	s.Plots[i].Critter = ""
	return nil
}

// HarvestResult reports what one manual harvest yielded.
type HarvestResult struct {
	CropID    string
	Payout    int64
	Discovery int64
	Failed    bool
	Golden    bool
}

// Harvest gathers mature plot i at time now, crediting the payout.
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

	payout, discovery, failed, golden := s.harvestPayout(c, crop)
	s.credit(payout)
	if discovery > 0 {
		s.credit(discovery)
	}
	s.LifetimeHarvests = satAdd(s.LifetimeHarvests, 1)
	autoH, autoS := plot.AutoHarvest, plot.AutoSow
	s.Plots[i] = Plot{AutoHarvest: autoH, AutoSow: autoS}
	return HarvestResult{CropID: crop.ID, Payout: payout, Discovery: discovery, Failed: failed, Golden: golden}, nil
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

// BuyMultiplier spends coins on the next level of a run-scoped multiplier.
func BuyMultiplier(s *State, c *content.Content, id string) error {
	m := c.MultiplierByID(id)
	if m == nil {
		return ErrUnknownItem
	}
	cost := s.MultiplierCost(m)
	if cost < 0 {
		return ErrMaxed
	}
	if s.Coins < cost {
		return ErrCantAfford
	}
	s.Coins -= cost
	s.Multipliers[id]++
	return nil
}

// BuySeedUpgrade spends coins on the next Hardier Strain level for a crop.
func BuySeedUpgrade(s *State, c *content.Content, id string) error {
	su := c.SeedUpgradeByID(id)
	if su == nil {
		return ErrUnknownItem
	}
	crop := c.Crop(su.CropID)
	if crop == nil || !s.Unlocked(crop.Unlock) {
		return ErrLocked
	}
	cost := s.SeedUpgradeCost(su)
	if cost < 0 {
		return ErrMaxed
	}
	if s.Coins < cost {
		return ErrCantAfford
	}
	s.Coins -= cost
	s.SeedUpgrades[su.CropID]++
	return nil
}

// UpgradePlotAuto purchases auto-harvest or auto-sow for a plot.
func UpgradePlotAuto(s *State, c *content.Content, plotIdx int, kind string) error {
	if plotIdx < 0 || plotIdx >= len(s.Plots) {
		return ErrUnknownPlot
	}
	plot := &s.Plots[plotIdx]
	switch kind {
	case "harvest":
		if plot.AutoHarvest {
			return ErrAlreadyOwned
		}
		cost := s.PlotAutoHarvestCost(c)
		if s.Coins < cost {
			return ErrCantAfford
		}
		s.Coins -= cost
		plot.AutoHarvest = true
	case "sow":
		if plot.AutoSow {
			return ErrAlreadyOwned
		}
		if !plot.AutoHarvest {
			return ErrLocked
		}
		if s.LifetimeEarnings < c.PlotAutomation.AutoSowMinEarnings {
			return ErrLocked
		}
		cost := c.PlotAutomation.AutoSowCost
		if s.Coins < cost {
			return ErrCantAfford
		}
		s.Coins -= cost
		plot.AutoSow = true
	default:
		return ErrUnknownItem
	}
	return nil
}

// BuyUpgrade spends Starseeds on the next level of a permanent bonus.
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

// GiftResult reports what a redeemed parcel contained.
type GiftResult struct {
	Coins      int64
	Starseeds  int64
}

// RedeemGift opens the pending parcel.
func RedeemGift(s *State, c *content.Content) (GiftResult, error) {
	if !s.GiftPending {
		return GiftResult{}, ErrNoGift
	}
	s.GiftPending = false
	s.GiftArrivedAt = 0

	var res GiftResult
	if s.Rebirths >= 1 && s.roll100() < c.Gifts.StarseedChancePct {
		base := isqrt(s.RunEarnings/c.Prestige.Divisor) + 1
		if base < 1 {
			base = 1
		}
		res.Starseeds = base + s.Rebirths
		s.PrestigeCurrency = satAdd(s.PrestigeCurrency, res.Starseeds)
	} else {
		res.Coins = giftCoinReward(s, c)
		s.credit(res.Coins)
	}
	return res, nil
}

func giftCoinReward(s *State, c *content.Content) int64 {
	base := isqrt(s.RunEarnings/10) + 10
	base += s.Rebirths * 50
	if base < c.Gifts.CoinRewardFloor {
		base = c.Gifts.CoinRewardFloor
	}
	if base > c.Gifts.CoinRewardCeiling {
		base = c.Gifts.CoinRewardCeiling
	}
	jitter := s.rollRange(-base/5, base/5)
	reward := base + jitter
	if reward < c.Gifts.CoinRewardFloor {
		reward = c.Gifts.CoinRewardFloor
	}
	return reward
}

// ShooCritter removes a cosmetic critter and awards a few coins.
func ShooCritter(s *State, c *content.Content, plotIdx int) (int64, error) {
	if plotIdx < 0 || plotIdx >= len(s.Plots) {
		return 0, ErrUnknownPlot
	}
	plot := &s.Plots[plotIdx]
	if plot.Critter == "" {
		return 0, ErrNoCritter
	}
	plot.Critter = ""
	reward := s.rollRange(c.Critters.ShooRewardMin, c.Critters.ShooRewardMax)
	s.credit(reward)
	return reward, nil
}

// SetFarmName sets the player's farm name (max 24 runes).
func SetFarmName(s *State, name string) error {
	name = strings.TrimSpace(name)
	if name == "" {
		return ErrNameEmpty
	}
	if utf8.RuneCountInString(name) > 24 {
		return ErrNameTooLong
	}
	s.FarmName = name
	return nil
}

// Rebirth resets the current run in exchange for Starseeds.
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
	s.Multipliers = map[string]int{}
	s.RunEarnings = 0
	s.EventID = ""
	s.EventEndsAt = 0
	s.UpdatedAt = now
	return gain, nil
}

// SetFlavor toggles the ambient-discovery flavor for this save.
func SetFlavor(s *State, enabled bool) { s.FlavorEnabled = enabled }
