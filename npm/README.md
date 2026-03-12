# hotwatermat-ble

BLE protocol library for KDO_HotWaterMat (hot water mat) devices. Provides packet building and parsing via a WASM-compiled Go core.

## Installation

```bash
npm install hotwatermat-ble
```

## Usage

```typescript
import {
  init,
  encodeTemp,
  decodeTemp,
  buildHandshake,
  buildHeat,
  buildPowerOn,
  buildPowerOff,
  parseStatus,
  SIDE_LEFT,
  SIDE_BOTH,
} from "hotwatermat-ble";

// Initialize WASM module (required once)
await init();

// Temperature encoding (0.5°C precision)
const byte33 = encodeTemp(33.0); // 33
const byte335 = encodeTemp(33.5); // 161 (0xA1)
const temp = decodeTemp(161); // 33.5

// Build authentication handshake
const handshake = buildHandshake("13CE3CC53E5A");

// Build HEAT command (set left to 34°C)
const leftTarget = encodeTemp(34.0);
const rightTarget = encodeTemp(28.0);
const heatPkt = buildHeat(SIDE_LEFT, 0x21, 0x1c, leftTarget, rightTarget);

// Build power commands
const onPkt = buildPowerOn(0x21, 0x1c);
const offPkt = buildPowerOff(0x21, 0x1c);

// Parse STATUS notification from device
const status = parseStatus(statusPacket);
console.log(status.leftCurrent, status.rightTarget, status.poweredOff);
```

## API

### `init(): Promise<void>`
Initialize the WASM module. Must be called before any other function.

### `encodeTemp(temp: number): number`
Encode temperature (28.0-48.0, 0.5 steps) to BLE byte value.

### `decodeTemp(byteVal: number): number`
Decode BLE byte value to temperature.

### `calcChecksum(packet: Uint8Array): number`
Calculate packet checksum.

### `buildHandshake(deviceGidHex?: string): Uint8Array`
Build authentication handshake packet.

### `buildHeat(side, leftCur, rightCur, leftTarget, rightTarget): Uint8Array`
Build HEAT command packet.

### `buildPowerOn(leftCur?, rightCur?): Uint8Array`
Build power-on command packet.

### `buildPowerOff(leftCur?, rightCur?): Uint8Array`
Build power-off command packet.

### `parseStatus(packet: Uint8Array): StatusResult`
Parse STATUS notification packet.

## Constants

| Name | Value | Description |
|------|-------|-------------|
| `SERVICE_UUID` | `00001c0d-...0000dd` | BLE service UUID |
| `CHAR1_UUID` | `00001c0d-...0100dd` | STATUS characteristic |
| `CHAR2_UUID` | `00001c0d-...0200dd` | COMMAND characteristic |
| `SIDE_LEFT` | `0x02` | Left side |
| `SIDE_RIGHT` | `0x04` | Right side |
| `SIDE_BOTH` | `0x06` | Both sides |
| `TEMP_MIN` | `28.0` | Minimum temperature |
| `TEMP_MAX` | `48.0` | Maximum temperature |

## License

MIT
