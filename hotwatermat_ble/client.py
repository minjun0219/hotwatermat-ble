"""BLE client for communicating with the hot water mat using bleak.

Connection method (confirmed working on macOS):
  1. Open the vendor app on the phone and connect the mat.
  2. Run get_status() or send_command() — bleak uses UUID cache to connect
     even while the phone is the primary BLE connection.
  3. Swipe-close (not force-stop) the phone app.
  4. The mat briefly continues sending notify packets to all subscribed centrals.

Note: write commands work only when Mac is the sole/primary connection.
      Status reading works while the phone is also connected.
"""

from __future__ import annotations

import asyncio
import logging

from bleak import BleakClient, BleakScanner
from bleak.backends.device import BLEDevice

from .parser import MatStatus, parse_notify
from .protocol import (
    CHAR_UUID, CHAR2_UUID, DEFAULT_ADDRESS,
    HANDSHAKE, PACKET_SIZE, SERVICE_UUID,
)

logger = logging.getLogger(__name__)

_CONNECT_TIMEOUT = 30.0   # long: allows waiting while phone disconnects
_STATUS_TIMEOUT  = 8.0
_WRITE_TIMEOUT   = 5.0


async def scan(timeout: float = 10.0) -> list[BLEDevice]:
    """Scan for BLE devices with the hot water mat service UUID."""
    devices = await BleakScanner.discover(
        timeout=timeout,
        service_uuids=[SERVICE_UUID],
    )
    return list(devices)


def _get_chars(client: BleakClient):
    """Return (char1, char2) characteristic objects."""
    char1 = char2 = None
    for svc in client.services:
        for ch in svc.characteristics:
            if ch.uuid == CHAR_UUID:
                char1 = ch
            elif ch.uuid == CHAR2_UUID:
                char2 = ch
    return char1, char2


async def get_status(address: str = DEFAULT_ADDRESS,
                     timeout: float = _CONNECT_TIMEOUT) -> MatStatus:
    """Connect and return the first valid status packet from the mat.

    Requires the phone app to be connected (or recently disconnected).
    Uses UUID cache to connect without advertising.
    """
    status: MatStatus | None = None
    event = asyncio.Event()

    def on_notify(_sender, data: bytearray) -> None:
        nonlocal status
        parsed = parse_notify(bytes(data))
        if parsed is not None and status is None:
            status = parsed
            event.set()

    client = BleakClient(address, timeout=timeout)
    await client.connect()
    logger.debug("Connected to mat")

    char1, char2 = _get_chars(client)
    if char1 is None:
        await client.disconnect()
        raise RuntimeError(f"Characteristic {CHAR_UUID} not found")

    await client.start_notify(char1, on_notify)
    if char2:
        await client.start_notify(char2, on_notify)

    # Send handshake to activate notification stream
    await client.write_gatt_char(char1, bytearray(HANDSHAKE), response=True)
    logger.debug("Handshake sent")

    try:
        await asyncio.wait_for(event.wait(), timeout=_STATUS_TIMEOUT)
    except asyncio.TimeoutError:
        await client.disconnect()
        raise TimeoutError("No status notify received from mat") from None

    await client.disconnect()
    assert status is not None
    return status


async def send_command(address: str = DEFAULT_ADDRESS,
                       *packets: bytearray,
                       timeout: float = _CONNECT_TIMEOUT,
                       wait_status: bool = True) -> MatStatus | None:
    """Connect as primary device and send command packet(s).

    NOTE: Commands are only accepted when Mac is the sole BLE connection.
    Before calling, ensure the phone app is disconnected from the mat.

    Returns the first MatStatus received after sending, or None.
    """
    status: MatStatus | None = None
    event = asyncio.Event()

    def on_notify(_sender, data: bytearray) -> None:
        nonlocal status
        parsed = parse_notify(bytes(data))
        if parsed is not None and status is None:
            status = parsed
            event.set()

    client = BleakClient(address, timeout=timeout)
    await client.connect()
    logger.debug("Connected to mat (primary mode)")

    char1, char2 = _get_chars(client)
    if char1 is None:
        await client.disconnect()
        raise RuntimeError(f"Characteristic {CHAR_UUID} not found")

    if wait_status:
        await client.start_notify(char1, on_notify)
        if char2:
            await client.start_notify(char2, on_notify)

    await client.write_gatt_char(char1, bytearray(HANDSHAKE), response=True)
    logger.debug("Handshake sent")

    # Wait briefly for auth (B2F1) then send commands
    await asyncio.sleep(0.5)

    for pkt in packets:
        if len(pkt) != PACKET_SIZE:
            raise ValueError(f"Packet must be {PACKET_SIZE} bytes, got {len(pkt)}")
        await client.write_gatt_char(char1, pkt, response=True)
        logger.debug("Command sent: %s", pkt.hex())
        await asyncio.sleep(0.1)

    if wait_status:
        try:
            await asyncio.wait_for(event.wait(), timeout=_WRITE_TIMEOUT)
        except asyncio.TimeoutError:
            pass  # status may not arrive for power-off

    await client.disconnect()
    return status
