#!/usr/bin/env python3
"""Basic example: scan, get status, and set temperature."""

import asyncio

from hotwatermat_ble.client import get_status, scan, send_command
from hotwatermat_ble.protocol import DEFAULT_ADDRESS, build_heat, build_power_off


async def main() -> None:
    # Scan for devices
    print("Scanning...")
    devices = await scan(timeout=5.0)
    for d in devices:
        print(f"  Found: {d.address} - {d.name}")

    # Get current status
    print("\nGetting status...")
    status = await get_status(DEFAULT_ADDRESS)
    print(status)

    # Set temperature to 35°C on both sides
    print("\nSetting temp to 35°C...")
    pkt = build_heat(left_temp=35, right_temp=35)
    result = await send_command(DEFAULT_ADDRESS, pkt)
    if result:
        print(result)

    # Power off
    print("\nPowering off...")
    pkts = build_power_off()
    await send_command(DEFAULT_ADDRESS, *pkts, wait_status=False)
    print("Done.")


if __name__ == "__main__":
    asyncio.run(main())
