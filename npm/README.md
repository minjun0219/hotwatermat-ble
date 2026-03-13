# hotwatermat-ble

CLI and MCP server for BLE-controlled heated mattress pads (KDO_HotWaterMat / EQM555).

This npm package downloads pre-built native binaries from [GitHub Releases](https://github.com/minjun0219/hotwatermat-ble/releases).

## Installation

```bash
npm install -g hotwatermat-ble
```

The `postinstall` script automatically downloads the correct binary for your platform.

### Skip binary download

```bash
HOTWATERMAT_SKIP_BINARY=1 npm install -g hotwatermat-ble
```

## Usage

### CLI

```bash
hotwatermat-ble --help
hotwatermat-ble status
hotwatermat-ble set --temp 36 --side both
```

### MCP Server

```bash
hotwatermat-ble-mcp
```

#### Claude Desktop configuration

```json
{
  "mcpServers": {
    "hotwatermat-ble": {
      "command": "npx",
      "args": ["-y", "hotwatermat-ble-mcp"]
    }
  }
}
```

## Supported Platforms

| OS      | Architecture |
|---------|-------------|
| macOS   | x64, arm64  |
| Linux   | x64, arm64  |
| Windows | x64, arm64  |

## Building from source

If your platform is not supported or you prefer building from source:

```bash
git clone https://github.com/minjun0219/hotwatermat-ble.git
cd hotwatermat-ble/go
go build -o hotwatermat-ble ./cmd/hotwatermat-ble
go build -o hotwatermat-ble-mcp ./cmd/mcp-server
```

## License

MIT
