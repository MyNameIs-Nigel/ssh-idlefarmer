---
name: project-overview
description: The story and shape of ssh-idlefarmer — read this before starting any task file. Explains what the game is and why, the architecture at a glance, the cross-cutting invariants every task must respect, the shared vocabulary, the locked design decisions, the hard constraints, and the build order and dependencies across the six task files.
---

# ssh-idlefarmer — Project Overview

> Read this first. The six task files (`01`–`06`) tell you *what to build* in each layer. This file tells you *what the project is, how the pieces relate, and the rules that hold across all of them*. If a task instruction ever seems to conflict with an invariant below, stop and reconcile — the invariants are load-bearing.

## What this is

`ssh-idlefarmer` is a cozy idle farming game that you play by SSH-ing into a server. There is no website, no client to install, no account to register — you `ssh` to the host and you are immediately in your farm. Your SSH key *is* your identity. You plant crops, wait, harvest them for coins, buy more land, and eventually rebirth: trading your current progress for permanent bonuses that make every future run faster.

The whole point is **low stakes**. You can log in for ninety seconds, harvest what grew while you were gone, plant the next batch, and leave. Nothing punishes you. Nothing demands your attention. The farm keeps moving forward whether you're watching or not, so coming back always feels rewarding rather than like catching up on chores.

## The player's story (the loop)

1. `ssh farm.example.com` — your key authenticates you and drops you straight into your farm. No shell, no menu maze.
2. Plant a crop in an open plot. Crops have different grow times and payoffs: fast cheap ones for a steady trickle, slow expensive ones for patience, a risky one for a gamble.
3. Leave. Time passes in the real world; your crops finish growing whether you're connected or not.
4. Come back. Harvest the ready crops for coins. Buy more plots (each costs more than the last), unlock better crops, buy tools — including an auto-harvester that tends the farm while you're away.
5. When a run is mature, **rebirth**: reset the farm in exchange for prestige currency, spent on permanent multipliers. The next run reaches the old ceiling faster and climbs past it. This is the long arc that keeps the game alive for weeks.

Connecting with a *different* SSH username opens a *separate* farm under the same key — a second save slot — so a single player can keep parallel farms.

## How it's built

A single static Go binary, served over SSH, backed by a single SQLite file. Chosen stack:

- **Go** — for the Charm ecosystem, which is almost purpose-built for "SSH into a terminal app."
- **Wish** — an in-process SSH server. The connection lands directly in the game; there is no real shell behind it.
- **Bubble Tea + Lip Gloss + Bubbles** — the terminal UI (Elm-style model/update/view, styling, components).
- **SQLite via `modernc.org/sqlite`** (pure Go, no cgo) — durable storage and a fully static binary, which keeps the Docker image trivial.
- **Docker** — the deployment unit; host port 22 maps to an unprivileged port inside a non-root container.

### Architecture at a glance — the path of one connection

```
ssh client
   │  public key (sole auth factor)        username string (save selector)
   ▼
[Task 1] Wish SSH server
   │  - trust-on-first-use: accept any valid key; fingerprint = identity
   │  - require a PTY; sanitize username → slot
   ▼
   SessionIdentity { fingerprint, slot, publicKey }
   ▼
[Task 2] resolve save (fingerprint, slot) → SAVE ACTOR (one goroutine owns this save)
   │  - sole writer to this save's rows; serializes all mutation
   ▼
[Task 5] Bubble Tea program (per session)        [Task 3/4] game engine runs INSIDE the actor
   │  renders state, captures keys                 - validates intents against authoritative state
   └────────── intents ───────────────────────────▶ - derives growth from timestamps
              (plant, harvest, buy, rebirth)        - applies economy + progression rules
   ◀───────── new state / outcomes ────────────────┘
                                                     │ write-through + periodic snapshot + flush
                                                     ▼
                                              [Task 2] SQLite file (on a Docker volume)
```

The key idea to internalize: the **terminal is a dumb display**. It sends *intents* and renders whatever the server returns. All authority — funds, ownership, timing, randomness — lives server-side in the engine, inside the single-owner actor.

## Invariants every task must respect

These hold across all six files. Violating one in a single task breaks the system even if that task's own acceptance criteria pass.

- **Authoritative server, dumb client.** The TUI never computes or mutates game state. It emits intents; the engine validates and applies them. Never trust a client-implied outcome — recompute from authoritative state. This is also the entire anti-cheat strategy.
- **Time is derived, not ticked.** A plot's readiness is `now >= planted_at + grow_time`. Nothing writes to storage every second. The live UI tick only re-renders; a dropped tick never loses progress. Offline progress is computed on attach from elapsed time.
- **Save isolation is absolute.** A save is reachable *only* by the key whose fingerprint created it. The username selects among *your own* saves; it can never reach another person's data. Two different keys sending the same username get two isolated saves. This is enforced at three layers: sanitization (Task 1), the `(fingerprint, slot)` unique key (Task 2), and the UI only ever rendering the connecting save (Task 5).
- **One writer per save.** Each active save is owned by a single actor goroutine (Task 2). The engine runs inside it. No save is ever written by two sessions at once.
- **No shell, ever.** The Wish handler is the only code path a connection can reach. There is nothing to escape into.
- **Untrusted strings in *and* out.** Sanitize the username at the door (Task 1); escape any user-influenced string on output (Task 5). Both sides close the terminal-escape-injection vector.
- **Money is integer.** Smallest-unit integers everywhere — no floats — to avoid rounding drift and exploits.
- **Content is data, not code.** Crop economics and balance live in TOML loaded at startup. Rebalancing never requires recompiling or a schema migration.
- **State must survive a stop.** On `SIGTERM` the server flushes every active save before exiting. This is what makes Docker redeploys non-destructive — every task that holds state in memory must participate in the flush.

## Shared vocabulary

- **Fingerprint** — SHA256 fingerprint of the connecting public key; the player's identity.
- **Slot** — the sanitized SSH username string; selects which save under a fingerprint. A save's identity is `(fingerprint, slot)`.
- **Save actor** — the single goroutine that owns one active save's authoritative in-memory state and is its only writer.
- **Intent** — a validated player action sent from the TUI to the actor (plant, harvest, buy plot, buy tool, rebirth).
- **Content files** — the TOML (`crops.toml`, `balance.toml`) holding crop economics and tunable balance, embedded with optional override.
- **Tick** — the live UI re-render interval; it re-renders only, it does not advance authoritative state.
- **TOFU** — trust-on-first-use: the first time a key connects, its save is created; no separate registration.
- **Rebirth / prestige** — resetting a run for permanent multipliers that make future runs faster.

## Locked design decisions

These were decided up front; treat them as settled unless the project owner changes them.

- **Auth is public-key only, TOFU.** No passwords, no keyboard-interactive, no registration step.
- **Default save = the client's default username.** When a player just runs `ssh host`, their client sends their local username, which becomes their default slot. Any other username opens another slot under the same key. An empty/blank result falls back to `IDLEFARM_DEFAULT_SLOT` (`default`).
- **Second connection to an active save wins (takeover).** The newer session attaches to the existing actor and the older session is gently disconnected, rather than refusing the new connection. (Flip to "refuse" only if the owner requests it.)
- **Pure-Go SQLite (`modernc.org/sqlite`).** Enables `CGO_ENABLED=0` static builds and a minimal image.
- **Port 22 by default via mapping.** The app binds an unprivileged port inside the container; the host maps 22 to it, so the container never needs root or `CAP_NET_BIND_SERVICE`.
- **Crops finish growing offline and wait for a manual harvest.** Automatic offline harvesting is an unlockable tool, not default behavior.

## Hard constraints

- Repo: `github.com/mynameis-nigel/ssh-idlefarmer`.
- Source in Go; deployable via Docker.
- Default public port 22, changeable via the Docker host-side port mapping.
- Config via `IDLEFARM_`-prefixed environment variables (the full table lives in Task 1).
- Every task file carries `name`/`description` frontmatter.

## Task map & build order

Build in order; each task assumes the seams created by the ones before it.

| # | File | Builds | Depends on |
| --- | --- | --- | --- |
| 1 | `01-ssh-server-foundation` | Repo scaffold, Wish server, pubkey/TOFU auth, fingerprint+slot identity, host-key persistence, config, connection limits, shutdown hook | — |
| 2 | `02-persistence-and-data-model` | SQLite schema + migrations, `(fingerprint, slot)` save resolution, the save-actor concurrency model, write cadence, shutdown flush | 1 |
| 3 | `03-game-engine-and-time-model` | Crops, plots, wallet, timestamp-driven growth, offline progression, intent validation, content loading, starter economy | 1, 2 |
| 4 | `04-progression-and-rebirth` | Geometric plot expansion, unlocks, tools (incl. offline auto-harvester), rebirth/prestige formula, achievements, balance | 2, 3 |
| 5 | `05-tui-and-gameplay-ux` | Bubble Tea screens, keymap, styling, live tick, resize handling, onboarding, output escaping | 1, 2, 3, 4 |
| 6 | `06-docker-deployment` | Multi-stage non-root image, 22→2222 mapping, volumes for DB + host key, SIGTERM flush contract, compose, hardening, backups | 1, 2 |

Dependency shape: **1 → 2** is the spine. **3** and **4** are the simulation, layered on **2**. **5** sits on top of everything and is the only player-facing layer. **6** operationalizes **1** and **2** and can be drafted in parallel once those seams exist.

## How to use this with the task files

Read this overview, then open the specific task you're implementing. The task file is authoritative for *its* requirements and acceptance criteria; this file is authoritative for the *cross-cutting* rules and the shared vocabulary. When a task references a term defined here (fingerprint, slot, actor, intent), use the meaning here. When you make an implementation choice, check it against the invariants above before committing to it.
