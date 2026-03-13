# BLE Protocol Specification

Reverse-engineered protocol for KDO_HotWaterMat (온수매트) BLE control.

## Device Info

| Field | Value |
|-------|-------|
| BLE Name | `KDO_HotWaterMat` |
| Service UUID | `00001c0d-d102-11e1-9b23-2ce2a80000dd` |
| CHAR1 (STATUS) | `00001c0d-d102-11e1-9b23-2ce2a80100dd` — Status notifications only |
| CHAR2 (COMMAND) | `00001c0d-d102-11e1-9b23-2ce2a80200dd` — All writes + B2F1 auth response |
| CHAR3 | `00001c0d-d102-11e1-9b23-2ce2a80400dd` — Role TBD |

**Critical:** ALL writes (handshake + commands) go to **CHAR2**. STATUS arrives on CHAR1, B2F1 auth on CHAR2.

## Connection Sequence

```
1. BLE connect
2. Subscribe to notifications on CHAR1 and CHAR2
3. Wait ~300ms
4. Write HANDSHAKE to CHAR2 (with response)
5. Receive B2F1 type=0x02 on CHAR2 → authenticated
6. STATUS packets stream on CHAR1 (~1/sec)
7. Send commands to CHAR2
8. Disconnect when done (short-lived connections are normal)
```

## Packet Structure (20 bytes)

```
Byte  Field          Description
────  ─────          ───────────
[0]   STX            0xB2 (fixed)
[1]   Direction      0x80=app→mat, 0x00=mat→app, 0x01=handshake, 0xF1=auth
[2]   Mode           Operating mode
[3]   Side           Target side (bitmask)
[4]   Vol/Water      (volume << 4) | water_level
[5]   Field5         0x02=HEAT, 0xFE=POWER commands
[6]   Field6         0x25=HEAT, 0x2B=POWER ON, 0xAB=POWER OFF
[7]   LeftCurrent    Left current confirmed temp (copy from STATUS)
[8]   RightCurrent   Right current confirmed temp (copy from STATUS)
[9]   Field9         0x23=HEAT, 0xFE=POWER
[10]  LeftTarget     Left new target temperature
[11]  RightTarget    Right new target temperature
[12]  SubCmd         Subcommand (FASTHEAT on/off, SLEEP timer, etc.)
[13‑18] Padding      0xFE
[19]  Checksum       See checksum algorithm below
```

## Temperature Encoding (0.5°C precision)

Temperatures support **0.5°C increments** using a special encoding:

```python
def encode_temp(temp: float) -> int:
    """Encode temperature to BLE byte value."""
    if temp != int(temp):  # has decimal (e.g. 33.5)
        return int(temp + 127.5)
    return int(temp)

def decode_temp(byte_val: int) -> float:
    """Decode BLE byte to temperature."""
    if byte_val > 127:
        return byte_val - 127.5
    return float(byte_val)
```

### Encoding Table

| Temperature | Byte Value | Hex |
|-------------|------------|-----|
| 28.0°C | 28 | 0x1C |
| 28.5°C | 156 | 0x9C |
| 33.0°C | 33 | 0x21 |
| 33.5°C | 161 | 0xA1 |
| 34.0°C | 34 | 0x22 |
| 34.5°C | 162 | 0xA2 |
| 48.0°C | 48 | 0x30 |

**Range:** 28.0°C – 48.0°C in 0.5°C steps.

## Mode Values (byte[2])

| Value | Name | Description |
|-------|------|-------------|
| 0x01 | HEAT | Normal heating |
| 0x02 | TIMER_OFF | Timer-off mode |
| 0x03 | SLEEP | Sleep mode |
| 0x06 | POWER | Power ON/OFF control |
| 0x07 | FASTHEAT | Fast heating |
| 0x08 | IONCARE | Ion care mode |

## Side Values (byte[3])

| Value | Meaning |
|-------|---------|
| 0x02 | Left only |
| 0x04 | Right only |
| 0x06 | Both sides |

## Commands

### Temperature Control (HEAT)

Set target temperature for one or both sides.

```
B2 80 01 [side] 00 02 25 [L_cur] [R_cur] 23 [L_tgt] [R_tgt] FE FE FE FE FE FE FE [checksum]
```

- `byte[7-8]`: Copy current temps from latest STATUS packet (raw byte, no modification)
- `byte[10-11]`: New target temps using 0.5°C encoding

**Example: Left 33°C → 34°C**
```
B2 80 01 02 00 02 25 21 1C 23 22 1C FE FE FE FE FE FE FE 3A
                     ^^          ^^ 
                   33°C        34°C
```

**Example: Left → 33.5°C**
```
B2 80 01 02 00 02 25 21 1C 23 A1 1C FE FE FE FE FE FE FE B9
                               ^^
                         0xA1 = 33.5°C
```

### Power ON

```
B2 80 06 06 00 FE 2B [L_cur] [R_cur] FE 00 00 00 00 00 00 FE FE FE [checksum]
                  ^^
              0x2B = ON
```

Mat restores previous temperature targets automatically.

### Power OFF

```
B2 80 06 04 00 FE AB [L_cur] [R_cur] FE 00 [R_cur] FE [R_cur] FE FE FE FE [checksum]
                  ^^
              0xAB = OFF
```

**Note:** The mat continues BLE advertising after power OFF — it can be turned back on via BLE without pressing the physical button.

## Authentication

### Initial Pairing (mat in pairing mode, one-time)

```
App → Mat (CHAR2): B2 01 FE FE FE FE FE FE FE FE FE FE FE FE FE FE FE FE FE DF
Mat → App (CHAR2): B2 F1 01 [6-byte DeviceGid] FF FF FF FF FF FF FF FF FF FF [checksum]
```

Mat generates a unique 6-byte key (DeviceGid) and sends it to the app.

### Subsequent Connections

```
App → Mat (CHAR2): B2 01 [6-byte DeviceGid] FE FE FE FE FE FE FE FE FE FE FE [checksum]
Mat → App (CHAR2): B2 F1 02 FF FF FF FF FF FF FF FF FF FF FF FF FF FF FF FF E3
```

- B2F1 type=0x02 = authenticated ✅
- Wrong key → mat disconnects after ~4.5 seconds
- DeviceGid is also stored on the cloud server for multi-device sync

### DeviceGid Structure

- 6 significant bytes + `FE FE` padding = 8 bytes in handshake positions [2..9]
- Example: key `13CE3CC53E5A` → handshake bytes: `13 CE 3C C5 3E 5A FE FE`

## Checksum Algorithm

```python
def calc_checksum(packet: bytes) -> int:
    """Calculate checksum for bytes[1:19]."""
    total = sum(packet[1:19]) & 0xFFFF   # 16-bit sum
    hex_str = f"{total:04X}"             # 4-digit hex string
    return int(hex_str[2:], 16)          # last 2 hex digits as int
```

Verified against APK source (`md/a.java`).

## STATUS Packet (mat → app)

Sent on CHAR1, ~1 per second while connected.

```
B2 00 [mode] [side] [vol/water] [field5] [field6] [L_cur] [R_cur] [field9] [L_tgt] [R_tgt] [sub] [extra...] [checksum]
```

- Temperature bytes use the same 0.5°C encoding
- `mode=0x06` + `L_tgt=0` + `R_tgt=0` → power OFF state

## Implementation Notes

1. **CHAR2 for all writes** — handshake AND commands both go to CHAR2
2. **Copy current temps** — always copy byte[7-8] from latest STATUS when building commands
3. **Short connections** — official app uses connect/command/disconnect cycles, not persistent connections
4. **No BLE bonding** — all auth is application-level via DeviceGid handshake
5. **Write with response** — use `write_gatt_char(..., response=True)` for reliability
6. **Power OFF keeps BLE** — mat continues advertising after power off, can be turned back on via BLE

---
*Reverse-engineered from Android app HCI snoop captures + APK source analysis, March 2026*
