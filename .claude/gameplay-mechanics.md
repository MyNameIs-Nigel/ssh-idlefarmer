# Gameplay Mechanics Reference

State of the game after the Gameplay Enrichment Overhaul (state v3). All numbers
live in `data/crops.toml` and `data/balance.toml` (embedded at build time,
overridable with `IDLEFARM_DATA_DIR`). Engine logic is in `internal/sim/`.

## Core loop

Plant → wait (online or offline) → harvest → reinvest in plots, multipliers,
and automation → rebirth at 10,000+ run earnings for Starseeds → spend
Starseeds on permanent upgrades → repeat with faster runs and new crops.

## Economy principle: patience pays

Profit-per-second strictly increases with grow time. Slow crops gate via large
seed costs, so faster crops still matter early in a run. Enforced by
`TestProfitPerSecondIncreasesWithGrowTime` in `internal/sim/sim_test.go`.

### Crop lineup

| Crop | Archetype | Seed | Grow | Sell | Profit/s | Unlock |
|------|-----------|------|------|------|----------|--------|
| Turnip | fast | 5 | 60s | 9 | 0.067 | start (the mercy crop) |
| Carrot | fast | 14 | 4m | 32 | 0.075 | start |
| Glimmercorn | risky | 40 | 15m | 150 | ~0.086 EV | 500 lifetime earnings |
| Pumpkin | slow | 80 | 1h | 440 | 0.10 | start |
| Starfruit | slow | 600 | 8h | 4,800 | 0.146 | 2,000 lifetime earnings |
| Moonberry | risky | 350 | 2h | 1,300 | ~0.08 EV | rebirth 1 |
| Emberwheat | fast | 120 | 10m | 190 | ~0.12 | rebirth 1 |
| Frostplum | slow | 900 | 4h | 2,700 | ~0.125 | rebirth 2 |
| Thunderpod | risky | 200 | 30m | 520 | ~0.15 EV | rebirth 2 |
| Sunroot | fast | 400 | 20m | 560 | ~0.13 | rebirth 3 |
| Voidlotus | slow | 2,000 | 12h | 14,000 | ~0.28 | rebirth 3 |
| Dewmelon | slow | 1,200 | 12h | 10,000 | 0.20 | greenhouse zone |

Two new crops per rebirth tier. Prestige crops are strictly better than the
base curve at comparable grow times — they are the reward for rebirthing.

### Crop visibility rule

A prestige-locked crop with unlock value V is shown (as locked) only when
`Rebirths >= V-1`; otherwise hidden entirely. Earnings/zone-locked crops are
always visible-locked. Implemented as `sim.VisibleCrops`.

## Risky crops (salvage model)

- Two outcomes per harvest: full sell value, or **salvage** on failure.
- Base salvage is **1/8 of sell value**; `fail_chance_pct` per crop (25–30%).
- **Hardier Strain** upgrades (Market, coins, run-scoped, 3 levels) raise
  salvage: 1/8 → 1/4 → 1/2 → 3/4. One strain upgrade per risky crop.
- Failure produces an explicit notice ("💥 X failed — salvaged Nc (1/8 of
  normal)") and a failure count in the away report.
- Salvage fraction logic: `sim.SalvageNumerator` (numerators 1/2/4/6 over 8).

## Soft-lock mercy plant

If **all plots are empty** and coins are below the cheapest unlocked crop's
seed cost, planting that crop is **free** (picker shows "FREE — the land
provides"). Only the cheapest unlocked crop qualifies; fires only when broke
and crop-less, so it is not exploitable. See `sim.MercyPlantEligible`.

## Rebirth and Starseeds

- **Starseeds** is the prestige currency (JSON key stays `prestige_currency`
  for save compatibility; display label from `[labels]` in balance.toml).
- Gain on rebirth: `isqrt(run_earnings / 100)`, requires 10,000+ run earnings.
- Rebirth resets: coins, plots (and their automation), zones, multipliers,
  Hardier Strains, active events. Keeps: Starseeds, permanent upgrades,
  achievements, lifetime stats, crop unlocks, farm name, pending gift.
- Rebirth tab previews the next tier's crop unlocks.

### Permanent upgrades (Progress tab, locked until first rebirth)

| Upgrade | Effect | Levels |
|---------|--------|--------|
| Rich Soil | −10% grow time / level | 5 |
| Market Haggling | +15% sell / level | 5 |
| Inheritance | +150 start coins / level | 4 |
| Surveying | −10% plot cost / level | 4 |
| Gift Magnet | +20% gift arrival rate / level | 3 |
| Stargazer's Almanac | +15% event rate, +10% duration / level | 3 |

## Market tab (coins, run-scoped)

- **Multipliers**: Fertilizer (−8% grow/level ×3), Merchant's Scale (+10%
  sell/level ×3). Escalating costs (×2.5 per level).
- **Hardier Strains**: per-risky-crop salvage upgrades (see above).
- **Zones**: Greenhouse (25,000c, +6 plots, unlocks Dewmelon).
- Seeds are NOT in the Market — the seed catalog lives in the Land tab.

## Per-plot automation (replaces the old global tools)

- `Plot.AutoHarvest` / `Plot.AutoSow` booleans; submenu on Farm via `u`.
- Auto-Harvest: base 600c, ×1.5 per plot already automated.
- Auto-Sow: flat 1,400c, requires that plot's harvester **and** 5,000 lifetime
  earnings.
- Resets on rebirth (plots are run-scoped). v2 saves with the old global
  auto-harvester/sower get the flags migrated onto all current plots.
- Offline catch-up simulates per-plot cycles; beyond 50,000 cycles the
  remainder settles at expected value (deterministic, O(1)).

## Gift parcels

- Max **1 pending** at a time; persists across rebirth and disconnects.
- Rolled in `Advance`: online ticks ~1 per 20 min (boosted by Gift Magnet);
  offline gap rolls once at ~1 per 5h.
- Banner "📦 A parcel waits at the gate — press g"; `g` redeems anywhere.
- Contents: with `Rebirths >= 1`, 20% chance of Starseeds (scaled by rebirth
  count); otherwise coins scaled from run earnings + rebirths, floor 8c,
  ceiling 5,000c, ±20% jitter.

## Online random events

- Trigger only on live ticks (~1 per 15–25 min online); last 90–180s; expire
  by timestamp so disconnects are safe. Stored as `EventID`/`EventEndsAt`.
- Types (TOML-driven): **Market Day** (seeds −40%), **Bumper Demand** (+30%
  sell), **Warm Front** (grow 2× faster — −50% grow time).
- Stargazer's Almanac makes them more frequent and longer.
- Header banner shows the event with a live countdown.

## Fun extras

1. **The Daily Furrow** — rotating one-line newspaper banner in the header;
   headlines from TOML, overridden by active events/pending gifts.
2. **Richer away report** — narrative vignettes ("A fox watched your turnips.
   It did not help."), per-crop maturity breakdown, failures, gift notice.
3. **Golden Harvest** — 1% chance any harvest pays 10×, with a celebration.
4. **Farm naming** — `n` on the Stats screen, max 24 runes, shown in header.
5. **Moon phases** — cosmetic 28-day cycle in the header; full moon grants
   Moonberry +15% sell.
6. **Critter visits** — cosmetic critters (crow/rabbit/mole) appear on empty
   plots while online; `x` shoos them for 3–12 coins.

## Determinism invariants

- All randomness flows from the save's seeded splitmix64 RNG; `Advance` is a
  pure function of (state, timestamp). No wall-clock reads in `internal/sim`.
- Roll order inside a harvest is fixed: risky roll → golden roll → discovery
  roll. Changing this order breaks replay determinism.
- State is JSON, version 3. `DecodeState` upgrades v1/v2 additively; future
  versions are refused.

## Balance gotchas / lessons learned

- **Prestige crops must clear the profit curve.** Emberwheat initially shipped
  selling 95c against a 120c seed (a guaranteed loss); Sunroot had the same
  bug. The EV monotonicity test only covered start/earnings crops, so
  prestige-tier numbers must be checked by hand (or extend the test) when
  adding crops.
- Risky EV must account for the salvage floor: EV = fail% × (sell/8) +
  (1−fail%) × sell, before sell multipliers.
- Grow-speed reductions stack additively (upgrades + multipliers + events) and
  are clamped below 100%; content validation prevents any single source from
  reaching 100% at max level.
