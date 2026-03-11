"""BLE client for communicating with the hot water mat using bleak."""

from __future__ import annotations

import asyncio

from bleak import BleakClient, BleakScanner
from bleak.backends.device import BLEDevice

from .parser import MatStatus, parse_notify
from .protocol import CHAR_UUID, DEFAULT_ADDRESS, PACKET_SIZE, SERVICE_UUID


async def scan(timeout: float = 10.0) -> list[BLEDevice]:
    """Scan for BLE devices with the hot water mat service UUID."""
    devices = await BleakScanner.discover(
        timeout=timeout,
        service_uuids=[SERVICE_UUID],
    )
    return list(devices)


async def get_status(address: str = DEFAULT_ADDRESS,
                     timeout: float = 10.0) -> MatStatus:
    """Connect to the mat and wait for a notify packet to read status."""
    status: MatStatus | None = None
    event = asyncio.Event()

    def on_notify(_sender: int, data: bytearray) -> None:
        nonlocal status
        parsed = parse_notify(bytes(data))
        if parsed is not None:
            status = parsed
            event.set()

    async with BleakClient(address, timeout=timeout) as client:
        await client.start_notify(CHAR_UUID, on_notify)
        try:
            await asyncio.wait_for(event.wait(), timeout=timeout)
        except asyncio.TimeoutError:
            raise TimeoutError("No status notify received from mat") from None
        finally:
            await client.stop_notify(CHAR_UUID)

    assert status is not None
    return status


async def send_command(address: str, *packets: bytearray,
                       timeout: float = 10.0,
                       wait_status: bool = True) -> MatStatus | None:
    """Connect, send one or more command packets, optionally wait for status.

    Returns the first MatStatus notify received after sending, or None.
    """
    status: MatStatus | None = None
    event = asyncio.Event()

    def on_notify(_sender: int, data: bytearray) -> None:
        nonlocal status
        parsed = parse_notify(bytes(data))
        if parsed is not None:
            status = parsed
            event.set()

    async with BleakClient(address, timeout=timeout) as client:
        if wait_status:
            await client.start_notify(CHAR_UUID, on_notify)

        for pkt in packets:
            assert len(pkt) == PACKET_SIZE
            await client.write_gatt_char(CHAR_UUID, pkt, response=False)
            await asyncio.sleep(0.1)

        if wait_status:
            try:
                await asyncio.wait_for(event.wait(), timeout=5.0)
            except asyncio.TimeoutError:
                pass  # status may not come for power-off
            finally:
                await client.stop_notify(CHAR_UUID)

    return status
