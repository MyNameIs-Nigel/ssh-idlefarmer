---
name: deployment-docker
description: Package ssh-idlefarmer as a hardened, minimal Docker image built from a static cgo-free Go binary, run it as a non-root user on a configurable port (default 22, mapped from a high internal port in Docker), persist the SQLite database and SSH host key on named volumes so state and host identity survive restarts and redeploys, orchestrate it with Docker Compose, wire graceful shutdown and a restart policy, apply container hardening, and document backups.
---

# Task 6 — Deployment (Docker)

## Objective

Make the finished game deployable with a single command and durable across restarts and redeploys, without weakening any of the security properties built in Tasks 1–5. This task produces the container image, the runtime configuration, the persistence wiring (volumes for the database and the host key), and the operational guidance (graceful shutdown, restarts, backups). It assumes the application is complete and focuses purely on packaging and running it.

## Prerequisites

- Tasks 1–5 complete: a working, persistent, playable SSH game with key-only auth, a cgo-free SQLite store, a persisted host key path, configurable port/limits via environment, and a SIGTERM-driven save flush.

## Scope

### In scope
- A multi-stage Dockerfile producing a minimal image from a static binary.
- Non-root runtime and the port-22 strategy.
- Named volumes for the SQLite database and the SSH host key.
- A Docker Compose definition for one-command operation.
- Graceful shutdown and restart policy.
- Container hardening.
- `.dockerignore`, `.gitignore` review, and backup/restore guidance.

### Out of scope
- Application/game changes — those belong to Tasks 1–5. If deployment reveals an app bug, fix it in the owning task.
- Multi-host orchestration, TLS termination, or cloud-specific tooling (note as future work if relevant).

## Requirements

### Image build (multi-stage, minimal, static)
- Use a **multi-stage** Dockerfile: a build stage with the Go toolchain that compiles a **statically linked, cgo-free** binary (relying on the pure-Go SQLite driver chosen in Task 2), and a final stage containing essentially just the binary and its embedded content.
- Base the final stage on a **minimal** image (e.g. distroless static or scratch). Because the build is cgo-free, no libc or shell is needed in the runtime image — which also shrinks the attack surface.
- Build content files (Task 3/4 `data/`) into the binary via `embed` so the runtime image needs no extra content files.
- Pin the toolchain and base image to specific versions for reproducible builds.

### Non-root runtime & the port-22 strategy
Constraint #4: default port 22, changeable for Docker. Reconcile that with running as non-root (ports below 1024 are privileged):
- The application's **own default listen port is 22** (per Task 1), configurable via its environment variable.
- In the container, run the process as a **non-root user** and have it **listen on a high, unprivileged port internally** (e.g. 2222) by setting that environment variable, then **map the host's port to it** (publish `host:22 -> container:2222`, or whatever host port the operator prefers). The Docker daemon handles the privileged host-side bind; the app never needs root.
- Document both the app's env override and the Compose port mapping so an operator can expose the game on host port 22 (or any other) while the process stays unprivileged. (If an operator insists on the container binding 22 directly, document the `CAP_NET_BIND_SERVICE` alternative, but default to the high-port-plus-mapping approach.)

### Persistence — volumes (critical)
- Mount a **named volume for the SQLite database** at the path Task 2 reads from, so all player saves survive container restarts and image redeploys.
- Mount a **named volume (or the same volume) for the SSH host key** at the path Task 1 reads from, so the host key is generated once and persists. Without this, every redeploy regenerates the host key and returning players get host-key-changed warnings — defeating the persistence work.
- Ensure the volume paths are writable by the non-root runtime user.
- Never bake the host key or database into the image; they live only on the volumes.

### Compose
- Provide a `docker-compose.yml` defining the service, the port mapping, the named volumes, the environment configuration, the restart policy, and a stop grace period long enough for the save flush.
- The goal is `docker compose up` → the game is reachable over SSH; `docker compose down` → state persists on the volumes; bringing it back up resumes cleanly.

### Graceful shutdown & restarts
- Ensure Docker delivers **SIGTERM** on stop and that the app's Task 2 flush-on-shutdown runs before the container exits. Set a **stop grace period** in Compose comfortably above the worst-case flush time so saves are never cut off.
- Set a **restart policy** (e.g. unless-stopped) so the game comes back after crashes or host reboots.
- Confirm a redeploy (down, rebuild, up) loses no in-flight progress thanks to the flush plus the database volume.

### Container hardening
- Run as a **non-root** user (above).
- Where feasible, run with a **read-only root filesystem**, granting write access only to the volume mount paths (and a tmpfs for any scratch needs). A scratch/distroless image with no shell helps here.
- **Drop all Linux capabilities** the process does not need; add back only what is strictly required (none, given the high-port approach).
- Add `no-new-privileges`.
- Set sensible **resource limits** (memory, and CPU if appropriate) so a traffic spike on the open port cannot exhaust the host. These complement the per-key/global session caps and rate limiting from Task 1.
- Keep the published surface minimal: expose only the single SSH port.

### Repo hygiene
- Add a `.dockerignore` excluding the database, host key, local config, VCS metadata, and build caches from the build context.
- Re-confirm `.gitignore` (from Task 1) excludes the host key, database, and any secrets — none of these should ever reach the repo or the image.

### Backups & restore
- Document a backup procedure: the entire game state is the SQLite database file on its volume, so backups are a matter of snapshotting/copying that volume or the file (taking WAL mode into account — prefer a consistent copy, e.g. via the database's backup mechanism or while briefly quiesced).
- Document restore: replacing the database file/volume and restarting. Note that keeping the **host key** volume intact across a restore preserves client trust.

### Documentation
- Update the README with: how to build the image, how to run via Compose, every environment variable and its default, the port-mapping explanation, the volume layout, and the backup/restore steps.

## Security considerations

- The deployment must not undo Task 1–5 guarantees: non-root, minimal image, dropped capabilities, read-only FS, and resource limits keep the open SSH port from becoming a liability.
- Secrets (host key) and all player data live only on volumes, never in the image or the repo.
- A persisted host key is both an operability and a trust requirement — treat the host-key volume as essential, not optional.

## Suggested artifacts produced by this task

- `Dockerfile` (multi-stage, static, minimal final image).
- `docker-compose.yml` (service, ports, volumes, env, restart policy, stop grace period).
- `.dockerignore`.
- README deployment section (build, run, env, volumes, backups/restore).

## Acceptance criteria

- [ ] `docker compose up` builds and runs the game; it is reachable over SSH on the mapped host port.
- [ ] The process runs as a non-root user and binds an unprivileged port inside the container.
- [ ] Player saves persist across `docker compose down`/`up` and across an image rebuild/redeploy (database volume).
- [ ] The SSH host key persists across restarts and redeploys (host-key volume); no host-key-changed warning on reconnect.
- [ ] SIGTERM triggers the save flush within the stop grace period; no in-flight progress is lost on stop/redeploy.
- [ ] The container runs hardened: minimal image, read-only root FS where feasible, dropped capabilities, no-new-privileges, resource limits.
- [ ] The host key, database, and secrets appear in neither the image nor the repo.
- [ ] The README documents build, run, env vars, port mapping, volumes, and backup/restore.

## Verification steps

1. Build and `docker compose up`; connect over SSH on the mapped host port and play.
2. `docker compose down` and back up; confirm saves persisted.
3. Rebuild the image (simulating a redeploy) and bring it up; confirm saves and host key persisted and no host-key warning appears.
4. Disconnect a mid-session player, `docker compose stop`, and confirm via logs that the flush ran within the grace period; restart and confirm no progress lost.
5. Inspect the running container to confirm non-root user, dropped capabilities, read-only FS, and resource limits.
6. Confirm the image and repo contain no host key or database.
7. Perform a backup-and-restore dry run of the database volume and confirm the game resumes from the restored state.

## If blocked

If a hardening requirement conflicts with running the game (for example a read-only root filesystem breaks a needed write path, or the high-port mapping cannot expose host port 22 in the target environment), stop and report the specific conflict with options rather than running the container as root or disabling persistence. Root-in-container and a non-persistent host key are both non-acceptable shortcuts.
