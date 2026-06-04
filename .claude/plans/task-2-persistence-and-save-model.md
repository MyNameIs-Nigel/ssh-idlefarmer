---
name: persistence-save-model
description: Add a SQLite-backed persistence layer (pure-Go, cgo-free) for ssh-idlefarmer with a versioned schema and migrations, store accounts by public-key fingerprint and saves keyed by (fingerprint, save name), enforce the ownership model and a single-active-session-per-save concurrency policy, and wire autosave plus a save-flush on graceful shutdown so no progress is lost across disconnects or redeploys.
---

# Task 2 — Persistence Layer, Identity Storage & Save Model

## Objective

Give the game durable state. After this task, a player who disconnects and reconnects — even after a server restart — returns to exactly the save they left, identified by their key and chosen save name. This task implements the storage engine, the data model, the ownership and concurrency rules, and the save lifecycle (load on connect, autosave, flush on shutdown). It does not yet contain any farming logic; the stored state is a small, well-defined record that Task 3 will populate.

## Prerequisites

- Task 1 complete: a working SSH server with key-only auth, identity fingerprinting, and resolved save names.

## Scope

### In scope
- Choice and integration of an embedded SQLite engine.
- A versioned schema and a forward migration mechanism.
- Tables for accounts (by key fingerprint) and saves (by fingerprint + save name).
- The storage API the rest of the game uses (load save, create save, persist save).
- The (key, save-name) ownership model, enforced in the storage layer.
- A single-active-session-per-save concurrency policy with an in-memory session registry.
- Save lifecycle: load-or-create on connect, periodic autosave, save-on-disconnect, flush-on-shutdown.
- Connecting the Task 1 session flow to load/create the player's save and show a real (if still minimal) "loaded" screen.

### Out of scope (deferred)
- The contents of the farm state beyond a minimal placeholder — Task 3 defines the real fields.
- Crop/economy logic — Task 3.
- Rebirth/progression fields — Task 4 will extend the schema via a new migration.
- UI screens — Task 5.

## Requirements

### Storage engine
- Use an **embedded, file-based SQLite** database. Prefer a **pure-Go, cgo-free** driver (e.g. `modernc.org/sqlite`) so the project can be compiled to a fully static binary and shipped in a minimal container in Task 6. Avoid cgo-based drivers, which complicate the Docker build.
- Open the database in **WAL mode** so reads do not block writes.
- The database file path comes from configuration and defaults to a location Task 6 will back with a volume.
- Serialize writes so concurrent sessions cannot corrupt state. Acceptable approaches: a single owning goroutine that processes write requests, or a guarding mutex around writes. Document the choice.

### Schema & migrations
- Maintain a **schema version** in the database (a dedicated table or pragma).
- Implement **forward migrations** applied automatically on startup: the server checks the current version and runs any pending migration steps in order. This lets Task 4 (and future work) add columns/tables without wiping existing saves.
- Migrations must be idempotent and ordered. Never destructive to existing player data without an explicit, separately gated step.

### Data model
Describe the schema at the field level (the storage layer owns the SQL; do not hardcode SQL elsewhere):

- **Account**, one row per identity:
  - key fingerprint (primary identifier, unique),
  - first-seen timestamp,
  - last-seen timestamp,
  - optionally a stored copy of the public key for auditing.
- **Save**, one row per *(fingerprint, save name)*:
  - owning key fingerprint (foreign key to account),
  - save name (sanitized, as resolved in Task 1),
  - created timestamp,
  - last-saved / last-active timestamp (the basis for offline catch-up in Task 3),
  - a serialized blob or set of columns holding the game state (kept minimal in this task; Task 3 defines the real shape),
  - a schema/state version tag if you version save payloads independently.
  - Uniqueness constraint on (fingerprint, save name).

The last-active timestamp is important: Task 3's offline-growth calculation reads it to know how much time elapsed while the player was away.

### Ownership model (enforced here)
- Every storage operation that touches a save **takes the key fingerprint as a parameter** and only ever returns or mutates saves owned by that fingerprint.
- There is no API that can fetch a save by name alone. A request for save `farmA` from key X can never return key Y's `farmA`.
- All access is via **parameterized queries**; the (already sanitized) save name is never string-concatenated into SQL.

### Concurrency policy — same save opened twice
A player can connect from two terminals into the same (key, save name) at once. Define and enforce a clear policy:
- **Recommended: single active session per save.** Maintain an in-memory registry of currently-open saves. When a session tries to open a save that is already active, refuse the second session with a friendly message ("This farm is already open in another session"), or — if you prefer — take over and disconnect the older session. Pick one and document it; the simpler and safer default is to refuse the newcomer.
- The registry is in-memory only, so a process restart naturally clears any stale locks. Ensure a save is removed from the registry on disconnect and on crash-safe cleanup paths.
- Different saves under the same key may be open simultaneously without restriction.

### Save lifecycle
- **On connect:** resolve identity + save name (Task 1), then load the matching save, creating it if it does not exist (first time a key uses a given save name). Update the account's last-seen and the save's last-active timestamps.
- **Autosave:** persist the active save on a configurable interval while the session is connected.
- **On disconnect:** persist the save and release it from the active-session registry.
- **On graceful shutdown (SIGTERM, from a Docker stop/redeploy):** stop accepting new connections, then flush every active save to the database before exiting. This fulfills the shutdown hook left in Task 1 and is what prevents redeploys from losing in-flight progress.

### Configuration additions
- database file path (with a volume-friendly default),
- autosave interval,
- concurrency policy toggle if you implement both refuse/takeover behaviors.
Document these in the README alongside the Task 1 variables.

## Security considerations

- Parameterized access plus the fingerprint-scoped API are the technical guarantees behind the ownership model — keep them airtight.
- Restrict the database file's permissions; it contains all player state.
- Do not log save contents or full keys; the fingerprint and save name are sufficient for diagnostics.
- Treat the save name as untrusted input even though Task 1 sanitized it — defense in depth.

## Suggested artifacts produced by this task

- `internal/store/` — database open/init, migrations, the fingerprint-scoped storage API, the active-session registry.
- Migration definitions (versioned, ordered).
- Updates to `internal/server/` to load/create saves on connect and to flush on shutdown.

## Acceptance criteria

- [ ] State survives disconnect/reconnect and a full server restart.
- [ ] A given key can hold multiple distinct saves selected by username, fully isolated from each other.
- [ ] No storage call can return a save belonging to a different key, even with an identical save name.
- [ ] Opening a save that is already active is handled per the chosen concurrency policy.
- [ ] Autosave persists progress on the configured interval; disconnect persists immediately.
- [ ] SIGTERM flushes all active saves before exit (verify no progress lost on redeploy).
- [ ] Migrations apply automatically on startup and are non-destructive to existing data.
- [ ] The database uses WAL mode and a cgo-free driver (static-build friendly).

## Verification steps

1. Connect, mutate the placeholder state, disconnect, reconnect — confirm it persisted.
2. Restart the server and reconnect — confirm persistence across process lifetime.
3. Use two save names under one key — confirm isolation.
4. Use the same save name from two different keys — confirm two separate saves.
5. Open the same save twice — confirm the concurrency policy fires.
6. Send SIGTERM while a session is mid-change — confirm the change is flushed and present on reconnect.
7. Bump the schema version with a trivial migration — confirm it applies cleanly to an existing database.

## If blocked

If the chosen driver cannot run cgo-free, or WAL mode or the migration approach cannot be made to work as specified, stop and report it with the alternatives and tradeoffs (especially any that would force a cgo build and thus complicate Task 6). Do not silently switch to a non-static driver or to an unversioned schema.
