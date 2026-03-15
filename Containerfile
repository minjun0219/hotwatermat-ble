# Linux BLE build/test environment
FROM golang:1.22

RUN apt-get update && apt-get install -y --no-install-recommends \
    libdbus-1-dev \
    && rm -rf /var/lib/apt/lists/*

WORKDIR /src

# Usage:
#   podman build -t hotwatermat-ble-dev -f Containerfile .
#   podman run --rm -v .:/src hotwatermat-ble-dev go build ./cmd/hotwatermat-ble/
#   podman run --rm -v .:/src hotwatermat-ble-dev go test -v -race ./pkg/protocol/...
