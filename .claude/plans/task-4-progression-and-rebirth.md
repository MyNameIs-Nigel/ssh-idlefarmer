---
name: progression-rebirth
description: Extend the ssh-idlefarmer simulation with the long-arc meta systems — a rebirth/prestige loop that trades current progress for permanent global bonuses, gated unlocks for new crops, automation, and zones, achievements and milestones, and optional low-stress ambient flavor — all data-driven and tuned so each prestige cycle is faster and more satisfying than the last.
---

# Task 4 — Progression, Rebirth / Prestige & Meta Systems

## Objective

Turn the playable core loop into a game with depth and a reason to keep returning. This task adds the prestige/rebirth system that defines the game's long arc, the unlock progression that keeps a "next goal" always in view, achievements for small wins, and optional ambient flavor for cozy texture. All of it stays inside the deterministic engine from Task 3 and remains UI-agnostic.

## Prerequisites

- Task 3 complete: a deterministic engine with the plant/grow/harvest/earn loop, data-driven crops, plots, and land expansion.
- Task 2's migration mechanism is available to add the new persistent fields this task introduces.

## Scope

### In scope
- The rebirth/prestige loop: prestige currency, what is sacrificed, and permanent bonuses.
- Unlock/progression gating for crops, automation, and zones.
- Achievements and milestones.
- Optional ambient flavor (seasons, weather, small discoveries), kept low-stress.
- The balance configuration for all of the above, in data files.
- A schema migration to persist the new state.

### Out of scope (deferred)
- Rendering any of this — Task 5 presents it.
- Deployment — Task 6.

## Requirements

### Rebirth / prestige loop
This is the system that gives the game its shape, so design it deliberately:
- **Prestige currency:** earned when the player chooses to rebirth, as a function of accumulated progress (e.g. total lifetime earnings on the current run). Define the formula and put its parameters in the balance file.
- **What is sacrificed:** rebirthing resets the current run — wallet, plots, planted crops — back to a starting state.
- **What persists:** prestige currency and the permanent bonuses bought with it. These carry across runs and are the point of rebirthing.
- **Permanent bonuses:** let players spend prestige currency on lasting multipliers/upgrades — e.g. faster growth, higher sell prices, a head start on plots or starting cash, cheaper expansion. Define a small initial set, each with a cost in prestige currency, all data-driven.
- **Pacing — the key design goal:** each cycle should reach prior milestones **faster** than the last, so rebirthing feels like acceleration, not punishment. Tune starting bonuses so the early game of a second run is noticeably quicker and the loop stays satisfying rather than grindy. The first rebirth in particular should feel clearly worth it.
- Rebirth must be an explicit player choice with a clear preview of what they will gain and lose — never automatic, never surprising.

### Unlock / progression gating
- Provide overlapping things to work toward so there is always a next goal:
  - **new crops** unlocked by progress (earnings reached, prestige level, or achievements),
  - **automation** — e.g. auto-harvest or auto-replant, sprinklers, or similar quality-of-life upgrades that reduce keystrokes (fitting the low-stakes, drop-in feel),
  - **zones / additional fields** — new areas (a greenhouse, a second field) that expand capacity or unlock crop types.
- Each unlock has a clearly defined gate (the condition that grants it) and is defined in data/config, not hardcoded.
- Unlock state is part of the persisted save and must survive rebirth appropriately (decide and document which unlocks are per-run vs. permanent — typically prestige upgrades are permanent, run-specific expansions reset).

### Achievements & milestones
- A set of milestones that fire on defined conditions (first harvest, N crops sold, N plots owned, first rebirth, a balance threshold, etc.), defined in data/config.
- Each achievement records when it was earned and persists. Achievements may optionally serve as unlock gates.
- Keep them as positive reinforcement only — no penalties, in keeping with the relaxing tone.

### Ambient flavor (optional, low-stress)
- Optionally add texture that never creates stress or a fail state:
  - **seasons** that make some crops more/less productive at certain times,
  - **weather** as a transient bonus (a rain day speeds growth) rather than a hazard,
  - **small discoveries** (an occasional rare seed or a found coin) using the engine's seeded, reproducible randomness.
- Anything here must be clearly optional and toggleable, and must not punish a player for being away or for short sessions.

### Balance & data
- All tunable numbers — prestige formula, bonus costs and effects, unlock gates, achievement thresholds, flavor parameters — live in the content/balance files from Task 3, loaded and validated at startup.
- Keep the engine deterministic: prestige math and any flavor randomness follow the same purity/seeding rules established in Task 3.

### Persistence
- Add a Task 2 migration to persist the new fields: prestige currency, owned permanent bonuses, unlock state, achievement records, and any seasonal/flavor state. The migration must be non-destructive to existing saves.

## Security considerations

- Validate the expanded content/balance files on load; a bad prestige formula or unlock gate should fail fast, not corrupt saves.
- The engine remains the authority: rebirth, purchases, and unlocks must be validated server-side and never trusted from the UI.
- Keep prestige/economy math integer-based or drift-free for long-lived accounts.

## Suggested artifacts produced by this task

- Extensions to `internal/sim/` for prestige, unlocks, achievements, and optional flavor.
- Additional `data/` content for bonuses, unlock gates, achievement definitions, and flavor parameters.
- A new Task 2 migration for the added persistent fields.
- Test coverage for prestige math, gate conditions, and reproducible flavor randomness.

## Acceptance criteria

- [ ] A player can rebirth, sacrificing the current run for prestige currency and keeping permanent bonuses.
- [ ] Permanent bonuses are purchasable and measurably affect a subsequent run.
- [ ] A second run reaches early milestones faster than the first (pacing goal demonstrably met).
- [ ] At least one each of: a gated crop unlock, an automation upgrade, and a new zone/field.
- [ ] Achievements fire on their conditions and persist across sessions and restarts.
- [ ] Optional flavor (if implemented) never creates a fail state or punishes absence, and is toggleable.
- [ ] All new tunables live in data files; the migration applies non-destructively.
- [ ] New systems keep the engine deterministic and pass unit tests.

## Verification steps

1. Play a run via the engine to a rebirth, rebirth, and confirm currency/bonuses carry while the run resets.
2. Buy a permanent bonus and confirm the next run is faster (compare time-to-milestone with and without it).
3. Trigger each unlock type and each of a few achievements; confirm persistence across restart.
4. If flavor is enabled, confirm it is reproducible under a fixed seed and never produces a loss the player cannot recover from.
5. Apply the migration to an existing pre-Task-4 save and confirm it upgrades cleanly.

## If blocked

If the pacing goal cannot be met with the current economy (e.g. rebirth feels like a setback rather than acceleration no matter the tuning), stop and report it with the numbers you tried — this is a design-critical property, not a detail. Do not ship a prestige loop that makes repeated play feel worse.
