# CLAUDE.md — Project Guidelines

## Project Overview

**hotwatermat-ble** is an open-source BLE controller for a heated mattress pad (KDO_HotWaterMat / EQM555). It communicates directly over BLE, replacing the proprietary app with open-source tooling.
The project provides a CLI, MCP server, and npm package — all built on a shared Go protocol library.

## Architecture

```
go/pkg/protocol/   → Core packet encoding/decoding (no BLE dependency)
go/pkg/ble/         → BLE client (tinygo bluetooth, CoreBluetooth on macOS)
go/cmd/hotwatermat-ble/ → CLI (cobra)
go/cmd/mcp-server/  → MCP server (JSON-RPC over stdio)
npm/                → npm package (CLI + MCP server wrapper)
skill/              → OpenClaw skill definition
```

## Key Protocol Rules

- **Packet size**: Always 20 bytes
- **Temperature encoding**: `encoded = temp + 127.5` (0.5°C precision). Bit7 is NOT a heating flag.
- **Checksum**: `sum(bytes[1:19]) & 0xFF`, stored at byte[19]
- **CHAR1** (`0100dd`): STATUS notifications only (subscribe + read)
- **CHAR2** (`0200dd`): ALL writes (handshake, commands, power) + B2F1 auth response
- **Write mode**: Always use write-with-response (not WriteWithoutResponse)
- **Auth flow**: Handshake → wait for B2F1 on CHAR2 → authenticated → send commands

## BLE Characteristics

| UUID suffix | Name  | Direction       | Usage                        |
|-------------|-------|-----------------|------------------------------|
| `0100dd`    | CHAR1 | Mat → App       | STATUS notifications         |
| `0200dd`    | CHAR2 | App → Mat (+ B2F1) | Commands, handshake, auth |
| `0400dd`    | CHAR3 | Unknown         | Not used                     |

## Development

### Testing

```bash
cd go && go test -v -race ./pkg/protocol/...
```

Only `pkg/protocol/` has tests. BLE package requires real hardware.
`tinygo.org/x/bluetooth` has a broken transitive dependency (`cyw43439`), so `go mod tidy` may fail — test protocol directly.

### Branch Strategy

- `main` = always deployable
- All changes via feature/fix/chore branches + PR
- Never push directly to main

### CI

- `test.yml`: Runs `go test` + `go vet` on protocol package
- `claude-review.yml`: Claude Code auto-review on human PRs (skips bot PRs)
- `release.yml`: goreleaser on tag push

## Trademark Policy

Do NOT use brand names "Navien" or "나비엔" anywhere in code, comments, docs, or commit messages.
Use generic terms: "hot water mat", "온수매트", "KDO_HotWaterMat" (BLE device name).

## Language

- Documentation (README, SKILL.md): Korean (한국어)
- Code comments: Korean
- CLAUDE.md, CI configs: English
- Commit messages: English prefix (`feat:`, `fix:`, `chore:`, `docs:`) + Korean or English body
