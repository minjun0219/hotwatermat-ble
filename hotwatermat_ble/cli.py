"""Click CLI for hotwatermat-ble."""

from __future__ import annotations

import asyncio
import sys

import click

from .client import get_status, scan, send_command
from .protocol import (
    DEFAULT_ADDRESS, SIDE_BOTH, TEMP_MAX, TEMP_MIN,
    build_fastheat, build_heat, build_power_off, build_power_on, build_standby,
)


@click.group()
@click.option(
    "--address", "-a",
    envvar="HOTWATERMAT_ADDRESS",
    default=DEFAULT_ADDRESS,
    show_default=True,
    help="BLE address of the mat (or set HOTWATERMAT_ADDRESS env var).",
)
@click.pass_context
def cli(ctx: click.Context, address: str) -> None:
    """Control a BLE hot water mat (KDO_HotWaterMat / EQM555)."""
    ctx.ensure_object(dict)
    ctx.obj["address"] = address


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
    """Connect and show current mat status."""
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


@cli.command("set-temp")
@click.option("--left", "-l", type=click.IntRange(TEMP_MIN, TEMP_MAX),
              required=True, help=f"Left temp ({TEMP_MIN}-{TEMP_MAX}°C).")
@click.option("--right", "-r", type=click.IntRange(TEMP_MIN, TEMP_MAX),
              required=True, help=f"Right temp ({TEMP_MIN}-{TEMP_MAX}°C).")
@click.pass_context
def set_temp(ctx: click.Context, left: int, right: int) -> None:
    """Set left and right temperatures."""
    address = ctx.obj["address"]
    pkt = build_heat(left, right, SIDE_BOTH)
    click.echo(f"Setting temp: left={left}°C, right={right}°C")
    st = asyncio.run(send_command(address, pkt))
    if st:
        click.echo(str(st))
    else:
        click.echo("Command sent (no status received).")


@cli.command()
@click.argument("state", type=click.Choice(["on", "off"]))
@click.pass_context
def power(ctx: click.Context, state: str) -> None:
    """Turn power on or off."""
    address = ctx.obj["address"]
    if state == "on":
        pkt = build_power_on()
        click.echo("Powering on...")
        st = asyncio.run(send_command(address, pkt))
    else:
        pkts = build_power_off()
        click.echo("Powering off...")
        st = asyncio.run(send_command(address, *pkts, wait_status=False))
    if st:
        click.echo(str(st))
    else:
        click.echo("Command sent.")


@cli.command()
@click.argument("state", type=click.Choice(["on", "off"]))
@click.pass_context
def fastheat(ctx: click.Context, state: str) -> None:
    """Turn fast heat on or off."""
    address = ctx.obj["address"]
    pkt = build_fastheat(on=(state == "on"))
    click.echo(f"Fast heat {state}...")
    st = asyncio.run(send_command(address, pkt))
    if st:
        click.echo(str(st))
    else:
        click.echo("Command sent.")


@cli.command()
@click.pass_context
def standby(ctx: click.Context) -> None:
    """Set mat to standby (sleep) mode."""
    address = ctx.obj["address"]
    pkt = build_standby()
    click.echo("Setting standby mode...")
    st = asyncio.run(send_command(address, pkt))
    if st:
        click.echo(str(st))
    else:
        click.echo("Command sent.")
