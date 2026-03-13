# hotwatermat-ble

BLE controller skill for heated mattress pad (KDO_HotWaterMat / EQM555). Control your mat from the terminal or AI agents — no proprietary app needed.

## Prerequisites

- `hotwatermat-ble` CLI binary installed and in PATH
- BLE adapter available on host machine
- Device address: set via `HOTWATERMAT_ADDRESS` env var (or use `setup` to configure)
- Auth key: set via `HOTWATERMAT_DEVICE_GID` env var (or use `setup` to configure)

## Available Commands

### Check Status
```bash
hotwatermat-ble status
```
Displays current mode, left/right temperature, water level, and heating state.

### Set Temperature
```bash
# Set both sides to 35°C
hotwatermat-ble temp --left 35 --right 35

# Set left side only
hotwatermat-ble temp --left 33.5

# Set right side only
hotwatermat-ble temp --right 40
```
Temperature range: 28.0°C – 48.0°C (0.5°C increments)

### Power On
```bash
hotwatermat-ble on
```
Turns on the mat. Previous temperature settings are automatically restored.

### Power Off
```bash
hotwatermat-ble off
```
**Note:** The mat continues BLE advertising when powered off — it can be restarted via BLE without pressing the physical button.

### Scan for Devices
```bash
hotwatermat-ble scan
```

## MCP Server

An MCP server binary is available for direct AI agent integration:

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
| `scan` | Scan for BLE heated mat devices |
| `status` | Get current mat status |
| `set_temp` | Set target temperature (left/right) |
| `power_on` | Turn mat on |
| `power_off` | Turn mat off |

## Environment Variables

| Variable | Description |
|----------|-------------|
| `HOTWATERMAT_ADDRESS` | BLE device address (UUID format on macOS) |
| `HOTWATERMAT_DEVICE_GID` | 12-character hex authentication key |

## Tips

- The mat supports only one BLE connection at a time
- If the official app is connected, force-close it first
- Status updates stream every ~1 second while connected
- Keep connections short — connect → send command → disconnect
