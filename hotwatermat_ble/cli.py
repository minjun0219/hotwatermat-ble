"""Click CLI for hotwatermat-ble."""

from __future__ import annotations

import asyncio
import sys

import click

from .client import get_status, scan, send_command
from .protocol import (
    DEFAULT_ADDRESS, SIDE_BOTH, SIDE_LEFT, SIDE_RIGHT,
    TEMP_MAX, TEMP_MIN,
    build_heat, build_power_off, build_power_on,
)


class FloatTempType(click.ParamType):
    """Custom type for 0.5C step temperatures."""
    name = "TEMP"

    def convert(self, value, param, ctx):
        try:
            v = float(value)
        except (ValueError, TypeError):
            self.fail(f"{value!r} is not a valid temperature", param, ctx)
        if v < TEMP_MIN or v > TEMP_MAX:
            self.fail(f"Temperature must be {TEMP_MIN}-{TEMP_MAX}°C", param, ctx)
        if (v * 2) != int(v * 2):
            self.fail("Temperature must be in 0.5°C steps", param, ctx)
        return v


TEMP_TYPE = FloatTempType()


@click.group()
@click.option(
    "--address", "-a",
    envvar="HOTWATERMAT_ADDRESS",
    default=DEFAULT_ADDRESS,
    show_default=True,
    help="BLE address of the mat (or set HOTWATERMAT_ADDRESS env var).",
)
@click.option("--debug", is_flag=True, help="Enable debug logging.")
@click.pass_context
def cli(ctx: click.Context, address: str, debug: bool) -> None:
    """Control a BLE hot water mat (KDO_HotWaterMat)."""
    ctx.ensure_object(dict)
    ctx.obj["address"] = address
    if debug:
        import logging
        logging.basicConfig(level=logging.DEBUG)


@cli.command("scan")
@click.option("--timeout", "-t", default=10.0, help="Scan timeout in seconds.")
def scan_cmd(timeout: float) -> None:
    """Scan for hot water mat BLE devices."""
    click.echo(f"Scanning for devices ({timeout}s)...")
    devices = asyncio.run(scan(timeout=timeout))
    if not devices:
        click.echo("No devices found.")
        return
    for d in devices:
        click.echo(f"  {d.address}  {d.name or '(unknown)'}")


@cli.command()
@click.pass_context
def status(ctx: click.Context) -> None:
    """Show current mat status."""
    address = ctx.obj["address"]
    click.echo(f"Connecting to {address}...")
    try:
        st = asyncio.run(get_status(address))
    except TimeoutError as e:
        click.echo(f"Error: {e}", err=True)
        sys.exit(1)
    except Exception as e:
        click.echo(f"Connection error: {e}", err=True)
        sys.exit(1)
    click.echo(str(st))
    click.echo(f"\nRaw: {st.raw.hex(' ')}")


@cli.command("temp")
@click.option("--left", "-l", type=TEMP_TYPE,
              help=f"Left side temp ({TEMP_MIN}-{TEMP_MAX}°C, 0.5 steps).")
@click.option("--right", "-r", type=TEMP_TYPE,
              help=f"Right side temp ({TEMP_MIN}-{TEMP_MAX}°C, 0.5 steps).")
@click.pass_context
def set_temp(ctx: click.Context, left: float | None, right: float | None) -> None:
    """Set target temperature for left/right sides.

    At least one of --left or --right must be specified.
    Unspecified side keeps its current temperature.
    """
    if left is None and right is None:
        click.echo("Error: specify at least one of --left or --right", err=True)
        sys.exit(1)

    address = ctx.obj["address"]
    click.echo(f"Connecting to {address}...")

    # Get current status first (need raw bytes for command)
    try:
        st = asyncio.run(get_status(address))
    except Exception as e:
        click.echo(f"Error getting status: {e}", err=True)
        sys.exit(1)

    left_tgt = left if left is not None else st.left_target
    right_tgt = right if right is not None else st.right_target

    # Determine side
    if left is not None and right is not None:
        side = SIDE_BOTH
    elif left is not None:
        side = SIDE_LEFT
    else:
        side = SIDE_RIGHT

    pkt = build_heat(
        left_cur=st.left_current_raw,
        right_cur=st.right_current_raw,
        left_tgt=left_tgt,
        right_tgt=right_tgt,
        side=side,
    )

    parts = []
    if left is not None:
        parts.append(f"L={left}°C")
    if right is not None:
        parts.append(f"R={right}°C")
    click.echo(f"Setting temp: {', '.join(parts)}")

    try:
        result = asyncio.run(send_command(address, pkt))
    except Exception as e:
        click.echo(f"Error: {e}", err=True)
        sys.exit(1)

    if result:
        click.echo(str(result))
    else:
        click.echo("Command sent (no status confirmation).")


@cli.command("on")
@click.pass_context
def power_on(ctx: click.Context) -> None:
    """Turn mat power on."""
    address = ctx.obj["address"]
    click.echo(f"Connecting to {address}...")

    try:
        st = asyncio.run(get_status(address))
    except Exception as e:
        click.echo(f"Error: {e}", err=True)
        sys.exit(1)

    pkt = build_power_on(st.left_current_raw, st.right_current_raw)
    click.echo("Powering on...")
    try:
        result = asyncio.run(send_command(address, pkt))
    except Exception as e:
        click.echo(f"Error: {e}", err=True)
        sys.exit(1)

    if result:
        click.echo(str(result))
    else:
        click.echo("Power on command sent.")


@cli.command("off")
@click.pass_context
def power_off(ctx: click.Context) -> None:
    """Turn mat power off.

    WARNING: After power off, the mat stops BLE advertising.
    Physical button required to turn back on.
    """
    address = ctx.obj["address"]
    click.echo(f"Connecting to {address}...")

    try:
        st = asyncio.run(get_status(address))
    except Exception as e:
        click.echo(f"Error: {e}", err=True)
        sys.exit(1)

    pkt = build_power_off(st.left_current_raw, st.right_current_raw)
    click.echo("⚠️  Powering off (BLE will be unavailable until physical restart)...")
    try:
        result = asyncio.run(send_command(address, pkt, wait_status=False))
    except Exception as e:
        click.echo(f"Error: {e}", err=True)
        sys.exit(1)

    click.echo("Power off command sent.")
