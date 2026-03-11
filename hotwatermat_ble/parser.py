"""Parse notify packets from the hot water mat."""

from __future__ import annotations

from dataclasses import dataclass

from .protocol import (
    DIR_MAT_TO_APP, MODE_HEAT, MODE_TIMER_OFF, MODE_SLEEP,
    MODE_POWER_OFF, MODE_FASTHEAT, MODE_IONCARE, PACKET_SIZE, STX,
)

MODE_NAMES = {
    MODE_HEAT: "HEAT",
    MODE_TIMER_OFF: "TIMER_OFF",
    MODE_SLEEP: "SLEEP",
    MODE_POWER_OFF: "POWER_OFF",
    MODE_FASTHEAT: "FASTHEAT",
    MODE_IONCARE: "IONCARE",
}

RIGHT_STATE_NAMES = {
    0x02: "HEAT",
    0x03: "STANDBY",
    0xFE: "OFF",
}

WATER_LEVEL_NAMES = {
    1: "LOW",
    2: "OK",
    3: "FULL",
}


@dataclass
class MatStatus:
    mode: int
    mode_name: str
    side: int
    volume: int
    water_level: int
    water_level_name: str
    right_state: int
    right_state_name: str
    left_set_temp: int
    right_set_temp: int
    left_cur_temp: int
    right_cur_temp: int
    left_heating: bool
    right_heating: bool
    fastheat_on: bool
    raw: bytes

    def __str__(self) -> str:
        lines = [
            f"Mode:         {self.mode_name}",
            f"Side:         {'both' if self.side == 0x06 else 'left'}",
            f"Volume:       {self.volume}",
            f"Water level:  {self.water_level_name}",
            f"Right state:  {self.right_state_name}",
            f"Left  temp:   {self.left_cur_temp}°C → {self.left_set_temp}°C {'(heating)' if self.left_heating else ''}",
            f"Right temp:   {self.right_cur_temp}°C → {self.right_set_temp}°C {'(heating)' if self.right_heating else ''}",
        ]
        if self.mode == MODE_FASTHEAT:
            lines.append(f"Fast heat:    {'ON' if self.fastheat_on else 'OFF'}")
        return "\n".join(lines)


def parse_notify(data: bytes) -> MatStatus | None:
    """Parse a 20-byte notify packet from the mat. Returns None if invalid."""
    if len(data) < PACKET_SIZE:
        return None
    if data[0] != STX:
        return None
    if data[1] != DIR_MAT_TO_APP:
        return None

    mode = data[2]
    side = data[3]
    vol = (data[4] >> 4) & 0x0F
    wlv = data[4] & 0x0F
    right_state = data[5]
    left_set = data[7]
    right_set = data[8]

    left_cur_raw = data[10]
    right_cur_raw = data[11]
    left_heating = bool(left_cur_raw & 0x80)
    right_heating = bool(right_cur_raw & 0x80)
    left_cur = left_cur_raw & 0x7F
    right_cur = right_cur_raw & 0x7F

    fastheat_on = (mode == MODE_FASTHEAT and data[12] == 0x01)

    return MatStatus(
        mode=mode,
        mode_name=MODE_NAMES.get(mode, f"UNKNOWN(0x{mode:02X})"),
        side=side,
        volume=vol,
        water_level=wlv,
        water_level_name=WATER_LEVEL_NAMES.get(wlv, f"UNKNOWN({wlv})"),
        right_state=right_state,
        right_state_name=RIGHT_STATE_NAMES.get(right_state, f"UNKNOWN(0x{right_state:02X})"),
        left_set_temp=left_set,
        right_set_temp=right_set,
        left_cur_temp=left_cur,
        right_cur_temp=right_cur,
        left_heating=left_heating,
        right_heating=right_heating,
        fastheat_on=fastheat_on,
        raw=bytes(data),
    )
