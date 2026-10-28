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
[5]   Run/Field5     HEAT: sides to run (0x02 both, 0x03 right standby, 0x04 left standby); 0xFE=POWER commands
[6]   Field6         0x25=HEAT, 0x2B=POWER ON, 0xAB=POWER OFF
[7]   LeftCurrent    Left current temp (copy from STATUS; 0x1C to wake a standby side, see Standby)
[8]   RightCurrent   Right current temp (copy from STATUS; 0x1C to wake a standby side)
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
- Example: key `A1B2C3D4E5F6` → handshake bytes: `A1 B2 C3 D4 E5 F6 FE FE`

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

### Live capture check (2026-10)

The layout above was re-checked against live STATUS packets and the official app's display (issue #54 suspected `[7..8]` and `[10..11]` were swapped; it did not reproduce):

```
B2 00 01 06 02 03 30 A5 19 23 A6 27 FE FE FE 00 C0 13 05 BC   app: left cur 38.5 / set 38.5, right standby cur 25.0
B2 00 01 06 02 03 AA A6 19 23 26 27 FE FE FE 00 C0 13 05 B7   after left set → 38.0 in the app
B2 00 01 06 02 02 29 26 1C 23 26 1C FE FE FE 00 C0 13 05 AD   after right standby → on, set 28.0 in the app
B2 00 01 06 02 04 28 25 1C 23 26 1C FE FE FE 00 C0 13 05 AD   after left → standby in the app
```

- `byte[10]` followed the left set change (`A6` 38.5 → `26` 38.0), so `[10..11]` = set temps
- `byte[7]` matched the app's left current temp (38.5), and `byte[8]=0x19` (25.0, below the 28°C set range) matched the right current temp, so `[7..8]` = current temps
- A side in standby still reports its last set temp in `byte[10..11]` (right `0x27` = 39.0 above)
- `byte[5]` tells which sides run (issue #67; HEAT `[5]` uses the same values, see Standby): `0x02` both, `0x03` left only (right standby), `0x04` right only (left standby). The value with both sides in standby is not yet known
- A side in standby keeps reporting its current temp in `byte[7..8]` (it follows the water as it cools)
- `byte[6]` changed (`30` → `AA` → `29`) without a matching user action; meaning unknown

### Standby (issue #69)

HEAT `[5]` is not a fixed value. It has the same meaning as STATUS `[5]`: the sides that should run (`0x02` both, `0x03` right standby, `0x04` left standby). Confirmed on the mat 2026-10-28.

**Entering standby:** send HEAT with `[5]=0x03` (right) or `0x04` (left).

```
B2 80 01 06 00 03 25 1C 1C 23 26 1C FE FE FE FE FE FE FE 3E   → right: STATUS [5] 02 → 03
B2 80 01 20 00 04 25 1C 1C 23 1C 1C FE FE FE FE FE FE FE ..   → left:  STATUS [5] 02 → 04
```

Both worked with different `[3]` values (`06`, `20`); the rule for `[3]` here is unknown.

**Leaving standby:** send HEAT with `[5]=0x02` and that side's "current" byte (`[7]` left, `[8]` right) set to `1C` (28.0). With the side's real water temperature above 28 in that byte, the mat ignores the command. Confirmed packet, left standby with left water at 32.0:

```
B2 80 01 02 00 02 25 1C 1C 23 1C 1C FE FE FE FE FE FE FE 2F   → STATUS [5] 04 → 02
```

- Left: `[7]=1C` with a target of 38 also woke the side and applied 38 in one command.
- Right: `[8]=1C` with a target of 38 did not wake it; it woke only with a target of 28.0 (or 28.5 in case C). The official app also wakes a side at 28.0 and raises it afterwards.
- So `temp` wakes a standby side at 28.0 first, waits for STATUS `[5]` to show it running, then sends the requested target (`ble.Client.SetTemp`). Waking the right side from water above 28 in two steps is not yet confirmed on the mat.
- A side that `temp` does not target keeps its standby: the CLI sends `[5]=0x03`/`0x04` for it instead of `0x02` (`protocol.HeatRun`).
- Earlier wakes fit this: in cases C and 3 the right water had cooled to 28.0, so the copied `[8]` was already `1C`.
- Unknown: whether values below 28 in `[7]`/`[8]` also wake a side.

**Side link (STATUS `[3]`, observed 2026-10-28):** with STATUS `[3]=06`, the first left-only HEAT (`[3]=02`) only switches STATUS `[3]` to `02` and drops the new target; the same command sent again applies. Seen with `temp --left 38.5` (`[10]` stayed `26`, then `A6` on the retry). A right-only command (`[3]=04`) from `06` applied at once. Some of the failed wakes below may include this effect. `ble.Client.SetTemp` resends a HEAT once when STATUS `[3]` changed but the command did not apply.

Earlier HEAT attempts (bytes counted from 0, checksums valid):

| Case | Before (STATUS) | Sent (HEAT) | After (STATUS) | Result |
|---|---|---|---|---|
| A: left standby, `temp --left 38.5` (left set 38.0 → 38.5) | `B2 00 01 06 02 04 A6 A4 1C 23 26 1C … AA` | `[3]=02 [5]=02 [10]=A6 [11]=1C` | `B2 00 01 02 02 04 A5 A4 1C 23 26 1C … A5` | stays standby |
| B: right standby, `temp --left 38.5 --right 28` (values unchanged) | `B2 00 01 02 02 03 AF A5 1C 23 A6 1C … 2F` | `B2 80 01 06 00 02 25 A5 1C 23 A6 1C FE FE FE FE FE FE FE 46` | `B2 00 01 06 02 03 AF A5 1C 23 A6 1C … 33` | stays standby |
| C: right standby, `temp --right 28.5` (right set 28.0 → 28.5) | `B2 00 01 06 02 03 2E A5 1C 23 A6 1C … B2` | `B2 80 01 04 00 02 25 A5 1C 23 A6 9C FE FE FE FE FE FE FE C4` | `B2 00 01 06 02 02 2E A5 1C 23 A6 9C … 31` | runs again (`[5]` 03 → 02) |

Later checks (2026-10-28, STATUS `[5]` read with `--debug`):

| Case | Sent (HEAT) | Result |
|---|---|---|
| 1, 2: left standby, `temp --left` 38.5 and 39.0, also right after a both-sides command | `[3]=02`, left set raised | stays standby |
| 3: right standby, `temp --right 28` (value unchanged at 28.0) | `[3]=04 [11]=1C` | runs again (`[5]` 03 → 02) |
| 5: the official app wakes the left side | STATUS after it: `[3]=02 [10]=1C` (28.0) | runs again |
| 6: left standby, `temp --left 28` (38.5 → 28.0) | `B2 80 01 02 00 02 25 A4 1C 23 1C 1C FE FE FE FE FE FE FE B7` | stays standby, STATUS unchanged |

- A left standby side ignored every HEAT we sent: `[3]=02` with 28.0, 38.5 and 39.0
- A right standby side woke on `[3]=04` with 28.0 and 28.5
- The app woke the left side and left `[10]=1C` (28.0) and `[3]=02`. The difference from our packets turned out to be `[7]` (see above)
- `[3]=06` with unchanged values did not wake the right side (B)
- While a side is running, `[3]=02` HEAT applies normally (left 28.5 and 38.5 both took)
- The app shows standby as the slider stop below 28.0, so entering standby may be a target below the CLI's floor. No packet for it has been seen
- STATUS `[3]` follows `02`/`06` commands and stays `06` after `04` commands. It looks like the app's "same temperature on both sides" link (`06` linked, `02` left on its own); see below

## Implementation Notes

1. **CHAR2 for all writes** — handshake AND commands both go to CHAR2
2. **Copy current temps** — always copy byte[7-8] from latest STATUS when building commands
3. **Short connections** — official app uses connect/command/disconnect cycles, not persistent connections
4. **No BLE bonding** — all auth is application-level via DeviceGid handshake
5. **Write with response** — use `write_gatt_char(..., response=True)` for reliability
6. **Power OFF keeps BLE** — mat continues advertising after power off, can be turned back on via BLE

---
*Reverse-engineered from Android app HCI snoop captures + APK source analysis, March 2026*
