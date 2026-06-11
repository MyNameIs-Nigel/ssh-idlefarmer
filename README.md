# ssh-idlefarmer

A cozy idle farming game you play over SSH: connect with your public key, land straight in your farm, plant crops, and come back later to harvest. No website, no client install, no password — your SSH key is your identity and the SSH username picks which save slot you open under that key. Crops keep growing while you're offline, rebirth trades a mature run for permanent bonuses, and nothing ever punishes you for being away.

Licensed under the [MIT License](LICENSE).

## Play

```bash
ssh farm.example.com            # your default save slot
ssh otherfarm@farm.example.com  # a second farm under the same key
```

- **Farm** — plant on empty plots, watch crops grow live, harvest when ready. Upgrade individual plots with auto-harvest and auto-sow. Shoo visiting critters for pocket change.
- **Market** — buy run-scoped multipliers (Fertilizer, Merchant's Scale), Hardier Strain upgrades for risky crops, and zones like the Greenhouse.
- **Land** — buy more plots and browse the full seed catalog (fast / slow / risky, with salvage values for risky crops).
- **Rebirth** — reset a mature run for **Starseeds** (cosmic currency earned from run earnings). Preview what the next rebirth unlocks.
- **Progress** — unlocked after your first rebirth; spend Starseeds on permanent upgrades (Rich Soil, Gift Magnet, Stargazer's Almanac, and more).
- **Stats** — lifetime numbers, achievements, farm naming, and lucky-find toggle.

While online you may see random events (Market Day, Bumper Demand, Warm Front), gift parcels at the gate (`g` to redeem), golden harvests, moon phases (full moon boosts Moonberry), and headlines in **The Daily Furrow**. If you're broke with empty plots, the cheapest seed plants for free.

Press `?` in game for keys. Quit any time — progress autosaves, and the farm keeps moving without you.

## Run locally

Requires Go 1.26+.

```powershell
# Build
go build -o bin/ssh-idlefarmer ./cmd/ssh-idlefarmer

# Listen on a non-privileged port (no root needed)
$env:IDLEFARM_LISTEN_PORT = "2222"
bin\ssh-idlefarmer.exe
```

In another terminal:

```bash
ssh -p 2222 -o StrictHostKeyChecking=accept-new localhost
```

Run the tests with `go test ./...`.

## Run with Docker (recommended for servers)

```bash
docker compose up -d --build
```

That's the whole deployment: the game is reachable on host port 22, saves and the SSH host key live on the `idlefarm-data` named volume, and `docker compose down`/`up` (or a rebuild) loses nothing.

How it fits together:

- **Image** — multi-stage build producing a static, cgo-free binary on `distroless/static`; no shell, no libc, pinned base images.
- **Non-root + port mapping** — the process runs as `nonroot` and listens on 2222 inside the container; Docker maps host port 22 to it, so nothing ever runs privileged. Change the host side of `ports:` in [docker-compose.yml](docker-compose.yml) to use another port.
- **Volume** — `/var/lib/idlefarm` holds the SQLite database *and* the SSH host key. Without the host key persisting, every redeploy would show players a scary host-key-changed warning.
- **Graceful shutdown** — Docker's stop sends SIGTERM; the server flushes every active save before exiting. `stop_grace_period: 45s` gives it room.
- **Hardening** — read-only root filesystem, all capabilities dropped, `no-new-privileges`, memory/CPU limits, single published port.

### Backups & restore

All game state is one SQLite file on the volume. To back up consistently, stop the server briefly (the flush guarantees a clean file), copy the volume, and start it again:

```bash
docker compose stop
docker run --rm -v idlefarm-data:/data -v "$PWD":/backup alpine \
    tar czf /backup/idlefarm-backup.tar.gz -C /data .
docker compose start
```

Restore by reversing it (extract into the volume while the server is stopped). Keep the host key file from the same backup so returning players' clients still trust the server. If you can't stop the server, use SQLite's online backup (`sqlite3 idlefarm.db ".backup ..."`) against the WAL-mode database instead of copying the raw file.

## Configuration

All settings use the `IDLEFARM_` prefix:

| Variable | Default | Description |
| --- | --- | --- |
| `IDLEFARM_LISTEN_HOST` | `0.0.0.0` | Bind address |
| `IDLEFARM_LISTEN_PORT` | `22` | SSH listen port (the container sets `2222` and maps host 22 to it) |
| `IDLEFARM_HOST_KEY_PATH` | `var/ssh_host_key` | Server host key file (created if missing) |
| `IDLEFARM_DB_PATH` | `var/idlefarm.db` | SQLite database (WAL mode), all player state |
| `IDLEFARM_AUTOSAVE_INTERVAL` | `30s` | How often active saves are written while connected |
| `IDLEFARM_SESSION_POLICY` | `takeover` | Same save opened twice: `takeover` (new session wins) or `refuse` (newcomer turned away) |
| `IDLEFARM_DATA_DIR` | *(embedded)* | Override directory for `crops.toml` / `balance.toml` |
| `IDLEFARM_IDLE_TIMEOUT` | `30m` | Disconnect after no input |
| `IDLEFARM_MAX_SESSIONS_PER_KEY` | `2` | Concurrent sessions per key fingerprint |
| `IDLEFARM_MAX_CONNECTIONS` | `100` | Global concurrent session cap |
| `IDLEFARM_DEFAULT_SLOT` | `default` | Save slot when the username is empty after sanitization |
| `IDLEFARM_LOG_LEVEL` | `info` | `debug`, `info`, `warn`, `error` |
| `IDLEFARM_LOG_FORMAT` | `text` | `text` or `json` |
| `IDLEFARM_RATE_LIMIT_PER_SECOND` | `2` | Per-IP connection rate (token bucket) |
| `IDLEFARM_RATE_LIMIT_BURST` | `5` | Burst size for rate limiter |
| `IDLEFARM_RATE_LIMIT_MAX_IPS` | `1000` | Max tracked client IPs |

Game balance (crop stats, costs, prestige formula, unlocks, achievements) lives in [data/crops.toml](data/crops.toml) and [data/balance.toml](data/balance.toml). The files are embedded into the binary at build time; point `IDLEFARM_DATA_DIR` at a directory with replacements to rebalance without recompiling.

## How saves work

- Your **key fingerprint** is your account; the first connection creates it (trust-on-first-use).
- The **SSH username** selects a save slot *within* your account. Two people using the same username get different farms because their keys differ; no slot name can ever reach another key's data.
- Each active save is owned by a **single writer** (an actor goroutine); opening the same save from two terminals follows `IDLEFARM_SESSION_POLICY`.
- Saves are written on autosave, on disconnect, and on shutdown (SIGTERM), so server redeploys never lose progress.

The host key and database files must never be committed; see `.gitignore`.
