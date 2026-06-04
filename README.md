# ssh-idlefarmer

A cozy idle farming game you play over SSH: connect with your public key, land straight in your farm, plant crops, and come back later to harvest. No website, no client install, no password — your SSH key is your identity and the SSH username picks which save slot you open under that key.

Licensed under the [MIT License](LICENSE).

## Run locally

Requires Go 1.23+.

```bash
# Build
go build -o bin/ssh-idlefarmer ./cmd/ssh-idlefarmer

# Listen on a non-privileged port (no root needed)
set IDLEFARM_LISTEN_PORT=2222
set IDLEFARM_HOST_KEY_PATH=var/ssh_host_key
bin\ssh-idlefarmer.exe
```

In another terminal:

```bash
ssh -p 2222 -o StrictHostKeyChecking=accept-new localhost
ssh -p 2222 -o StrictHostKeyChecking=accept-new other@localhost
```

Press `q` or `Ctrl+C` to disconnect.

## Configuration

All settings use the `IDLEFARM_` prefix:

| Variable | Default | Description |
| --- | --- | --- |
| `IDLEFARM_LISTEN_HOST` | `0.0.0.0` | Bind address |
| `IDLEFARM_LISTEN_PORT` | `22` | SSH listen port |
| `IDLEFARM_HOST_KEY_PATH` | `var/ssh_host_key` | Server host key file (created if missing) |
| `IDLEFARM_IDLE_TIMEOUT` | `30m` | Disconnect after no input |
| `IDLEFARM_MAX_SESSIONS_PER_KEY` | `2` | Concurrent sessions per key fingerprint |
| `IDLEFARM_MAX_CONNECTIONS` | `100` | Global concurrent session cap |
| `IDLEFARM_DEFAULT_SLOT` | `default` | Save slot when username is empty after sanitization |
| `IDLEFARM_LOG_LEVEL` | `info` | `debug`, `info`, `warn`, `error` |
| `IDLEFARM_LOG_FORMAT` | `text` | `text` or `json` |
| `IDLEFARM_RATE_LIMIT_PER_SECOND` | `2` | Per-IP connection rate (token bucket) |
| `IDLEFARM_RATE_LIMIT_BURST` | `5` | Burst size for rate limiter |
| `IDLEFARM_RATE_LIMIT_MAX_IPS` | `1000` | Max tracked client IPs |

The host key and database files must never be committed; see `.gitignore`.
