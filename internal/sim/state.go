// Package sim is the headless, deterministic game engine. Every state
// transition is a pure function of the state and the timestamps passed in;
// the engine never reads the wall clock and all randomness flows from the
// seeded RNG stored inside the save. Money is int64 coins everywhere.
package sim

import (
	"encoding/json"
	"fmt"
	"maps"
	"math"

	"github.com/mynameis-nigel/ssh-idlefarmer/internal/content"
)

// StateVersion is the current save-payload version. Older payloads are
// upgraded in place by DecodeState.
const StateVersion = 2

// Plot is one tile of farmland. Empty when Crop is "".
type Plot struct {
	Crop      string `json:"crop,omitempty"`
	PlantedAt int64  `json:"planted_at,omitempty"` // unix seconds
}

// State is the full authoritative save. It is JSON-serialized into the
// store's save row. Run-scoped fields reset on rebirth; lifetime and
// prestige fields persist across rebirths.
type State struct {
	Version int    `json:"version"`
	RNG     uint64 `json:"rng"` // seeded random stream; advances on every roll

	// UpdatedAt is the last timestamp the state was simulated to. Advance
	// uses it as the implicit "from" so time can never be replayed twice.
	UpdatedAt int64 `json:"updated_at"`

	// Run-scoped (reset on rebirth).
	Coins          int64           `json:"coins"`
	Plots          []Plot          `json:"plots"`
	PurchasedPlots int             `json:"purchased_plots"`
	Tools          map[string]bool `json:"tools"`
	Zones          map[string]bool `json:"zones"`
	RunEarnings    int64           `json:"run_earnings"`

	// Permanent (survive rebirth).
	PrestigeCurrency int64            `json:"prestige_currency"`
	Upgrades         map[string]int   `json:"upgrades"`     // upgrade id -> level
	Achievements     map[string]int64 `json:"achievements"` // id -> earned unix
	Rebirths         int64            `json:"rebirths"`
	LifetimeEarnings int64            `json:"lifetime_earnings"`
	LifetimeHarvests int64            `json:"lifetime_harvests"`
	FlavorEnabled    bool             `json:"flavor_enabled"`
}

// New creates a fresh save seeded with seed, simulated as of now.
func New(c *content.Content, seed uint64, now int64) *State {
	s := &State{
		Version:       StateVersion,
		RNG:           seed,
		UpdatedAt:     now,
		Coins:         c.Start.Coins,
		Plots:         make([]Plot, c.Start.Plots),
		Tools:         map[string]bool{},
		Zones:         map[string]bool{},
		Upgrades:      map[string]int{},
		Achievements:  map[string]int64{},
		FlavorEnabled: true,
	}
	return s
}

// Encode serializes the state for storage.
func (s *State) Encode() ([]byte, error) {
	return json.Marshal(s)
}

// DecodeState parses a stored payload and upgrades older payload versions to
// the current shape. It never discards data it does not understand the
// version of — unknown future versions are an error.
func DecodeState(b []byte) (*State, error) {
	var s State
	if err := json.Unmarshal(b, &s); err != nil {
		return nil, fmt.Errorf("sim: decode save payload: %w", err)
	}
	if s.Version > StateVersion {
		return nil, fmt.Errorf("sim: save payload version %d is newer than supported %d", s.Version, StateVersion)
	}
	upgradeState(&s)
	return &s, nil
}

// upgradeState migrates older payload versions forward. Version 1 was the
// pre-progression shape (no prestige, upgrades, achievements, or flavor);
// version 2 added them. The upgrade is additive and never destructive.
func upgradeState(s *State) {
	if s.Version < 2 {
		s.FlavorEnabled = true
		s.Version = 2
	}
	// Normalize nil maps regardless of version so the engine can assume
	// they exist (JSON omits empty maps depending on writer).
	if s.Tools == nil {
		s.Tools = map[string]bool{}
	}
	if s.Zones == nil {
		s.Zones = map[string]bool{}
	}
	if s.Upgrades == nil {
		s.Upgrades = map[string]int{}
	}
	if s.Achievements == nil {
		s.Achievements = map[string]int64{}
	}
}

// UpgradeLevel returns the owned level of a permanent upgrade.
func (s *State) UpgradeLevel(id string) int { return s.Upgrades[id] }

// Clone returns a deep copy, used to hand read-only snapshots to the UI
// without sharing the actor's authoritative state.
func (s *State) Clone() *State {
	c := *s
	c.Plots = append([]Plot(nil), s.Plots...)
	c.Tools = maps.Clone(s.Tools)
	c.Zones = maps.Clone(s.Zones)
	c.Upgrades = maps.Clone(s.Upgrades)
	c.Achievements = maps.Clone(s.Achievements)
	return &c
}

// satAdd adds two non-negative coin amounts, saturating at MaxInt64 so
// long-lived saves can never overflow into negative balances.
func satAdd(a, b int64) int64 {
	if a > math.MaxInt64-b {
		return math.MaxInt64
	}
	return a + b
}

// satMul multiplies two non-negative amounts, saturating at MaxInt64.
func satMul(a, b int64) int64 {
	if a == 0 || b == 0 {
		return 0
	}
	if a > math.MaxInt64/b {
		return math.MaxInt64
	}
	return a * b
}

// credit adds coins to the wallet and both earnings counters.
func (s *State) credit(amount int64) {
	if amount <= 0 {
		return
	}
	s.Coins = satAdd(s.Coins, amount)
	s.RunEarnings = satAdd(s.RunEarnings, amount)
	s.LifetimeEarnings = satAdd(s.LifetimeEarnings, amount)
}

// nextRand advances the save's RNG (splitmix64) and returns the next value.
// All engine randomness must come from here so replays are reproducible.
func (s *State) nextRand() uint64 {
	s.RNG += 0x9E3779B97F4A7C15
	z := s.RNG
	z = (z ^ (z >> 30)) * 0xBF58476D1CE4E5B9
	z = (z ^ (z >> 27)) * 0x94D049BB133111EB
	return z ^ (z >> 31)
}

// roll100 returns a uniform integer in [0, 100).
func (s *State) roll100() int64 { return int64(s.nextRand() % 100) }

// rollRange returns a uniform integer in [lo, hi].
func (s *State) rollRange(lo, hi int64) int64 {
	if hi <= lo {
		return lo
	}
	return lo + int64(s.nextRand()%uint64(hi-lo+1))
}
