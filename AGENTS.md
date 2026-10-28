# AGENTS.md — Project Guidelines

## Project Overview

**hotwatermat-ble** is an open-source BLE controller for a heated mattress pad (KDO_HotWaterMat / EQM555). It communicates directly over BLE, replacing the proprietary app with open-source tooling.
The project provides a CLI built on a shared Go protocol library.

## Architecture

```
pkg/protocol/        → Core packet encoding/decoding (no BLE dependency)
pkg/ble/              → BLE client (tinygo bluetooth, CoreBluetooth on macOS)
pkg/config/           → Device config storage
pkg/audit/            → Audit records for mat access (macOS unified log via cgo os_log, no-op elsewhere)
cmd/hotwatermat-ble/  → CLI (cobra)
skills/hotwatermat-ble/SKILL.md → Agent Skill for coding agents (Agent Skills format)
context7.json         → Context7 indexing config (validated against its schema in `test.yml`)
```

The skill, the README "AI 에이전트" section and the `context7.json` rules restate CLI behavior
(commands, flags, output format, config resolution order, error messages). When a PR changes any of
those, update them in the same PR.

## Audit Records

Every CLI command that touches the mat (`scan`, `status`, `temp`, `on`, `off`, `setup`) is wrapped by
`withAudit` and emits an `attempt` record before and a `result` record after, linked by `req`.
The mat is a heating appliance, so this is not optional: never add a flag or env variable that skips
it, and wrap any new command that connects to the mat. Records go to the macOS unified log because a
process in the same user account (including coding agents) cannot delete or edit entries there.
Long-term retention (a root collector, off-box copies) belongs to the deployment, not this repo.

## Key Protocol Rules

- **Packet size**: Always 20 bytes
- **Temperature encoding**: whole degrees are sent as-is (`33.0 → 0x21`); only `x.5` values use `encoded = temp + 127.5` (`33.5 → 0xA1`). Bit7 is NOT a heating flag.
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
go test -v -race ./pkg/protocol/... ./pkg/audit/...
```

Only `pkg/protocol/` and `pkg/audit/` have tests. BLE package requires real hardware.
The darwin `pkg/audit` code uses cgo, so it only builds on macOS (the `darwin` CI job).
`tinygo.org/x/bluetooth` has a broken transitive dependency (`cyw43439`), so `go mod tidy` may fail — test protocol directly.

### Branch Strategy

- `main` = always deployable
- All changes via feature/fix/chore branches + PR
- Never push directly to main

### CI

- `test.yml`: gofmt check, `go vet ./...` + `go build ./...` (Linux, cgo off), `go test` on the protocol and audit packages, a macOS job that builds and tests with cgo on (the darwin audit code), and validates `context7.json` against the Context7 schema
- `claude-code-review.yml`: Claude Code auto-review on every PR
- `claude.yml`: Runs Claude Code when an owner/member/collaborator mentions `@claude` in an issue or PR
- `release.yml`: Binary release on tag push (darwin amd64/arm64, linux amd64/arm64). Darwin binaries embed `cmd/hotwatermat-ble/Info.plist` (`-sectcreate __TEXT __info_plist`) and are signed under its `CFBundleIdentifier` with the maintainer's self-signed certificate from the `MACOS_SIGNING_CERT_P12` and `MACOS_SIGNING_CERT_PASSWORD` secrets; the job fails without them. A binary launched by launchd gets Bluetooth permission in its own name, and macOS checks that grant against the identifier and the certificate, so it survives updates. Keep the plist and the signing identifier in sync, and do not change the identifier or the certificate lightly: either change makes every launchd user approve again.

## Trademark Policy

Do NOT use specific manufacturer brand names anywhere in code, comments, docs, or commit messages.
Use generic terms: "hot water mat", "온수매트", "KDO_HotWaterMat" (BLE device name).

## Language

- Documentation (README): Korean (한국어)
- Code comments: Korean
- AGENTS.md, CI configs: English
- Commit messages: English prefix (`feat:`, `fix:`, `chore:`, `docs:`) + Korean or English body
