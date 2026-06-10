// Package content loads and validates the data-driven game definitions
// (crops, balance, upgrades, tools, zones, achievements, flavor). Content is
// configuration, not code: it is validated strictly on load so the server
// fails fast on a bad file instead of running with a broken economy.
package content

import (
	"fmt"
	"io/fs"
	"os"

	"github.com/BurntSushi/toml"

	"github.com/mynameis-nigel/ssh-idlefarmer/data"
)

// Unlock gates a crop, tool, or zone behind a progression condition.
type Unlock struct {
	Kind  string `toml:"kind"`  // "start", "earnings", "prestige", "zone"
	Value int64  `toml:"value"` // threshold for earnings/prestige kinds
	Zone  string `toml:"zone"`  // zone id for the zone kind
}

// Crop is one plantable crop definition.
type Crop struct {
	ID             string `toml:"id"`
	Name           string `toml:"name"`
	Archetype      string `toml:"archetype"` // "fast", "slow", "risky"
	SeedCost       int64  `toml:"seed_cost"`
	GrowSeconds    int64  `toml:"grow_seconds"`
	SellValue      int64  `toml:"sell_value"`
	FailChancePct  int64  `toml:"fail_chance_pct"`
	FailValue      int64  `toml:"fail_value"`
	BonusChancePct int64  `toml:"bonus_chance_pct"`
	BonusValue     int64  `toml:"bonus_value"`
	Unlock         Unlock `toml:"unlock"`
}

// Upgrade is a permanent prestige-bought bonus.
type Upgrade struct {
	ID            string `toml:"id"`
	Name          string `toml:"name"`
	Description   string `toml:"description"`
	Cost          int64  `toml:"cost"`
	CostGrowthPct int64  `toml:"cost_growth_pct"`
	MaxLevel      int    `toml:"max_level"`
	Effect        string `toml:"effect"` // grow_speed_pct, sell_bonus_pct, start_coins, plot_discount_pct
	EffectValue   int64  `toml:"effect_value"`
}

// Tool is a run-scoped automation purchase.
type Tool struct {
	ID          string `toml:"id"`
	Name        string `toml:"name"`
	Description string `toml:"description"`
	Cost        int64  `toml:"cost"`
	Unlock      Unlock `toml:"unlock"`
}

// Zone is a run-scoped farm expansion.
type Zone struct {
	ID          string `toml:"id"`
	Name        string `toml:"name"`
	Description string `toml:"description"`
	Cost        int64  `toml:"cost"`
	ExtraPlots  int    `toml:"extra_plots"`
	Unlock      Unlock `toml:"unlock"`
}

// Condition triggers an achievement.
type Condition struct {
	Kind  string `toml:"kind"` // harvests, earnings, plots, rebirths, balance
	Value int64  `toml:"value"`
}

// Achievement is a positive-only milestone.
type Achievement struct {
	ID          string    `toml:"id"`
	Name        string    `toml:"name"`
	Description string    `toml:"description"`
	Condition   Condition `toml:"condition"`
}

// Flavor configures the optional ambient discoveries.
type Flavor struct {
	Enabled            bool  `toml:"enabled"`
	DiscoveryChancePct int64 `toml:"discovery_chance_pct"`
	DiscoveryMin       int64 `toml:"discovery_min"`
	DiscoveryMax       int64 `toml:"discovery_max"`
}

// Start configures a fresh run.
type Start struct {
	Coins int64 `toml:"coins"`
	Plots int   `toml:"plots"`
}

// Land configures plot expansion costs.
type Land struct {
	BasePlotCost int64 `toml:"base_plot_cost"`
	GrowthPct    int64 `toml:"growth_pct"`
	MaxPlots     int   `toml:"max_plots"`
}

// Prestige configures the rebirth formula.
type Prestige struct {
	Divisor     int64 `toml:"divisor"`
	MinEarnings int64 `toml:"min_earnings"`
}

type cropsFile struct {
	Crops []Crop `toml:"crop"`
}

type balanceFile struct {
	Start        Start         `toml:"start"`
	Land         Land          `toml:"land"`
	Prestige     Prestige      `toml:"prestige"`
	Upgrades     []Upgrade     `toml:"upgrade"`
	Tools        []Tool        `toml:"tool"`
	Zones        []Zone        `toml:"zone"`
	Achievements []Achievement `toml:"achievement"`
	Flavor       Flavor        `toml:"flavor"`
}

// Content is the validated, immutable game configuration. Lookup maps are
// built once at load; treat the whole struct as read-only after Load.
type Content struct {
	Crops        []Crop
	Upgrades     []Upgrade
	Tools        []Tool
	Zones        []Zone
	Achievements []Achievement
	Start        Start
	Land         Land
	Prestige     Prestige
	Flavor       Flavor

	cropByID    map[string]*Crop
	upgradeByID map[string]*Upgrade
	toolByID    map[string]*Tool
	zoneByID    map[string]*Zone
}

// Load reads content from overrideDir when set, otherwise from the embedded
// files, and validates everything.
func Load(overrideDir string) (*Content, error) {
	var fsys fs.FS = data.FS
	if overrideDir != "" {
		fsys = os.DirFS(overrideDir)
	}

	var cf cropsFile
	if err := decodeTOML(fsys, "crops.toml", &cf); err != nil {
		return nil, err
	}
	var bf balanceFile
	if err := decodeTOML(fsys, "balance.toml", &bf); err != nil {
		return nil, err
	}

	c := &Content{
		Crops:        cf.Crops,
		Upgrades:     bf.Upgrades,
		Tools:        bf.Tools,
		Zones:        bf.Zones,
		Achievements: bf.Achievements,
		Start:        bf.Start,
		Land:         bf.Land,
		Prestige:     bf.Prestige,
		Flavor:       bf.Flavor,
	}
	if err := c.validate(); err != nil {
		return nil, err
	}
	c.buildIndexes()
	return c, nil
}

func decodeTOML(fsys fs.FS, name string, v any) error {
	b, err := fs.ReadFile(fsys, name)
	if err != nil {
		return fmt.Errorf("content: read %s: %w", name, err)
	}
	if err := toml.Unmarshal(b, v); err != nil {
		return fmt.Errorf("content: parse %s: %w", name, err)
	}
	return nil
}

func (c *Content) buildIndexes() {
	c.cropByID = make(map[string]*Crop, len(c.Crops))
	for i := range c.Crops {
		c.cropByID[c.Crops[i].ID] = &c.Crops[i]
	}
	c.upgradeByID = make(map[string]*Upgrade, len(c.Upgrades))
	for i := range c.Upgrades {
		c.upgradeByID[c.Upgrades[i].ID] = &c.Upgrades[i]
	}
	c.toolByID = make(map[string]*Tool, len(c.Tools))
	for i := range c.Tools {
		c.toolByID[c.Tools[i].ID] = &c.Tools[i]
	}
	c.zoneByID = make(map[string]*Zone, len(c.Zones))
	for i := range c.Zones {
		c.zoneByID[c.Zones[i].ID] = &c.Zones[i]
	}
}

// Crop returns the crop definition for id, or nil.
func (c *Content) Crop(id string) *Crop { return c.cropByID[id] }

// UpgradeByID returns the upgrade definition for id, or nil.
func (c *Content) UpgradeByID(id string) *Upgrade { return c.upgradeByID[id] }

// ToolByID returns the tool definition for id, or nil.
func (c *Content) ToolByID(id string) *Tool { return c.toolByID[id] }

// ZoneByID returns the zone definition for id, or nil.
func (c *Content) ZoneByID(id string) *Zone { return c.zoneByID[id] }

func (c *Content) validate() error {
	if len(c.Crops) == 0 {
		return fmt.Errorf("content: no crops defined")
	}
	if c.Start.Coins < 0 || c.Start.Plots < 1 {
		return fmt.Errorf("content: start must have coins >= 0 and plots >= 1")
	}
	if c.Land.BasePlotCost < 1 || c.Land.GrowthPct < 100 {
		return fmt.Errorf("content: land base_plot_cost must be >= 1 and growth_pct >= 100")
	}
	if c.Land.MaxPlots < c.Start.Plots {
		return fmt.Errorf("content: land max_plots (%d) below start plots (%d)", c.Land.MaxPlots, c.Start.Plots)
	}
	if c.Prestige.Divisor < 1 {
		return fmt.Errorf("content: prestige divisor must be >= 1")
	}
	if c.Prestige.MinEarnings < 1 {
		return fmt.Errorf("content: prestige min_earnings must be >= 1")
	}

	zoneIDs := map[string]bool{}
	for i, z := range c.Zones {
		if err := requireID("zone", i, z.ID, z.Name); err != nil {
			return err
		}
		if zoneIDs[z.ID] {
			return fmt.Errorf("content: duplicate zone id %q", z.ID)
		}
		zoneIDs[z.ID] = true
	}
	for _, z := range c.Zones {
		if z.Unlock.Kind == "zone" && z.Unlock.Zone == z.ID {
			return fmt.Errorf("content: zone %q cannot be unlocked by itself", z.ID)
		}
		if z.Cost < 0 || z.ExtraPlots < 1 {
			return fmt.Errorf("content: zone %q needs cost >= 0 and extra_plots >= 1", z.ID)
		}
		if err := validateUnlock("zone", z.ID, z.Unlock, zoneIDs); err != nil {
			return err
		}
	}

	archetypes := map[string]bool{"fast": false, "slow": false, "risky": false}
	cropIDs := map[string]bool{}
	for i, cr := range c.Crops {
		if err := requireID("crop", i, cr.ID, cr.Name); err != nil {
			return err
		}
		if cropIDs[cr.ID] {
			return fmt.Errorf("content: duplicate crop id %q", cr.ID)
		}
		cropIDs[cr.ID] = true
		if _, ok := archetypes[cr.Archetype]; !ok {
			return fmt.Errorf("content: crop %q has unknown archetype %q", cr.ID, cr.Archetype)
		}
		archetypes[cr.Archetype] = true
		if cr.SeedCost < 0 || cr.SellValue < 0 || cr.GrowSeconds < 1 {
			return fmt.Errorf("content: crop %q needs seed_cost >= 0, sell_value >= 0, grow_seconds >= 1", cr.ID)
		}
		if cr.Archetype == "risky" {
			if cr.FailChancePct < 0 || cr.BonusChancePct < 0 ||
				cr.FailChancePct+cr.BonusChancePct > 100 {
				return fmt.Errorf("content: crop %q risky chances must be >= 0 and sum to <= 100", cr.ID)
			}
			if cr.FailValue < 0 || cr.BonusValue < 0 {
				return fmt.Errorf("content: crop %q risky values must be >= 0", cr.ID)
			}
		} else if cr.FailChancePct != 0 || cr.BonusChancePct != 0 {
			return fmt.Errorf("content: crop %q sets risk chances but is not risky", cr.ID)
		}
		if err := validateUnlock("crop", cr.ID, cr.Unlock, zoneIDs); err != nil {
			return err
		}
	}
	for a, present := range archetypes {
		if !present {
			return fmt.Errorf("content: no crop with archetype %q (all three are required)", a)
		}
	}

	upgradeIDs := map[string]bool{}
	validEffects := map[string]bool{
		"grow_speed_pct": true, "sell_bonus_pct": true,
		"start_coins": true, "plot_discount_pct": true,
	}
	for i, u := range c.Upgrades {
		if err := requireID("upgrade", i, u.ID, u.Name); err != nil {
			return err
		}
		if upgradeIDs[u.ID] {
			return fmt.Errorf("content: duplicate upgrade id %q", u.ID)
		}
		upgradeIDs[u.ID] = true
		if !validEffects[u.Effect] {
			return fmt.Errorf("content: upgrade %q has unknown effect %q", u.ID, u.Effect)
		}
		if u.Cost < 1 || u.CostGrowthPct < 100 || u.MaxLevel < 1 || u.EffectValue < 1 {
			return fmt.Errorf("content: upgrade %q needs cost >= 1, cost_growth_pct >= 100, max_level >= 1, effect_value >= 1", u.ID)
		}
		if u.Effect == "grow_speed_pct" || u.Effect == "plot_discount_pct" {
			if u.EffectValue*int64(u.MaxLevel) >= 100 {
				return fmt.Errorf("content: upgrade %q would reach a %d%% reduction at max level; must stay below 100%%", u.ID, u.EffectValue*int64(u.MaxLevel))
			}
		}
	}

	toolIDs := map[string]bool{}
	for i, tl := range c.Tools {
		if err := requireID("tool", i, tl.ID, tl.Name); err != nil {
			return err
		}
		if toolIDs[tl.ID] {
			return fmt.Errorf("content: duplicate tool id %q", tl.ID)
		}
		toolIDs[tl.ID] = true
		if tl.Cost < 0 {
			return fmt.Errorf("content: tool %q needs cost >= 0", tl.ID)
		}
		if err := validateUnlock("tool", tl.ID, tl.Unlock, zoneIDs); err != nil {
			return err
		}
	}

	achIDs := map[string]bool{}
	validConds := map[string]bool{
		"harvests": true, "earnings": true, "plots": true, "rebirths": true, "balance": true,
	}
	for i, a := range c.Achievements {
		if err := requireID("achievement", i, a.ID, a.Name); err != nil {
			return err
		}
		if achIDs[a.ID] {
			return fmt.Errorf("content: duplicate achievement id %q", a.ID)
		}
		achIDs[a.ID] = true
		if !validConds[a.Condition.Kind] {
			return fmt.Errorf("content: achievement %q has unknown condition kind %q", a.ID, a.Condition.Kind)
		}
		if a.Condition.Value < 1 {
			return fmt.Errorf("content: achievement %q condition value must be >= 1", a.ID)
		}
	}

	f := c.Flavor
	if f.Enabled {
		if f.DiscoveryChancePct < 0 || f.DiscoveryChancePct > 100 {
			return fmt.Errorf("content: flavor discovery_chance_pct must be 0-100")
		}
		if f.DiscoveryMin < 0 || f.DiscoveryMax < f.DiscoveryMin {
			return fmt.Errorf("content: flavor discovery range invalid (min %d, max %d)", f.DiscoveryMin, f.DiscoveryMax)
		}
	}

	return nil
}

func requireID(kind string, idx int, id, name string) error {
	if id == "" {
		return fmt.Errorf("content: %s #%d has no id", kind, idx+1)
	}
	if name == "" {
		return fmt.Errorf("content: %s %q has no name", kind, id)
	}
	return nil
}

func validateUnlock(kind, id string, u Unlock, zoneIDs map[string]bool) error {
	switch u.Kind {
	case "", "start":
		return nil
	case "earnings", "prestige":
		if u.Value < 1 {
			return fmt.Errorf("content: %s %q unlock value must be >= 1", kind, id)
		}
		return nil
	case "zone":
		if !zoneIDs[u.Zone] {
			return fmt.Errorf("content: %s %q unlock references unknown zone %q", kind, id, u.Zone)
		}
		return nil
	default:
		return fmt.Errorf("content: %s %q has unknown unlock kind %q", kind, id, u.Kind)
	}
}
