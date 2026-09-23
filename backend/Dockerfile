# syntax=docker/dockerfile:1

# ---- Builder ----------------------------------------------------------
# Official Go image, pinned to the toolchain version in go.mod (go 1.27.1).
FROM golang:1.27-alpine AS builder

WORKDIR /src

# Cache module downloads in their own layer, separate from source changes.
COPY go.mod go.sum ./
RUN go mod download

COPY . .

# CGO_ENABLED=0 produces a fully static binary with no libc dependency, so it
# runs unmodified on the distroless base below. GOOS/GOARCH are pinned to
# linux/amd64 to match Cloud Run's runtime regardless of the host arch this
# image is built on (e.g. an Apple Silicon dev machine).
RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 \
    go build -trimpath -ldflags="-s -w" -o /out/server ./cmd/server

# ---- Runtime ------------------------------------------------------------
# distroless "static" has no shell, package manager, or libc - just enough to
# run a static binary, which minimizes image size and attack surface. The
# "nonroot" variant runs as an unprivileged user (uid/gid 65532) by default.
FROM gcr.io/distroless/static-debian12:nonroot

WORKDIR /app

# cmd/server/main.go's repoRoot() finds the backend module root by checking
# whether the current working directory contains go.mod; it then loads
# config/config.yaml and prompts/render.tmpl relative to that root. Since
# WORKDIR is /app, go.mod (plus config/ and prompts/) must also live at /app
# so repoRoot() resolves to /app - the file-source fallback in repoRoot()
# only works under `go run`/`go build`, not in this image, so getting this
# WORKDIR/COPY layout right is required for the binary to find its files.
COPY --from=builder --chown=nonroot:nonroot /out/server ./server
COPY --from=builder --chown=nonroot:nonroot /src/go.mod ./go.mod
COPY --from=builder --chown=nonroot:nonroot /src/config ./config
COPY --from=builder --chown=nonroot:nonroot /src/prompts ./prompts

# Cloud Run injects PORT (config.Load overrides config.yaml's server.port
# with it - see internal/config/config.go); 8080 is the documented default.
EXPOSE 8080

ENTRYPOINT ["/app/server"]
