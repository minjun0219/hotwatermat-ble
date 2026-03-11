"""BLE protocol constants and packet builders for KDO_HotWaterMat.

Packet structure (20 bytes), verified from HCI snoop captures:
  [0]  STX         = 0xB2
  [1]  Direction   = 0x80 (app->mat) | 0x00 (mat->app)
  [2]  Mode        = operating mode
  [3]  Side        = 0x02 left | 0x04 right | 0x06 both
  [4]  Vol/Water   = (volume<<4)|water_level
  [5]  Field5      = 0x02 for HEAT, 0xFE for POWER cmds
  [6]  Field6      = 0x25 for HEAT, 0x2B=ON / 0xAB=OFF for POWER
  [7]  LeftCurrent = current left temp (raw byte from STATUS)
  [8]  RightCurrent= current right temp (raw byte from STATUS)
  [9]  Field9      = 0x23 for HEAT, 0xFE for POWER
  [10] LeftTarget  = new left target temperature (0.5C encoded)
  [11] RightTarget = new right target temperature (0.5C encoded)
  [12] SubCmd      = 0xFE pad (or fastheat 0x01/0x00)
 [13-18] Padding   = 0xFE
  [19] Checksum    = last 2 hex digits of sum(bytes[1:19]) & 0xFFFF

Write target: CHAR2 (0200dd) for ALL writes.
B2F1 auth response arrives on CHAR2.
STATUS stream arrives on CHAR1 (0100dd).
"""

from __future__ import annotations

PACKET_SIZE = 20
STX = 0xB2
DIR_APP_TO_MAT = 0x80
DIR_MAT_TO_APP = 0x00

SERVICE_UUID = "00001c0d-d102-11e1-9b23-2ce2a80000dd"
CHAR1_UUID = "00001c0d-d102-11e1-9b23-2ce2a80100dd"  # STATUS (read + notify)
CHAR2_UUID = "00001c0d-d102-11e1-9b23-2ce2a80200dd"  # COMMAND write + B2F1 receive

DEFAULT_ADDRESS = "FD319CFA-2E62-116D-D348-5B9FEEE95D2F"

# Default DeviceGid -- 6 bytes extracted from BLE HCI log during pairing.
# Replace with your own if you re-pair the device.
DEFAULT_GID = bytes.fromhex("13CE3CC53E5A")

# Mode byte (byte[2])
MODE_HEAT = 0x01
MODE_TIMER_OFF = 0x02
MODE_SLEEP = 0x03
MODE_POWER = 0x06   # used for both ON and OFF
MODE_FASTHEAT = 0x07
MODE_IONCARE = 0x08

# Side byte (byte[3])
SIDE_LEFT = 0x02
SIDE_RIGHT = 0x04
SIDE_BOTH = 0x06

# Temperature range
TEMP_MIN = 28.0
TEMP_MAX = 48.0
PAD = 0xFE


def encode_temp(temp: float) -> int:
    """Encode temperature to BLE byte value (0.5C precision).

    Whole degrees map directly (e.g. 33 -> 33).
    Half degrees add 127.5 (e.g. 33.5 -> 161 = 0xA1).
    """
    if temp != int(temp):  # has decimal (e.g. 33.5)
        return int(temp + 127.5)
    return int(temp)


def decode_temp(byte_val: int) -> float:
    """Decode BLE byte to temperature (0.5C precision).

    Values > 127 represent half-degree temps (e.g. 161 -> 33.5).
    Values <= 127 are direct degree values (e.g. 33 -> 33.0).
    """
    if byte_val > 127:
        return byte_val - 127.5
    return float(byte_val)


def _checksum(pkt: bytearray) -> int:
    """sum(bytes[1:19]) & 0xFFFF -> take last 2 hex digits."""
    total = sum(pkt[1:19]) & 0xFFFF
    return int(f"{total:04X}"[2:], 16)


def build_handshake(gid: bytes = DEFAULT_GID) -> bytes:
    """Build auth handshake packet. Write to CHAR2 after subscribing."""
    pkt = bytearray(PACKET_SIZE)
    pkt[0] = STX
    pkt[1] = 0x01           # handshake direction byte
    pkt[2:8] = gid[:6]
    for i in range(8, 19):
        pkt[i] = PAD
    pkt[19] = _checksum(pkt)
    return bytes(pkt)


def build_heat(left_cur: int, right_cur: int,
               left_tgt: float, right_tgt: float,
               side: int = SIDE_BOTH) -> bytes:
    """Build temperature control command.

    Args:
        left_cur:  current left temp raw byte from STATUS (no modification)
        right_cur: current right temp raw byte from STATUS (no modification)
        left_tgt:  new left target temperature (28.0-48.0, 0.5 steps)
        right_tgt: new right target temperature (28.0-48.0, 0.5 steps)
        side:      SIDE_LEFT / SIDE_RIGHT / SIDE_BOTH
    """
    pkt = bytearray(PACKET_SIZE)
    pkt[0] = STX
    pkt[1] = DIR_APP_TO_MAT
    pkt[2] = MODE_HEAT
    pkt[3] = side
    pkt[4] = 0x00
    pkt[5] = 0x02
    pkt[6] = 0x25
    pkt[7] = left_cur
    pkt[8] = right_cur
    pkt[9] = 0x23
    pkt[10] = encode_temp(left_tgt)
    pkt[11] = encode_temp(right_tgt)
    for i in range(12, 19):
        pkt[i] = PAD
    pkt[19] = _checksum(pkt)
    return bytes(pkt)


def build_power_on(left_cur: int, right_cur: int) -> bytes:
    """Build power ON command.

    Args:
        left_cur:  current left temp raw byte from STATUS
        right_cur: current right temp raw byte from STATUS
    """
    pkt = bytearray(PACKET_SIZE)
    pkt[0] = STX
    pkt[1] = DIR_APP_TO_MAT
    pkt[2] = MODE_POWER
    pkt[3] = SIDE_BOTH
    pkt[4] = 0x00
    pkt[5] = PAD
    pkt[6] = 0x2B          # ON signal
    pkt[7] = left_cur
    pkt[8] = right_cur
    pkt[9] = PAD
    # bytes[10..15] = 0x00 (already zeroed)
    for i in range(16, 19):
        pkt[i] = PAD
    pkt[19] = _checksum(pkt)
    return bytes(pkt)


def build_power_off(left_cur: int, right_cur: int) -> bytes:
    """Build power OFF command.

    Warning: After power OFF, the mat stops BLE advertising entirely.
    Physical button required to restart.

    Args:
        left_cur:  current left temp raw byte from STATUS
        right_cur: current right temp raw byte from STATUS
    """
    pkt = bytearray(PACKET_SIZE)
    pkt[0] = STX
    pkt[1] = DIR_APP_TO_MAT
    pkt[2] = MODE_POWER
    pkt[3] = SIDE_RIGHT     # 0x04 from capture
    pkt[4] = 0x00
    pkt[5] = PAD
    pkt[6] = 0xAB           # OFF signal
    pkt[7] = left_cur
    pkt[8] = right_cur
    pkt[9] = PAD
    pkt[10] = 0x00
    pkt[11] = right_cur
    pkt[12] = PAD
    pkt[13] = right_cur
    for i in range(14, 19):
        pkt[i] = PAD
    pkt[19] = _checksum(pkt)
    return bytes(pkt)


# Convenience: pre-built handshake with default key
HANDSHAKE = build_handshake()
