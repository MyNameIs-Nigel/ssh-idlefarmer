# syntax=docker/dockerfile:1

# ---- Build stage: compile a static, cgo-free binary -------------------------
# Pinned toolchain for reproducible builds. The pure-Go SQLite driver
# (modernc.org/sqlite) and embedded content files mean the result is a single
# self-contained executable.
FROM golang:1.26.4-alpine3.22 AS build

WORKDIR /src

COPY go.mod go.sum ./
RUN go mod download

COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -trimpath -ldflags="-s -w" \
        -o /out/ssh-idlefarmer ./cmd/ssh-idlefarmer

# Pre-create the data directory with the runtime user's ownership so the
# named volume inherits writable permissions on first use (distroless has no
# shell to chown with).
RUN mkdir -p /out/data-dir && chown 65532:65532 /out/data-dir

# ---- Runtime stage: distroless, non-root, no shell --------------------------
# distroless/static has no libc, no shell, no package manager: nothing to
# escape into, nothing to exploit beyond the game itself.
FROM gcr.io/distroless/static-debian12:nonroot

COPY --from=build /out/ssh-idlefarmer /usr/local/bin/ssh-idlefarmer
COPY --from=build --chown=65532:65532 /out/data-dir /var/lib/idlefarm

# The app's own default port is 22; inside the container it listens on an
# unprivileged port instead and the operator maps host 22 to it, so the
# process never needs root or CAP_NET_BIND_SERVICE.
ENV IDLEFARM_LISTEN_PORT=2222 \
    IDLEFARM_HOST_KEY_PATH=/var/lib/idlefarm/ssh_host_key \
    IDLEFARM_DB_PATH=/var/lib/idlefarm/idlefarm.db

EXPOSE 2222

# Player saves and the SSH host key live here — always mount a volume, or
# both are lost (and clients see host-key warnings) on every redeploy.
VOLUME ["/var/lib/idlefarm"]

USER nonroot:nonroot

ENTRYPOINT ["/usr/local/bin/ssh-idlefarmer"]
