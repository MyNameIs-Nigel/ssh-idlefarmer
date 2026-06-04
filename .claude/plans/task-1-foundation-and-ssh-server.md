---
name: foundation-ssh-server
description: Stand up the ssh-idlefarmer repository, Go module, and a Wish-based SSH server that authenticates solely on the client's public key, lands the user directly in the game (never a shell), persists its host key, requires an interactive PTY, and renders a placeholder screen proving the connection loop end to end.
---

# Task 1 — Project Foundation & SSH Server Skeleton

## Objective

Create the repository skeleton and a working SSH entry point. By the end of this task, a person can `ssh` into the server with nothing but an SSH key, get authenticated by that key alone, and see a placeholder terminal screen that confirms their identity and selected save slot. No game logic yet — this task proves the hardest plumbing (SSH-served TUI, key-only auth, no shell) is solid before anything is built on top of it.

This is the foundation checkpoint. Everything else depends on it.

## Prerequisites

None. This is the first task.

## Scope

### In scope
- Repository and Go module initialization.
- Directory/package layout for the whole project.
- A Wish-based SSH server listening on a configurable port (default 22).
- Public-key-only authentication (no passwords, no pre-provisioned OS users).
- Derivation of a stable identity from the client's public key.
- Persistent SSH host key.
- PTY requirement and window-resize handling.
- A baseline security posture (no shell fall-through, idle timeout, session limits).
- A placeholder Bubble Tea program that displays the client's key fingerprint and chosen save name.
- Structured logging and graceful startup/shutdown wiring (the hooks; full save-flush logic lands in Task 2).

### Out of scope (deferred)
- Any persistence to disk — Task 2.
- Any farming/economy logic — Task 3.
- The real UI — Task 5.
- Dockerfile and deployment — Task 6.

## Requirements

### Repository & module
- Initialize the Go module as `github.com/mynameis-nigel/ssh-idlefarmer`.
- Target a current, supported Go release; record the version in `go.mod`.
- Add a `README.md` describing the project in one paragraph and how to run it locally.
- Add a `LICENSE` (choose one; note the choice in the README).
- Add a `.gitignore` that excludes the host key, the database file, build artifacts, and any local config containing secrets. The host key must never be committed.

### Project layout
Establish this layout (no implementation yet, just the structure and package boundaries):

```
ssh-idlefarmer/
  cmd/
    ssh-idlefarmer/        # main entrypoint: config load, server start, signal handling
  internal/
    server/                # SSH server setup, middleware, auth, PTY handling
    identity/              # public-key fingerprinting, save-name resolution & sanitization
    config/                # configuration loading (env first)
    log/                   # structured logging setup
    tui/                   # Bubble Tea program (placeholder for now)
  data/                    # game content files (added in Task 3)
  tasks/                   # these task files
  README.md
  go.mod
```

Keep all non-entrypoint code under `internal/` so package boundaries stay private to this module.

### SSH server (Wish)
- Use the Charm `wish` library on top of `golang.org/x/crypto/ssh` to build an in-process SSH server. The session handler *is* the game — there is no real shell behind it and no way to reach one.
- Compose the server from middleware: at minimum a logging middleware, a Bubble Tea middleware that attaches the TUI program to the session, and the auth handler below.
- The listen address and port come from configuration. Default port is **22**. It must be overridable (this is what Docker will use to run on a high internal port).

### Authentication — public key only
- Accept **any** public key. There is no password auth and no allow-list of users; the first time a key connects it is implicitly "registered" (persistence comes in Task 2), and every subsequent connection with the same key is the same identity.
- Reject any non-public-key auth method.
- The auth decision is based purely on the presence of a valid public key — not on the SSH username.

### Identity & save-name resolution
This is the core of constraint #6 and must be implemented carefully:
- **Identity = the public key.** Derive a stable fingerprint (SHA-256 of the key) and treat that as the account identifier. Two connections share an account if and only if they present the same key.
- **The SSH username selects a save slot *within* that identity.** When a user runs `ssh host` with no explicit user, the SSH client sends their local OS username; treat that as the default save name. When they run `ssh someothername@host`, `someothername` selects a different save **owned by the same key**.
- Saves are therefore keyed by the pair *(key fingerprint, save name)*. A given key can own many saves; a save name on its own means nothing without the key.
- **Sanitize the save name** before it is used anywhere: enforce an allowed character set (lowercase letters, digits, hyphen, underscore), a length bound (e.g. 1–32 characters), and reject or normalize anything outside it. This prevents path traversal, malformed keys, and display problems. Never interpolate it into a query string — Task 2's storage layer will use parameterized access.
- A different person using the same username (e.g. two people who both `ssh host` as `nigel`) must get **different** saves, because their keys differ. Make sure the resolution logic cannot collapse two keys into one save.

### Host key persistence
- On startup, load the server's SSH host key from a configured path. If it does not exist, generate one and write it to that path, then reuse it on every subsequent start.
- The host key path must default to a location that Task 6 will back with a Docker volume.
- The key file must be created with restrictive permissions and must be excluded by `.gitignore`.

### PTY & terminal handling
- Require an interactive PTY. If a client connects without one (e.g. `ssh host somecommand`, or a non-interactive/scripted session), respond with a short friendly message explaining the game needs an interactive terminal, then close the session cleanly.
- Read the initial terminal size from the PTY request and subscribe to window-change events so the UI (Task 5) can re-layout on resize.

### Security baseline
- Guarantee there is no path from a session to a shell or to arbitrary command execution. The only thing a session can do is run the TUI program.
- Apply an **idle timeout**: disconnect sessions that have sent no input for a configured period.
- Apply a **per-key concurrent session cap** and a **global connection cap**, both configurable, to blunt abuse and resource exhaustion.
- Apply lightweight **connection rate limiting** so a single source cannot open connections in a tight loop.
- Do not log private key material, full public keys at info level, or anything else sensitive; logging the fingerprint is fine.

### Configuration
Load configuration from environment variables (twelve-factor style), with sane defaults. At minimum:
- listen host and port (default port 22),
- host key path,
- idle timeout,
- per-key session cap and global connection cap,
- log level/format.
Document every variable in the README.

### Logging & lifecycle
- Use Go's standard structured logging (`log/slog`) with a configurable level and format.
- Wire signal handling in `main`: on SIGINT/SIGTERM, stop accepting new connections and shut down the server cleanly. (Flushing player state on shutdown is added in Task 2; leave a clear hook for it.)

### Placeholder TUI
- A minimal Bubble Tea program that, on connect, shows: a title, the connecting key's fingerprint, the resolved save name, and a line telling the user how to quit (e.g. `q` or `Ctrl+C`).
- It should redraw correctly on window resize.
- This exists only to prove the full path (SSH → auth → identity → PTY → TUI → quit) works.

## Security considerations (summary)

- Key-only auth with no user provisioning is the intended design, but it means *anyone* can connect — so the session sandbox (no shell), idle timeouts, and session/connection caps are load-bearing, not optional.
- Persisted host key prevents host-key-mismatch warnings and trust-on-first-use churn.
- Save-name sanitization plus the (key, save) ownership rule are what keep one player out of another's data.

## Suggested artifacts produced by this task

- `go.mod`, `README.md`, `LICENSE`, `.gitignore`
- `cmd/ssh-idlefarmer/` entrypoint
- `internal/server/`, `internal/identity/`, `internal/config/`, `internal/log/`, `internal/tui/` (placeholder)

## Acceptance criteria

- [ ] `github.com/mynameis-nigel/ssh-idlefarmer` builds from a clean checkout.
- [ ] Connecting with `ssh -p <port> host` (no username) authenticates on the key alone and shows the placeholder screen with the default save name.
- [ ] Connecting with `ssh -p <port> other@host` shows a different save name while using the same identity.
- [ ] Two different keys using the same username resolve to different saves.
- [ ] Password auth and other non-key methods are rejected.
- [ ] A non-interactive connection (no PTY) is declined with a friendly message and closed.
- [ ] The host key is generated once and reused across restarts; no host-key-changed warning on reconnect.
- [ ] Idle sessions time out; the per-key and global session caps are enforced.
- [ ] The placeholder UI re-lays-out on terminal resize and exits cleanly.
- [ ] The host key, database path, and build artifacts are git-ignored.

## Verification steps

1. Build and run locally on a non-privileged port (e.g. 2222) to avoid needing root during development.
2. Connect with and without an explicit username; confirm save-name resolution and fingerprint display.
3. Connect with a second key and the same username; confirm a distinct save.
4. Restart the server and reconnect; confirm no host-key warning.
5. Attempt a non-interactive connection and confirm graceful refusal.
6. Open more sessions than the per-key cap allows and confirm the cap holds.

## If blocked

If any requirement cannot be met as written — for example if the chosen SSH library cannot express key-only auth, per-session PTY handling, or the no-shell guarantee — stop and report the specific blocker and the options considered rather than substituting a weaker security model. Do not fall back to OS-user accounts or password auth without explicit approval.
