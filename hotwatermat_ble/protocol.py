"""BLE protocol constants and packet builders for KDO_HotWaterMat / EQM555."""

from __future__ import annotations

PACKET_SIZE = 20

STX = 0xB2
DIR_APP_TO_MAT = 0x80
DIR_MAT_TO_APP = 0x00

SERVICE_UUID = "00001c0d-d102-11e1-9b23-2ce2a80000dd"
CHAR_UUID = "00001c0d-d102-11e1-9b23-2ce2a80100dd"

DEFAULT_ADDRESS = "FD319CFA-2E62-116D-D348-5B9FEEE95D2F"

# Mode byte values (byte[2])
MODE_HEAT = 0x01
MODE_TIMER_OFF = 0x02
MODE_SLEEP = 0x03
MODE_POWER_OFF = 0x06  # also TIMER_ON
MODE_FASTHEAT = 0x07
MODE_IONCARE = 0x08

# Side byte values (byte[3])
SIDE_LEFT = 0x02
SIDE_BOTH = 0x06

# Right-side state (byte[5])
RIGHT_HEAT = 0x02
RIGHT_STANDBY = 0x03
RIGHT_OFF = 0xFE

# Temperature range
TEMP_MIN = 28
TEMP_MAX = 48

# Fixed bytes
FIXED_BYTE6 = 0x25
FIXED_BYTE9 = 0x23
PAD = 0xFE

# Sequence values (byte[19])
SEQ_A = 0x3C
SEQ_B = 0xBC

_seq_toggle = False


def _next_seq() -> int:
    global _seq_toggle
    _seq_toggle = not _seq_toggle
    return SEQ_B if _seq_toggle else SEQ_A


def _base_packet(mode: int, side: int, right_state: int,
                 left_temp: int, right_temp: int,
                 vol: int = 0, sub: int = PAD) -> bytearray:
    pkt = bytearray(PACKET_SIZE)
    pkt[0] = STX
    pkt[1] = DIR_APP_TO_MAT
    pkt[2] = mode
    pkt[3] = side
    pkt[4] = (vol & 0x0F) << 4  # volume in high nibble, water level 0
    pkt[5] = right_state
    pkt[6] = FIXED_BYTE6
    pkt[7] = left_temp
    pkt[8] = right_temp
    pkt[9] = FIXED_BYTE9
    pkt[10] = PAD
    pkt[11] = PAD
    pkt[12] = sub
    for i in range(13, 19):
        pkt[i] = PAD
    pkt[19] = _next_seq()
    return pkt


def build_heat(left_temp: int, right_temp: int, side: int = SIDE_BOTH,
               vol: int = 0) -> bytearray:
    """Build a HEAT mode packet."""
    return _base_packet(MODE_HEAT, side, RIGHT_HEAT, left_temp, right_temp, vol)


def build_set_temp(left_temp: int, right_temp: int, side: int = SIDE_BOTH,
                   vol: int = 0) -> bytearray:
    """Build a temperature-set packet (same as heat command)."""
    return build_heat(left_temp, right_temp, side, vol)


def build_fastheat(on: bool, left_temp: int = 45, right_temp: int = 45,
                   side: int = SIDE_BOTH, vol: int = 0) -> bytearray:
    """Build a FASTHEAT mode packet."""
    sub = 0x01 if on else 0x00
    return _base_packet(MODE_FASTHEAT, side, RIGHT_HEAT, left_temp, right_temp, vol, sub)


def build_standby(left_temp: int = 40, right_temp: int = 40,
                  side: int = SIDE_BOTH, vol: int = 0) -> bytearray:
    """Build a SLEEP/standby mode packet."""
    return _base_packet(MODE_SLEEP, side, RIGHT_STANDBY, left_temp, right_temp, vol)


def build_power_on(left_temp: int = 40, right_temp: int = 40,
                   vol: int = 0) -> bytearray:
    """Build a power-on (HEAT) packet."""
    return build_heat(left_temp, right_temp, SIDE_BOTH, vol)


def build_power_off() -> list[bytearray]:
    """Build power-off packets (two packets required).

    First packet: side=0x00, second: side=0x02.
    """
    pkts = []
    for side_val in (0x00, SIDE_LEFT):
        pkt = bytearray(PACKET_SIZE)
        pkt[0] = STX
        pkt[1] = DIR_APP_TO_MAT
        pkt[2] = MODE_POWER_OFF
        pkt[3] = side_val
        pkt[4] = 0x00
        pkt[5] = RIGHT_OFF
        pkt[6] = FIXED_BYTE6
        pkt[7] = 0x00
        pkt[8] = 0x00
        pkt[9] = FIXED_BYTE9
        pkt[10] = 0x00
        pkt[11] = 0x00
        pkt[12] = PAD
        for i in range(13, 19):
            pkt[i] = PAD
        pkt[19] = _next_seq()
        pkts.append(pkt)
    return pkts
