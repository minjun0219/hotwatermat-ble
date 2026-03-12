# hotwatermat-ble

Control a BLE hot water mat (KDO_HotWaterMat / EQM555) from the command line, MCP server, or JavaScript/TypeScript.

## Architecture

```
┌─────────────────────────────────────────────────┐
│                   Applications                   │
│  ┌──────────┐  ┌───────────┐  ┌──────────────┐  │
│  │   CLI    │  │MCP Server │  │  npm package │  │
│  │  (cobra) │  │ (JSON-RPC)│  │  (TypeScript)│  │
│  └────┬─────┘  └─────┬─────┘  └──────┬───────┘  │
│       │              │               │           │
│  ┌────┴──────────────┴───┐    ┌──────┴───────┐  │
│  │     go/pkg/ble/       │    │  WASM build  │  │
│  │   (tinygo bluetooth)  │    │  (TinyGo)    │  │
│  └────────────┬──────────┘    └──────┬───────┘  │
│               │                      │           │
│  ┌────────────┴──────────────────────┴───────┐  │
│  │          go/pkg/protocol/                  │  │
│  │    Packet build/parse, temp encoding,      │  │
│  │    checksum, handshake, STATUS parser       │  │
│  └────────────────────────────────────────────┘  │
└─────────────────────────────────────────────────┘
```

## Installation

### CLI (from release)

Download from [Releases](https://github.com/minjun0219/hotwatermat-ble/releases):

```bash
# macOS ARM
curl -L -o hotwatermat-ble \
  https://github.com/minjun0219/hotwatermat-ble/releases/latest/download/hotwatermat-ble-darwin-arm64
chmod +x hotwatermat-ble
sudo mv hotwatermat-ble /usr/local/bin/
```

### CLI (from source)

Requires Go 1.22+:

```bash
cd go
go build -o hotwatermat-ble ./cmd/hotwatermat-ble/
```

### npm package

```bash
npm install hotwatermat-ble-core
```

### Python CLI (prototype)

```bash
pip install -e .
```

## Usage

### CLI Commands

```bash
# Scan for devices
hotwatermat-ble scan

# Show current status
hotwatermat-ble status

# Set temperature (28.0-48.0°C, 0.5°C steps)
hotwatermat-ble temp --left 35 --right 35
hotwatermat-ble temp --left 33.5

# Power on
hotwatermat-ble on

# Power off (requires physical button to restart)
hotwatermat-ble off
```

### CLI Options

```bash
# Specify BLE address
hotwatermat-ble --address <ADDRESS> status

# Specify device authentication key
hotwatermat-ble --device-gid 13CE3CC53E5A status

# Enable debug output
hotwatermat-ble --debug status

# Environment variables
export HOTWATERMAT_ADDRESS=FD319CFA-2E62-116D-D348-5B9FEEE95D2F
export HOTWATERMAT_DEVICE_GID=13CE3CC53E5A
```

### MCP Server

The MCP server enables tool-based control from AI assistants:

```bash
# Build and run
cd go && go build -o hotwatermat-ble-mcp ./cmd/mcp-server/
```

Configure in Claude Code (`settings.json`):

```json
{
  "mcpServers": {
    "hotwatermat": {
      "command": "/path/to/hotwatermat-ble-mcp"
    }
  }
}
```

Available MCP tools: `scan`, `status`, `set_temp`, `power_on`, `power_off`

### npm Package

```typescript
import { init, encodeTemp, buildHeat, parseStatus, SIDE_BOTH } from 'hotwatermat-ble-core';

await init(); // Load WASM

const target = encodeTemp(35.0);
const packet = buildHeat(SIDE_BOTH, 0x21, 0x1C, target, target);
```

## Protocol

See [PROTOCOL.md](PROTOCOL.md) for the complete BLE protocol specification.

Key details:
- BLE name: `KDO_HotWaterMat`
- Service UUID: `00001c0d-d102-11e1-9b23-2ce2a80000dd`
- 20-byte fixed packets with checksum
- Temperature: 28.0–48.0°C, 0.5°C precision
- Application-level auth via 6-byte DeviceGid handshake
- Single BLE connection only (mat stops advertising when connected)

## Project Structure

```
hotwatermat-ble/
├── go/
│   ├── pkg/protocol/     # Packet build/parse (pure Go, no BLE deps)
│   ├── pkg/ble/          # BLE client (tinygo bluetooth)
│   ├── cmd/hotwatermat-ble/  # CLI binary
│   └── cmd/mcp-server/   # MCP server (JSON-RPC stdio)
├── wasm/                 # TinyGo WASM build of protocol
├── npm/                  # npm package (TypeScript wrapper)
├── skill/                # OpenClaw skill definition
├── hotwatermat_ble/      # Python prototype
├── PROTOCOL.md           # BLE protocol specification
└── .github/workflows/    # CI/CD (test + release)
```

## Development

```bash
# Run Go tests
cd go && go test -v ./pkg/protocol/...

# Build WASM
chmod +x wasm/build.sh && ./wasm/build.sh

# Build npm package
cd npm && npm run build
```

## License

MIT
