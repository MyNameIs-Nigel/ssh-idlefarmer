---
name: simulation-engine
description: Build the deterministic, headless game-simulation engine for ssh-idlefarmer — the plant/grow/harvest/earn core loop, data-driven crop definitions, plots and land expansion, the wallet/economy, and offline progression computed as a pure function of elapsed time so it is reproducible and fully unit-testable independent of the SSH and UI layers.
---

# Task 3 — Game Simulation Engine (Core Loop & Offline Progression)

## Objective

Implement the heart of the game as a **headless, deterministic** package: given a save's state and an elapsed duration, it computes the new state. This is where crops grow, harvests pay out, and land gets bought. Keeping it independent of SSH (Task 1) and the terminal UI (Task 5) makes the game logic testable and reproducible, and makes offline progression a clean calculation rather than a tangle of timers.

By the end of this task the game is fully *playable in principle* — you can drive the engine from tests and see a farm progress over simulated time — even though the only interface so far is the minimal screen from Task 2.

## Prerequisites

- Task 1 (server, identity) and Task 2 (persistence, save lifecycle) complete.
- The save record from Task 2 is ready to hold the real state fields defined here (extend the stored payload/columns accordingly).

## Scope

### In scope
- The canonical in-memory representation of a farm's state.
- Data-driven crop definitions loaded from content files.
- The plant → grow → harvest → earn loop.
- The wallet/economy and selling.
- Plots and buying additional land.
- The time model: real-time growth, lazy offline catch-up on connect, and a live tick while connected.
- Determinism and a thorough unit-test suite.

### Out of scope (deferred)
- Rebirth/prestige, unlock gating, achievements, ambient flavor — Task 4.
- All rendering, input handling, and screens — Task 5.

## Requirements

### Determinism (the central constraint)
- The engine's state transitions must be **pure functions of their inputs**. The core advance function takes the current state plus a "from" and "to" timestamp (or an elapsed duration) and returns the new state. It must **not** read the wall clock internally — time is always passed in.
- Given identical inputs it must produce identical outputs. If any randomness is used (e.g. for risky crops), it must be seeded and reproducible, with the seed/state part of the save so a replay yields the same result.
- This makes offline catch-up and live ticks the *same* code path, just with different time spans, and makes the whole engine unit-testable without SSH, the database, or a clock.

### Farm state representation
Define an explicit state structure including at least:
- the wallet balance,
- the set of plots, each with: whether it is planted, which crop, the time it was planted, and its current growth stage / maturity,
- the player's currently owned land (plot count) and any per-plot unlock state,
- the last-active timestamp (sourced from the Task 2 save record) used to compute elapsed time.
Keep this structure serializable so Task 2's storage can persist it.

### Crop definitions (data-driven)
- Crop stats live in **content files under `data/`** (TOML or JSON), loaded at startup and embeddable via Go's `embed` so they ship inside the binary. Balancing must not require recompiling logic.
- Each crop defines at minimum: an id/name, seed cost, time to mature, sell value, and which archetype it belongs to.
- Provide at least three archetypes so the economy has texture:
  - **fast / cheap** — short grow time, low cost, modest reliable return (the early-game staple),
  - **slow / valuable** — long grow time, higher cost, larger payout (rewards patience),
  - **risky** — a chance of partial failure or a chance of a bonus yield, higher variance (uses the seeded randomness above).
- Validate content on load: reject malformed or contradictory definitions with a clear error rather than starting in a broken state.

### Core loop
- **Plant:** spend the seed cost to plant a crop on an empty plot, recording the plant time. Reject planting if the wallet cannot afford it or the plot is occupied.
- **Grow:** maturity is a function of (now − plant time) against the crop's grow time. Crops continue maturing whether or not the player is connected.
- **Harvest:** a mature plot can be harvested, which clears the plot and credits the crop's sell value (and applies risky-crop variance if applicable). Harvesting an immature plot is rejected (or, if you prefer, allowed with no/penalized yield — document the choice).
- **Earn & sell:** harvest proceeds go to the wallet. If you add a separate market/sell step rather than auto-crediting, define it here.

### Economy & land expansion
- The wallet is the single currency at this stage (prestige currency arrives in Task 4).
- Define a **land expansion** mechanic: spend money to add plots. The cost should scale with the number already owned (e.g. each additional plot costs more than the last) so expansion is a meaningful long-run sink. Put the cost curve parameters in the content/balance files.
- Keep the early curve gentle and the feel low-stakes — there should be no failure state that loses the player's farm or money beyond an unaffordable purchase being refused.

### Time model
- **Offline catch-up:** on connect, the engine advances the loaded save from its last-active timestamp to now in a single computation — crops that matured while the player was away are mature on arrival. This is the "something always happened while I was gone" property.
- **Live ticks:** while connected, the session advances the state on a regular interval (e.g. once per second) so the player can watch crops progress. Because the advance function is pure and time-driven, this is the same logic as catch-up over a tiny span.
- The engine exposes the state and the available actions; it does not own the timer loop or the rendering — Task 5 drives ticks and input, calling into the engine.

### Testability
- Provide a unit-test suite that exercises: planting affordability, growth over fixed spans, harvest payouts, offline catch-up over long spans, risky-crop reproducibility under a fixed seed, and land-expansion cost scaling.
- Tests must run without SSH, without the database, and without sleeping on real time (advance by passing timestamps). This is both a quality bar and a demonstration of the determinism requirement.

## Security considerations

- Validate all content files on load; treat them as a configuration surface.
- Guard every action against invalid state transitions (negative balances, double-harvest, planting on an occupied plot) — the engine is the authority on legal moves, and the UI must not be trusted to enforce rules.
- Keep economic operations integer-based or otherwise free of floating-point drift where balances are concerned, so long-running saves don't accumulate rounding errors.

## Suggested artifacts produced by this task

- `internal/sim/` — state types, the pure advance function, action functions (plant/harvest/buy), and the economy rules.
- `internal/content/` — loading and validating crop/balance definitions.
- `data/crops.toml` (or `.json`) and a balance/config content file.
- A `_test.go` suite under `internal/sim/`.
- Extensions to Task 2's stored save payload to hold the real farm state.

## Acceptance criteria

- [ ] The advance function is pure and time-parameterized; it reads no internal clock.
- [ ] Identical inputs yield identical outputs; risky-crop randomness is seeded and reproducible.
- [ ] Crops mature over real time and are correctly mature after an offline gap on reconnect.
- [ ] Planting, harvesting, and selling behave correctly and reject illegal actions.
- [ ] At least three crop archetypes exist and are defined in data files, not code.
- [ ] Land expansion works and its cost scales as configured.
- [ ] The unit-test suite passes and runs without SSH, the database, or real-time sleeps.
- [ ] Saved state round-trips through Task 2's storage and resumes correctly.

## Verification steps

1. Run the unit suite; confirm coverage of the loop, offline catch-up, and seeded randomness.
2. Drive a scripted scenario through the engine: plant, advance an hour, harvest, buy a plot, advance a day, harvest again — confirm expected balances.
3. Persist a save (Task 2), simulate a long offline gap, reconnect, and confirm crops matured during the gap.
4. Tweak a crop's stats in the data file and confirm behavior changes without recompiling logic.

## If blocked

If determinism cannot be preserved (e.g. a dependency forces hidden clock access or non-reproducible randomness), stop and report it — determinism is a hard requirement here because Task 5 and the offline model both rely on it. Do not move time-handling into the engine's internals as a workaround.
