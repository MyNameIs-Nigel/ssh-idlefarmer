package sim

import (
	"reflect"
	"testing"

	"github.com/mynameis-nigel/ssh-idlefarmer/internal/content"
)

// testContent loads the fixture under testdata/. No SSH, DB, or clock.
func testContent(t *testing.T) *content.Content {
	t.Helper()
	c, err := content.Load("testdata")
	if err != nil {
		t.Fatalf("load test content: %v", err)
	}
	return c
}

func newTestState(t *testing.T, c *content.Content) *State {
	t.Helper()
	return New(c, 42, 1000)
}

func TestPlantValidation(t *testing.T) {
	c := testContent(t)
	s := newTestState(t, c) // 25 coins, 3 plots

	if err := Plant(s, c, 0, "turnip", 1000); err != nil {
		t.Fatalf("plant: %v", err)
	}
	if s.Coins != 20 {
		t.Fatalf("coins = %d, want 20", s.Coins)
	}
	if err := Plant(s, c, 0, "turnip", 1000); err != ErrPlotOccupied {
		t.Fatalf("expected ErrPlotOccupied, got %v", err)
	}
	if err := Plant(s, c, 99, "turnip", 1000); err != ErrUnknownPlot {
		t.Fatalf("expected ErrUnknownPlot, got %v", err)
	}
	if err := Plant(s, c, 1, "nope", 1000); err != ErrUnknownCrop {
		t.Fatalf("expected ErrUnknownCrop, got %v", err)
	}
	if err := Plant(s, c, 1, "pumpkin", 1000); err != ErrCantAfford {
		t.Fatalf("expected ErrCantAfford for pumpkin at 20 coins, got %v", err)
	}
	if err := Plant(s, c, 1, "gamble", 1000); err != ErrLocked {
		t.Fatalf("expected ErrLocked for gated crop, got %v", err)
	}
}

func TestGrowthAndHarvestOverFixedSpans(t *testing.T) {
	c := testContent(t)
	s := newTestState(t, c)

	if err := Plant(s, c, 0, "turnip", 1000); err != nil {
		t.Fatal(err)
	}
	if s.PlotReady(c, 0, 1059) {
		t.Fatal("turnip ready 1s early")
	}
	if _, err := Harvest(s, c, 0, 1059); err != ErrNotMature {
		t.Fatalf("expected ErrNotMature, got %v", err)
	}
	if !s.PlotReady(c, 0, 1060) {
		t.Fatal("turnip not ready at grow time")
	}
	if got := s.PlotProgressPct(c, 0, 1030); got != 50 {
		t.Fatalf("progress = %d, want 50", got)
	}

	res, err := Harvest(s, c, 0, 1060)
	if err != nil {
		t.Fatal(err)
	}
	if res.Payout != 9 {
		t.Fatalf("payout = %d, want 9", res.Payout)
	}
	if s.Plots[0].Crop != "" {
		t.Fatal("plot not cleared after harvest")
	}
	if _, err := Harvest(s, c, 0, 1060); err != ErrPlotEmpty {
		t.Fatalf("double harvest: expected ErrPlotEmpty, got %v", err)
	}
	wantCoins := int64(25 - 5 + 9 + res.Discovery)
	if s.Coins != wantCoins {
		t.Fatalf("coins = %d, want %d", s.Coins, wantCoins)
	}
	if s.LifetimeHarvests != 1 || s.RunEarnings != 9+res.Discovery {
		t.Fatalf("stats wrong: harvests=%d earnings=%d", s.LifetimeHarvests, s.RunEarnings)
	}
}

func TestOfflineCatchUpMaturesCrops(t *testing.T) {
	c := testContent(t)
	s := newTestState(t, c)
	if err := Plant(s, c, 0, "turnip", 1000); err != nil {
		t.Fatal(err)
	}
	if err := Plant(s, c, 1, "turnip", 1000); err != nil {
		t.Fatal(err)
	}
	s.UpdatedAt = 1000

	// Tick before maturity: nothing.
	ev := Advance(s, c, 1030)
	if len(ev.Matured) != 0 {
		t.Fatalf("unexpected maturity at 1030: %v", ev.Matured)
	}
	// Long offline gap: both mature exactly once.
	ev = Advance(s, c, 100_000)
	if ev.Matured["turnip"] != 2 {
		t.Fatalf("matured = %v, want turnip:2", ev.Matured)
	}
	// Re-advancing reports nothing new and time cannot run backwards.
	ev = Advance(s, c, 100_001)
	if ev.Matured["turnip"] != 0 {
		t.Fatalf("duplicate maturity event: %v", ev.Matured)
	}
	before := *s
	ev = Advance(s, c, 50)
	if !ev.Empty() || s.UpdatedAt != before.UpdatedAt {
		t.Fatal("advance into the past must be a no-op")
	}
	// The crops are still standing, ready for manual harvest.
	if !s.PlotReady(c, 0, 100_001) || !s.PlotReady(c, 1, 100_001) {
		t.Fatal("offline-matured crops should await manual harvest")
	}
}

func TestDeterminismIdenticalInputsIdenticalOutputs(t *testing.T) {
	c := testContent(t)

	run := func() *State {
		s := New(c, 12345, 0)
		s.Coins = 10_000
		s.LifetimeEarnings = 10_000 // unlock the risky crop
		for i := 0; i < 3; i++ {
			if err := Plant(s, c, i, "gamble", 0); err != nil {
				t.Fatal(err)
			}
		}
		Advance(s, c, 600)
		for i := 0; i < 3; i++ {
			if _, err := Harvest(s, c, i, 600); err != nil {
				t.Fatal(err)
			}
		}
		Advance(s, c, 1_000_000)
		return s
	}

	a, b := run(), run()
	if !reflect.DeepEqual(a, b) {
		t.Fatalf("identical inputs diverged:\n%+v\n%+v", a, b)
	}
	if a.RNG == 12345 {
		t.Fatal("risky harvests should have advanced the RNG stream")
	}
}

func TestRiskyCropDistributionUsesConfiguredOutcomes(t *testing.T) {
	c := testContent(t)
	s := New(c, 7, 0)
	s.FlavorEnabled = false // isolate the variance roll
	s.LifetimeEarnings = 10_000

	outcomes := map[int64]int{}
	for n := 0; n < 500; n++ {
		s.Coins = 1000
		if err := Plant(s, c, 0, "gamble", int64(n)*1000); err != nil {
			t.Fatal(err)
		}
		res, err := Harvest(s, c, 0, int64(n)*1000+600)
		if err != nil {
			t.Fatal(err)
		}
		outcomes[res.Payout]++
	}
	if len(outcomes) != 2 {
		t.Fatalf("expected 2 distinct payouts (salvage/full), got %v", outcomes)
	}
	salvage := int64(140 / 8) // 17
	for _, want := range []int64{salvage, 140} {
		if outcomes[want] == 0 {
			t.Fatalf("payout %d never occurred in 500 rolls: %v", want, outcomes)
		}
	}
}

func TestLandExpansionCostScaling(t *testing.T) {
	c := testContent(t)
	s := newTestState(t, c)
	s.Coins = 1_000_000

	// base 50, growth 170%: 50, 85, 144 — then the 6-plot cap (3 start + 3).
	wantCosts := []int64{50, 85, 144}
	for _, want := range wantCosts {
		got := s.NextPlotCost(c)
		if got != want {
			t.Fatalf("next plot cost = %d, want %d", got, want)
		}
		paid, err := BuyPlot(s, c)
		if err != nil {
			t.Fatal(err)
		}
		if paid != want {
			t.Fatalf("paid %d, want %d", paid, want)
		}
	}
	if len(s.Plots) != 6 {
		t.Fatalf("plots = %d, want 6", len(s.Plots))
	}
	if _, err := BuyPlot(s, c); err != ErrMaxed {
		t.Fatalf("expected ErrMaxed at cap, got %v", err)
	}

	// Unaffordable purchase is refused without side effects.
	s2 := newTestState(t, c)
	s2.Coins = 10
	if _, err := BuyPlot(s2, c); err != ErrCantAfford {
		t.Fatalf("expected ErrCantAfford, got %v", err)
	}
	if s2.Coins != 10 || len(s2.Plots) != 3 {
		t.Fatal("refused purchase must not mutate state")
	}
}

func TestAutoHarvesterGathersOffline(t *testing.T) {
	c := testContent(t)
	s := newTestState(t, c)
	s.FlavorEnabled = false
	s.Coins = 10_000
	s.LifetimeEarnings = 10_000
	s.Plots[0].AutoHarvest = true
	if err := Plant(s, c, 0, "turnip", 1000); err != nil {
		t.Fatal(err)
	}
	coinsBefore := s.Coins
	s.UpdatedAt = 1000

	ev := Advance(s, c, 5000)
	if ev.AutoHarvested["turnip"] != 1 {
		t.Fatalf("auto-harvested = %v, want turnip:1", ev.AutoHarvested)
	}
	if s.Plots[0].Crop != "" {
		t.Fatal("plot should be empty after auto-harvest without auto-sower")
	}
	if s.Coins != coinsBefore+9 {
		t.Fatalf("coins = %d, want %d", s.Coins, coinsBefore+9)
	}
}

func TestAutoSowerReplantsRepeatedly(t *testing.T) {
	c := testContent(t)
	s := newTestState(t, c)
	s.FlavorEnabled = false
	s.Coins = 20_000
	s.LifetimeEarnings = 20_000
	s.Plots[0].AutoHarvest = true
	s.Plots[0].AutoSow = true
	if err := Plant(s, c, 0, "turnip", 0); err != nil {
		t.Fatal(err)
	}
	coinsBefore := s.Coins
	s.UpdatedAt = 0

	// 10 full turnip cycles in 600s: harvest+replant each, plot left planted.
	ev := Advance(s, c, 600)
	if ev.AutoHarvested["turnip"] != 10 {
		t.Fatalf("cycles = %d, want 10", ev.AutoHarvested["turnip"])
	}
	if s.Plots[0].Crop != "turnip" {
		t.Fatal("auto-sower should leave the plot replanted")
	}
	want := coinsBefore + 10*(9-5)
	if s.Coins != want {
		t.Fatalf("coins = %d, want %d", s.Coins, want)
	}
}

func TestVeryLongAutoFarmCatchUpIsBoundedAndExact(t *testing.T) {
	c := testContent(t)

	run := func(seconds int64) *State {
		s := New(c, 99, 0)
		s.FlavorEnabled = false
		s.Coins = 20_000
		s.LifetimeEarnings = 20_000
		s.Plots[0].AutoHarvest = true
		s.Plots[0].AutoSow = true
		if err := Plant(s, c, 0, "turnip", 0); err != nil {
			t.Fatal(err)
		}
		s.UpdatedAt = 0
		Advance(s, c, seconds)
		return s
	}

	// Far past the per-plot cycle cap: (cap + 1234) turnip cycles.
	cycles := int64(autoCycleCap + 1234)
	s := run(cycles * 60)
	coinsBefore := int64(20_000) - 5
	want := coinsBefore + cycles*(9-5)
	if s.Coins != want {
		t.Fatalf("batched catch-up coins = %d, want %d", s.Coins, want)
	}
	if s.LifetimeHarvests != cycles {
		t.Fatalf("harvests = %d, want %d", s.LifetimeHarvests, cycles)
	}
	// Determinism across the batch boundary.
	if s2 := run(cycles * 60); !reflect.DeepEqual(s, s2) {
		t.Fatal("batched catch-up is not deterministic")
	}
}

func TestPrestigeMathAndRebirth(t *testing.T) {
	c := testContent(t)
	s := newTestState(t, c)

	if _, err := Rebirth(s, c, 2000); err != ErrRebirthTooSoon {
		t.Fatalf("expected ErrRebirthTooSoon, got %v", err)
	}

	s.RunEarnings = 250_000
	s.LifetimeEarnings = 250_000
	if got := s.PrestigeGain(c); got != 50 { // isqrt(250000/100) = 50
		t.Fatalf("prestige gain = %d, want 50", got)
	}
	s.Coins = 9999
	s.Plots[0].AutoHarvest = true
	s.Zones["greenhouse"] = true
	s.Plots = append(s.Plots, Plot{Crop: "turnip", PlantedAt: 100})
	s.PurchasedPlots = 1

	gain, err := Rebirth(s, c, 5000)
	if err != nil {
		t.Fatal(err)
	}
	if gain != 50 || s.PrestigeCurrency != 50 {
		t.Fatalf("gain=%d currency=%d, want 50/50", gain, s.PrestigeCurrency)
	}
	if s.Coins != 25 || len(s.Plots) != 3 || s.PurchasedPlots != 0 {
		t.Fatal("run state did not reset")
	}
	if len(s.Zones) != 0 || len(s.Multipliers) != 0 || s.RunEarnings != 0 {
		t.Fatal("run-scoped purchases must reset on rebirth")
	}
	if s.Plots[0].AutoHarvest {
		t.Fatal("plot automation must reset on rebirth")
	}
	if s.Rebirths != 1 || s.LifetimeEarnings != 250_000 {
		t.Fatal("lifetime stats must survive rebirth")
	}
	if s.UpdatedAt != 5000 {
		t.Fatalf("UpdatedAt = %d, want 5000", s.UpdatedAt)
	}
}

func TestUpgradesAffectTheNextRun(t *testing.T) {
	c := testContent(t)
	s := newTestState(t, c)
	s.PrestigeCurrency = 100

	// growth: 2, 4 prestige for levels 1-2.
	if cost := s.UpgradeCost(c.UpgradeByID("growth")); cost != 2 {
		t.Fatalf("growth L1 cost = %d, want 2", cost)
	}
	if err := BuyUpgrade(s, c, "growth"); err != nil {
		t.Fatal(err)
	}
	if cost := s.UpgradeCost(c.UpgradeByID("growth")); cost != 4 {
		t.Fatalf("growth L2 cost = %d, want 4", cost)
	}
	if err := BuyUpgrade(s, c, "haggling"); err != nil {
		t.Fatal(err)
	}
	if err := BuyUpgrade(s, c, "inheritance"); err != nil {
		t.Fatal(err)
	}
	if err := BuyUpgrade(s, c, "surveying"); err != nil {
		t.Fatal(err)
	}

	turnip := c.Crop("turnip")
	if got := s.GrowSeconds(c, turnip); got != 54 { // 60 * 90%
		t.Fatalf("grow seconds = %d, want 54", got)
	}
	if got := s.sellMultiplied(c, 100, ""); got != 115 {
		t.Fatalf("sell multiplied = %d, want 115", got)
	}
	if got := s.NextPlotCost(c); got != 45 { // 50 * 90%
		t.Fatalf("discounted plot = %d, want 45", got)
	}
	if got := startCoins(c, s.Upgrades); got != 175 { // 25 + 150
		t.Fatalf("start coins = %d, want 175", got)
	}

	// Max-level and unaffordable refusals.
	s.PrestigeCurrency = 0
	if err := BuyUpgrade(s, c, "growth"); err != ErrCantAffordPP {
		t.Fatalf("expected ErrCantAffordPP, got %v", err)
	}
	s.PrestigeCurrency = 1_000_000
	for i := 0; i < 10; i++ {
		_ = BuyUpgrade(s, c, "growth")
	}
	if s.UpgradeLevel("growth") != 5 {
		t.Fatalf("growth level = %d, want max 5", s.UpgradeLevel("growth"))
	}
	if err := BuyUpgrade(s, c, "growth"); err != ErrMaxed {
		t.Fatalf("expected ErrMaxed, got %v", err)
	}
}

// playUntil simulates simple optimal turnip farming until the wallet reaches
// target, returning the in-game seconds it took.
func playUntil(t *testing.T, c *content.Content, s *State, target int64) int64 {
	t.Helper()
	start := s.UpdatedAt
	turnip := c.Crop("turnip")
	for s.Coins < target {
		now := s.UpdatedAt
		for i := range s.Plots {
			if s.Plots[i].Crop == "" && s.Coins >= turnip.SeedCost {
				if err := Plant(s, c, i, "turnip", now); err != nil {
					t.Fatal(err)
				}
			}
		}
		now += s.GrowSeconds(c, turnip)
		Advance(s, c, now)
		for i := range s.Plots {
			if s.PlotReady(c, i, now) {
				if _, err := Harvest(s, c, i, now); err != nil {
					t.Fatal(err)
				}
			}
		}
		if now-start > 10_000_000 {
			t.Fatal("playUntil never reached target")
		}
	}
	return s.UpdatedAt - start
}

func TestRebirthPacingSecondRunIsFaster(t *testing.T) {
	c := testContent(t)

	fresh := New(c, 1, 0)
	fresh.FlavorEnabled = false
	firstRun := playUntil(t, c, fresh, 1000)

	// A post-rebirth save with a few permanent upgrades.
	reborn := New(c, 1, 0)
	reborn.FlavorEnabled = false
	reborn.RunEarnings = 250_000
	reborn.LifetimeEarnings = 250_000
	if _, err := Rebirth(reborn, c, 0); err != nil {
		t.Fatal(err)
	}
	for _, id := range []string{"growth", "haggling", "inheritance"} {
		if err := BuyUpgrade(reborn, c, id); err != nil {
			t.Fatal(err)
		}
	}
	secondRun := playUntil(t, c, reborn, 1000)

	if secondRun >= firstRun {
		t.Fatalf("pacing goal failed: second run took %ds, first run %ds", secondRun, firstRun)
	}
}

func TestZonePurchaseAddsPlotsAndUnlocksCrops(t *testing.T) {
	c := testContent(t)
	s := newTestState(t, c)
	s.Coins = 30_000
	s.LifetimeEarnings = 20_000

	if err := Plant(s, c, 0, "hothouse_vine", 0); err != ErrLocked {
		t.Fatalf("zone crop should be locked, got %v", err)
	}
	if err := BuyZone(s, c, "greenhouse"); err != nil {
		t.Fatal(err)
	}
	if len(s.Plots) != 9 { // 3 + 6
		t.Fatalf("plots = %d, want 9", len(s.Plots))
	}
	if err := Plant(s, c, 5, "hothouse_vine", 0); err != nil {
		t.Fatalf("zone crop should unlock with the zone: %v", err)
	}
	if err := BuyZone(s, c, "greenhouse"); err != ErrAlreadyOwned {
		t.Fatalf("expected ErrAlreadyOwned, got %v", err)
	}
}

func TestPlotAutoGatingAndAffordability(t *testing.T) {
	c := testContent(t)
	s := newTestState(t, c)

	if err := UpgradePlotAuto(s, c, 0, "harvest"); err != ErrCantAfford {
		t.Fatalf("expected ErrCantAfford, got %v", err)
	}
	s.Coins = 1000
	if err := UpgradePlotAuto(s, c, 0, "harvest"); err != nil {
		t.Fatal(err)
	}
	if err := UpgradePlotAuto(s, c, 0, "harvest"); err != ErrAlreadyOwned {
		t.Fatalf("expected ErrAlreadyOwned, got %v", err)
	}
	if err := UpgradePlotAuto(s, c, 0, "sow"); err != ErrLocked {
		t.Fatalf("expected ErrLocked before earnings gate for sow, got %v", err)
	}
	s.LifetimeEarnings = 5000
	if err := UpgradePlotAuto(s, c, 0, "sow"); err != ErrCantAfford {
		t.Fatalf("expected ErrCantAfford for sow, got %v", err)
	}
}

func TestAchievementsFireOnceAndPersist(t *testing.T) {
	c := testContent(t)
	s := newTestState(t, c)

	if err := Plant(s, c, 0, "turnip", 1000); err != nil {
		t.Fatal(err)
	}
	if _, err := Harvest(s, c, 0, 1060); err != nil {
		t.Fatal(err)
	}
	newly := s.CheckAchievements(c, 1060)
	if len(newly) != 1 || newly[0] != "first_sprout" {
		t.Fatalf("newly earned = %v, want [first_sprout]", newly)
	}
	if s.Achievements["first_sprout"] != 1060 {
		t.Fatal("achievement timestamp not recorded")
	}
	if again := s.CheckAchievements(c, 2000); len(again) != 0 {
		t.Fatalf("achievement fired twice: %v", again)
	}

	// Round-trips through serialization.
	b, err := s.Encode()
	if err != nil {
		t.Fatal(err)
	}
	s2, err := DecodeState(b)
	if err != nil {
		t.Fatal(err)
	}
	if s2.Achievements["first_sprout"] != 1060 {
		t.Fatal("achievement lost in round-trip")
	}
}

func TestStateRoundTripAndPayloadUpgrade(t *testing.T) {
	c := testContent(t)
	s := newTestState(t, c)
	s.Coins = 777
	if err := Plant(s, c, 1, "turnip", 1234); err != nil {
		t.Fatal(err)
	}

	b, err := s.Encode()
	if err != nil {
		t.Fatal(err)
	}
	got, err := DecodeState(b)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(s, got) {
		t.Fatalf("round-trip mismatch:\n%+v\n%+v", s, got)
	}

	// A version-1 payload (pre-progression) upgrades cleanly and additively.
	v1 := []byte(`{"version":1,"rng":9,"updated_at":50,"coins":123,` +
		`"plots":[{"crop":"turnip","planted_at":10},{}],"purchased_plots":0,` +
		`"run_earnings":200}`)
	up, err := DecodeState(v1)
	if err != nil {
		t.Fatal(err)
	}
	if up.Version != StateVersion {
		t.Fatalf("version = %d, want %d", up.Version, StateVersion)
	}
	if up.Coins != 123 || up.Plots[0].Crop != "turnip" || up.RunEarnings != 200 {
		t.Fatal("v1 fields lost during upgrade")
	}
	if up.Upgrades == nil || up.Achievements == nil || up.Tools == nil || up.Zones == nil {
		t.Fatal("upgrade must initialize new maps")
	}
	if !up.FlavorEnabled {
		t.Fatal("upgrade should default flavor on")
	}

	// Unknown future versions are refused, not mangled.
	if _, err := DecodeState([]byte(`{"version":99}`)); err == nil {
		t.Fatal("expected error for future payload version")
	}
	if _, err := DecodeState([]byte(`{not json`)); err == nil {
		t.Fatal("expected error for malformed payload")
	}
}

func TestFlavorDiscoveriesAreSeededAndOptional(t *testing.T) {
	c := testContent(t)

	harvestMany := func(seed uint64, flavor bool) (int64, int) {
		s := New(c, seed, 0)
		s.FlavorEnabled = flavor
		discoveries := 0
		for n := int64(0); n < 300; n++ {
			s.Coins = 100
			if err := Plant(s, c, 0, "turnip", n*100); err != nil {
				t.Fatal(err)
			}
			res, err := Harvest(s, c, 0, n*100+60)
			if err != nil {
				t.Fatal(err)
			}
			if res.Discovery > 0 {
				discoveries++
				if res.Discovery < c.Flavor.DiscoveryMin || res.Discovery > c.Flavor.DiscoveryMax {
					t.Fatalf("discovery %d outside configured range", res.Discovery)
				}
			}
		}
		return s.Coins, discoveries
	}

	coinsA, discA := harvestMany(555, true)
	coinsB, discB := harvestMany(555, true)
	if coinsA != coinsB || discA != discB {
		t.Fatal("flavor must be reproducible under a fixed seed")
	}
	if discA == 0 {
		t.Fatal("4% discovery chance should fire at least once in 300 harvests")
	}
	if _, disc := harvestMany(555, false); disc != 0 {
		t.Fatal("disabled flavor must produce no discoveries")
	}
}

func TestIsqrtBoundaries(t *testing.T) {
	cases := map[int64]int64{
		0: 0, 1: 1, 3: 1, 4: 2, 99: 9, 100: 10,
		(1 << 62): 1 << 31, // exact power of four
		1<<63 - 1: 3037000499, // MaxInt64: must not overflow the loops
	}
	for n, want := range cases {
		if got := isqrt(n); got != want {
			t.Errorf("isqrt(%d) = %d, want %d", n, got, want)
		}
	}
	if got := isqrt(-5); got != 0 {
		t.Errorf("isqrt(-5) = %d, want 0", got)
	}
}

func TestSaturatingMoneyNeverOverflows(t *testing.T) {
	c := testContent(t)
	s := newTestState(t, c)
	s.Coins = 1<<63 - 10
	s.LifetimeEarnings = 1<<63 - 10
	s.credit(100)
	if s.Coins < 0 || s.LifetimeEarnings < 0 {
		t.Fatal("credit overflowed into negative")
	}
	if s.Coins != 1<<63-1 {
		t.Fatalf("expected saturation at MaxInt64, got %d", s.Coins)
	}
	_ = c
}

func TestMercyPlantWhenBrokeAndEmpty(t *testing.T) {
	c := testContent(t)
	s := newTestState(t, c)
	s.Coins = 2
	if err := Plant(s, c, 0, "turnip", 1000); err != nil {
		t.Fatalf("mercy plant: %v", err)
	}
	if s.Coins != 2 {
		t.Fatalf("mercy plant should be free, coins = %d", s.Coins)
	}
	if err := Plant(s, c, 1, "pumpkin", 1000); err != ErrCantAfford {
		t.Fatalf("only cheapest crop is free, got %v", err)
	}
}

func TestPayloadUpgradeV2ToV3MigratesAutoTools(t *testing.T) {
	v2 := []byte(`{"version":2,"rng":1,"updated_at":100,"coins":50,` +
		`"plots":[{},{}],"tools":{"auto_harvester":true,"auto_sower":true},` +
		`"run_earnings":0}`)
	up, err := DecodeState(v2)
	if err != nil {
		t.Fatal(err)
	}
	if up.Version != StateVersion {
		t.Fatalf("version = %d, want %d", up.Version, StateVersion)
	}
	for i := range up.Plots {
		if !up.Plots[i].AutoHarvest || !up.Plots[i].AutoSow {
			t.Fatalf("plot %d missing migrated automation flags", i)
		}
	}
}

func TestVisibleCropsHidesFuturePrestigeTiers(t *testing.T) {
	c, err := content.Load("")
	if err != nil {
		t.Fatal(err)
	}
	s := New(c, 1, 0)
	visible := VisibleCrops(s, c)
	for _, crop := range visible {
		if crop.Unlock.Kind == "prestige" && crop.Unlock.Value > 1 {
			t.Fatalf("rebirth-0 save should not see %q (tier %d)", crop.ID, crop.Unlock.Value)
		}
	}
	s.Rebirths = 1
	visible = VisibleCrops(s, c)
	found := false
	for _, crop := range visible {
		if crop.ID == "frostplum" {
			found = true
		}
	}
	if !found {
		t.Fatal("rebirth 1 should preview frostplum as locked")
	}
}

func TestProfitPerSecondIncreasesWithGrowTime(t *testing.T) {
	c, err := content.Load("")
	if err != nil {
		t.Fatal(err)
	}
	type row struct {
		id   string
		grow int64
		pps  float64
	}
	var rows []row
	for _, crop := range c.Crops {
		if crop.Unlock.Kind != "start" && crop.Unlock.Kind != "earnings" {
			continue
		}
		var pps float64
		if crop.Archetype == "risky" {
			failPct := float64(crop.FailChancePct)
			salvage := float64(crop.SellValue) / 8
			ev := (failPct*salvage + (100-failPct)*float64(crop.SellValue)) / 100
			pps = (ev - float64(crop.SeedCost)) / float64(crop.GrowSeconds)
		} else {
			pps = float64(crop.SellValue-crop.SeedCost) / float64(crop.GrowSeconds)
		}
		rows = append(rows, row{crop.ID, crop.GrowSeconds, pps})
	}
	for i := 0; i < len(rows); i++ {
		for j := i + 1; j < len(rows); j++ {
			if rows[i].grow >= rows[j].grow {
				continue
			}
			if rows[j].pps+1e-9 < rows[i].pps {
				t.Fatalf("slower %s (%.4f/s) should beat faster %s (%.4f/s)",
					rows[j].id, rows[j].pps, rows[i].id, rows[i].pps)
			}
		}
	}
}

func TestGiftRedeemAndCap(t *testing.T) {
	c := testContent(t)
	s := newTestState(t, c)
	if _, err := RedeemGift(s, c); err != ErrNoGift {
		t.Fatalf("expected ErrNoGift, got %v", err)
	}
	s.GiftPending = true
	res, err := RedeemGift(s, c)
	if err != nil {
		t.Fatal(err)
	}
	if res.Coins < c.Gifts.CoinRewardFloor {
		t.Fatalf("gift coins %d below floor", res.Coins)
	}
	if s.GiftPending {
		t.Fatal("gift should be consumed")
	}
}

func TestSalvageUpgradeImprovesPayout(t *testing.T) {
	c := testContent(t)
	s := newTestState(t, c)
	crop := c.Crop("gamble")
	base := s.SalvageValue(c, crop)
	s.SeedUpgrades["gamble"] = 3
	upgraded := s.SalvageValue(c, crop)
	if upgraded <= base {
		t.Fatalf("salvage should improve: %d -> %d", base, upgraded)
	}
}
