# CLAUDE.md

Guidance for AI agents working in this repository.

## What this is

**ssh-idlefarmer** is an idle farming game played entirely over SSH. There is no web client and no password auth: the player's SSH public key *is* their account, and the SSH username selects a save slot under that key. A Bubble Tea TUI is the only thing a session can reach — there is no shell.

```bash
ssh farm.example.com            # default save slot
ssh otherfarm@farm.example.com  # second farm under the same key
```

## Stack

- **Go 1.26+** (pure Go, `CGO_ENABLED=0`)
- **charm.land/wish/v2** — SSH server framework (wraps `github.com/charmbracelet/ssh`)
- **charm.land/bubbletea/v2** + **lipgloss/v2** — terminal UI
- **modernc.org/sqlite** — cgo-free SQLite driver
- **Docker** — distroless static image, deployed via `docker-compose.yml`

## Commands

```bash
go build -o bin/ssh-idlefarmer ./cmd/ssh-idlefarmer   # build
go test ./...                                          # run all tests
go vet ./...                                           # vet
docker compose up -d --build                           # deploy (host port 22)
```

Run locally without root by setting `IDLEFARM_LISTEN_PORT=2222`, then connect with
`ssh -p 2222 -o StrictHostKeyChecking=accept-new localhost`.

## Layout

| Path | Purpose |
| --- | --- |
| `cmd/ssh-idlefarmer/main.go` | Entry point: config → store → game manager → SSH server, graceful shutdown |
| `internal/config/` | All settings from `IDLEFARM_*` env vars (see README for full table) |
| `internal/server/` | Wish server, middleware chain, PTY requirement, session limits, shutdown hooks |
| `internal/identity/` | Key fingerprint (`SHA256:...`) + username → save-slot sanitization |
| `internal/game/` | Save lifecycle: `Manager` (attach/detach), per-save actor goroutines, autosave |
| `internal/sim/` | Headless deterministic game engine: state, tick advance, actions, derived values |
| `internal/tui/` | Bubble Tea model/views (Farm, Market, Land, Rebirth, Stats) |
| `internal/store/` | SQLite persistence + migrations |
| `internal/content/` | Loads/validates game content from TOML |
| `data/` | `crops.toml` + `balance.toml`, embedded into the binary via `embed.go` |
| `.claude/plans`, `.claude/qa`, `.claude/completed` | Task-planning workflow (see `.claude/skills/task-router`) |

## Architecture in one pass

1. **Auth**: `wish.WithPublicKeyAuth` accepts any non-nil key (trust-on-first-use). Password auth is rejected. A keyless client fails at the SSH protocol layer before any middleware runs.
2. **Middleware** (composed in `internal/server/server.go`; Wish runs them bottom-up, so the listed order is: logging → rate limit → session caps → `RequirePTY()` → `attachSave()` → Bubble Tea handler).
3. **attachSave** resolves `(fingerprint, slot)`, asks `game.Manager.Attach` for the save, applies offline catch-up, and stores session state on the `ssh.Context`.
4. **Single-writer actors**: each active save is owned by exactly one actor goroutine in `internal/game/actor.go`. Opening the same save twice follows `IDLEFARM_SESSION_POLICY` (`takeover` default, or `refuse`).
5. **Sim vs TUI**: `internal/sim` is the authoritative engine (pure, testable, no I/O); the TUI renders it and forwards player actions. Game state is JSON-serialized into the `saves.state` BLOB.

## Persistence

- One SQLite file (WAL mode, single connection) holds everything. Schema: `accounts` (keyed by fingerprint) and `saves` (PK `(fingerprint, slot)`), defined in `internal/store/migrations.go`. Add new schema changes as **append-only migrations** there.
- Saves are written on autosave (every `IDLEFARM_AUTOSAVE_INTERVAL`, default 30s), on disconnect, and on SIGTERM flush.
- Paths: local default `var/idlefarm.db`; in Docker `/var/lib/idlefarm/idlefarm.db` on the `idlefarm-data` named volume (which also holds the SSH host key, so it survives rebuilds and redeploys).

## Conventions and gotchas

- **Never commit runtime data**: `var/`, `*.db`, and host key files are gitignored. The DB and host key must stay out of git.
- **Middleware order matters** in `server.New()` — last listed runs first on inbound connections. Keep `logging`/`ratelimiter` outermost and `attachSave` adjacent to the Bubble Tea handler.
- **Game balance lives in TOML**, not code. Tweak `data/crops.toml` / `data/balance.toml`; they're embedded at build time but can be overridden at runtime with `IDLEFARM_DATA_DIR`.
- **Sim purity**: keep `internal/sim` free of I/O, logging, and wall-clock reads — it takes timestamps as arguments so offline catch-up and tests stay deterministic.
- **Messages to users use `\r\n`** line endings (raw SSH sessions, no tty cooking) — see `internal/server/pty.go` and the attach-failure messages.
- **Graceful shutdown is load-bearing**: SIGTERM must flush all active saves before exit (`server.RegisterShutdownHook`, compose `stop_grace_period: 45s`). Don't break this path.
- **Docker runtime is hardened**: distroless, non-root (uid 65532), read-only root FS — the only writable paths in the container are the volume and `/tmp`. Code must not write anywhere else.
- Run `go test ./...` after changes; most packages have table-driven tests alongside the code (`*_test.go`).
