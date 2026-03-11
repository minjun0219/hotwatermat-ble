"""Parse BLE notify packets from the hot water mat.

STATUS packet arrives on CHAR1 (~1/sec while connected).
Temperature encoding uses 0.5C precision:
  Values <= 127 are direct degrees (e.g. 33 = 33.0C)
  Values > 127 are half-degree (e.g. 161 = 33.5C, decoded as val - 127.5)
"""

from __future__ import annotations
from dataclasses import dataclass, field as dc_field
from .protocol import (
    DIR_MAT_TO_APP, MODE_POWER, STX, PACKET_SIZE, decode_temp,
)

MODE_NAMES = {
    0x01: "HEAT", 0x02: "TIMER_OFF", 0x03: "SLEEP",
    0x06: "POWER", 0x07: "FASTHEAT", 0x08: "IONCARE",
}


@dataclass
class MatStatus:
    mode: int
    side: int
    left_current: float    # decoded temperature (0.5C precision)
    right_current: float
    left_target: float
    right_target: float
    left_current_raw: int  # raw byte from packet (needed for building commands)
    right_current_raw: int
    volume: int            # 0=mute, 1-3
    water_level: int       # 1=low, 2=ok, 3=full
    raw: bytes = dc_field(repr=False)

    @property
    def mode_name(self) -> str:
        return MODE_NAMES.get(self.mode, f"0x{self.mode:02X}")

    @property
    def is_on(self) -> bool:
        """True if mat is powered on."""
        return not (self.mode == MODE_POWER and
                    self.left_target == 0 and self.right_target == 0)

    def __str__(self) -> str:
        power = "ON" if self.is_on else "OFF"
        water = ["", "💧low", "💧ok", "💧full"]
        wl = water[self.water_level] if self.water_level < len(water) else ""
        return (
            f"Power:{power}  Mode:{self.mode_name}  "
            f"L:{self.left_current}°C→{self.left_target}°C  "
            f"R:{self.right_current}°C→{self.right_target}°C  "
            f"Vol:{self.volume} {wl}"
        )


def parse_status(data: bytes) -> MatStatus | None:
    """Parse a STATUS notify packet. Returns None if not a valid status."""
    if len(data) < PACKET_SIZE:
        return None
    if data[0] != STX or data[1] != DIR_MAT_TO_APP:
        return None
    return MatStatus(
        mode=data[2],
        side=data[3],
        left_current=decode_temp(data[7]),
        right_current=decode_temp(data[8]),
        left_target=decode_temp(data[10]),
        right_target=decode_temp(data[11]),
        left_current_raw=data[7],
        right_current_raw=data[8],
        volume=(data[4] >> 4) & 0x0F,
        water_level=data[4] & 0x0F,
        raw=bytes(data),
    )


# Back-compat alias
def parse_notify(data: bytes) -> MatStatus | None:
    return parse_status(data)
