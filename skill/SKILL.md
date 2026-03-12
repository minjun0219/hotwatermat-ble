# hotwatermat-ble

Control a BLE hot water mat (KDO_HotWaterMat) from Claude Code.

## Overview

This skill provides control of a KDO_HotWaterMat BLE hot water mat device. It can check status, set temperature, and control power via the CLI tool or MCP server.

## Prerequisites

- The `hotwatermat-ble` CLI binary must be installed and in PATH
- BLE adapter must be available on the host machine
- Device address can be set via `HOTWATERMAT_ADDRESS` environment variable
- Device authentication key can be set via `HOTWATERMAT_DEVICE_GID` environment variable

## Available Commands

### Check Status
```bash
hotwatermat-ble status
```
Shows current mode, temperatures (left/right), water level, and heating state.

### Set Temperature
```bash
# Set both sides to 35°C
hotwatermat-ble temp --left 35 --right 35

# Set left side only
hotwatermat-ble temp --left 33.5

# Set right side only
hotwatermat-ble temp --right 40
```
Temperature range: 28.0°C to 48.0°C in 0.5°C increments.

### Power On
```bash
hotwatermat-ble on
```
Turns on the mat. Previous temperature settings are restored automatically.

### Power Off
```bash
hotwatermat-ble off
```
**Warning:** After power off, the mat stops BLE advertising. A physical button press is required to turn it back on.

### Scan for Devices
```bash
hotwatermat-ble scan
```

## MCP Server

The MCP server can be used as a Claude Code MCP server for direct tool integration:

```json
{
  "mcpServers": {
    "hotwatermat": {
      "command": "hotwatermat-ble-mcp",
      "args": []
    }
  }
}
```

### MCP Tools

| Tool | Description |
|------|-------------|
| `scan` | Scan for BLE hot water mat devices |
| `status` | Get current mat status |
| `set_temp` | Set target temperature (left/right) |
| `power_on` | Turn on the mat |
| `power_off` | Turn off the mat |

## Configuration

| Environment Variable | Description |
|---------------------|-------------|
| `HOTWATERMAT_ADDRESS` | BLE device address (UUID format on macOS) |
| `HOTWATERMAT_DEVICE_GID` | 12-char hex authentication key |

## Tips

- The mat only supports one BLE connection at a time
- If the official app is connected, force-stop it first
- Status updates stream at ~1/second while connected
- Connections are short-lived by design — connect, send command, disconnect
