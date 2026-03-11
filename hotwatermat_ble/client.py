"""BLE client for communicating with the hot water mat using bleak.

Connection sequence (verified working on macOS):
  1. BLE connect
  2. Subscribe to CHAR1 (STATUS) and CHAR2 (B2F1 auth)
  3. Wait ~300ms
  4. Write HANDSHAKE to CHAR2 (with response)
  5. Receive B2F1 type=0x02 on CHAR2 -> authenticated
  6. STATUS packets stream on CHAR1 (~1/sec)
  7. Send commands to CHAR2
  8. Disconnect when done (short-lived connections are normal)

All writes go to CHAR2. CHAR1 is STATUS receive only.
"""

from __future__ import annotations

import asyncio
import logging

from bleak import BleakClient, BleakScanner
from bleak.backends.device import BLEDevice

from .parser import MatStatus, parse_status
from .protocol import (
    CHAR1_UUID, CHAR2_UUID, DEFAULT_ADDRESS,
    HANDSHAKE, PACKET_SIZE, SERVICE_UUID,
)

logger = logging.getLogger(__name__)

_CONNECT_TIMEOUT = 30.0
_AUTH_TIMEOUT = 5.0
_STATUS_TIMEOUT = 8.0
_CMD_TIMEOUT = 5.0


async def scan(timeout: float = 10.0) -> list[BLEDevice]:
    """Scan for BLE devices with the hot water mat service UUID."""
    devices = await BleakScanner.discover(
        timeout=timeout,
        service_uuids=[SERVICE_UUID],
    )
    return list(devices)


async def _connect_and_auth(
    address: str,
    timeout: float = _CONNECT_TIMEOUT,
) -> tuple[BleakClient, asyncio.Event, list[MatStatus]]:
    """Connect, subscribe, handshake, and wait for B2F1 auth.

    Returns (client, auth_event, status_list).
    status_list is a mutable list that accumulates incoming STATUS packets.
    """
    auth_event = asyncio.Event()
    statuses: list[MatStatus] = []

    def on_char1_notify(_sender, data: bytearray) -> None:
        """STATUS packets arrive on CHAR1."""
        parsed = parse_status(bytes(data))
        if parsed is not None:
            statuses.append(parsed)
            logger.debug("STATUS: %s", parsed)

    def on_char2_notify(_sender, data: bytearray) -> None:
        """B2F1 auth response arrives on CHAR2."""
        if len(data) >= 3 and data[0] == 0xB2 and data[1] == 0xF1:
            auth_type = data[2]
            logger.debug("B2F1 auth type=0x%02X", auth_type)
            if auth_type == 0x02:
                auth_event.set()

    client = BleakClient(address, timeout=timeout)
    await client.connect()
    logger.debug("Connected to %s", address)

    # Subscribe to both characteristics
    await client.start_notify(CHAR1_UUID, on_char1_notify)
    await client.start_notify(CHAR2_UUID, on_char2_notify)

    # Wait for subscriptions to settle
    await asyncio.sleep(0.3)

    # Send handshake to CHAR2
    await client.write_gatt_char(CHAR2_UUID, bytearray(HANDSHAKE), response=True)
    logger.debug("Handshake sent to CHAR2")

    # Wait for B2F1 auth
    try:
        await asyncio.wait_for(auth_event.wait(), timeout=_AUTH_TIMEOUT)
        logger.debug("Authenticated (B2F1 type=02)")
    except asyncio.TimeoutError:
        await client.disconnect()
        raise TimeoutError("B2F1 auth response not received") from None

    return client, auth_event, statuses


async def get_status(
    address: str = DEFAULT_ADDRESS,
    timeout: float = _CONNECT_TIMEOUT,
) -> MatStatus:
    """Connect, authenticate, and return the first STATUS packet."""
    client, _, statuses = await _connect_and_auth(address, timeout)

    # May already have status from during auth
    if statuses:
        await client.disconnect()
        return statuses[-1]

    # Wait for first STATUS
    event = asyncio.Event()
    orig_len = len(statuses)

    def _check():
        if len(statuses) > orig_len:
            event.set()

    # Poll briefly
    try:
        for _ in range(int(_STATUS_TIMEOUT * 10)):
            if len(statuses) > orig_len:
                break
            await asyncio.sleep(0.1)
    except asyncio.CancelledError:
        pass

    await client.disconnect()

    if statuses:
        return statuses[-1]
    raise TimeoutError("No STATUS packet received from mat")


async def send_command(
    address: str = DEFAULT_ADDRESS,
    *packets: bytes,
    timeout: float = _CONNECT_TIMEOUT,
    wait_status: bool = True,
) -> MatStatus | None:
    """Connect, authenticate, send command(s), and optionally wait for STATUS.

    Returns the latest MatStatus after sending, or None.
    """
    client, _, statuses = await _connect_and_auth(address, timeout)

    # Wait for at least one STATUS before sending commands
    if wait_status:
        for _ in range(int(_STATUS_TIMEOUT * 10)):
            if statuses:
                break
            await asyncio.sleep(0.1)

    # Send command packets to CHAR2
    for pkt in packets:
        if len(pkt) != PACKET_SIZE:
            raise ValueError(f"Packet must be {PACKET_SIZE} bytes, got {len(pkt)}")
        await client.write_gatt_char(CHAR2_UUID, bytearray(pkt), response=True)
        logger.debug("Command sent to CHAR2: %s", pkt.hex())
        await asyncio.sleep(0.1)

    # Wait for updated STATUS
    result = None
    if wait_status:
        pre_count = len(statuses)
        for _ in range(int(_CMD_TIMEOUT * 10)):
            if len(statuses) > pre_count:
                break
            await asyncio.sleep(0.1)
        if statuses:
            result = statuses[-1]

    await client.disconnect()
    return result
