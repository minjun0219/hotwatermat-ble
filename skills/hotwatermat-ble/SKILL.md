---
name: hotwatermat-ble
description: Control a BLE hot water mat (온수매트, BLE name KDO_HotWaterMat, model EQM555) from the terminal with the hotwatermat-ble CLI — read its status (current and target temperature per side, mode, water level), set the left/right target temperature (28.0-48.0°C, 0.5°C steps), and power it on or off, with no vendor app, Wi-Fi or cloud. Use when the user asks to turn the mat (매트, 온수매트) on or off, warm it up or cool it down, check how warm it is or whether the water is low, or to script or schedule the mat; also when working with the Go packages in github.com/minjun0219/hotwatermat-ble.
license: MIT
compatibility: macOS (amd64, arm64; CoreBluetooth) or Linux (amd64 release binary; BlueZ over D-Bus) with a Bluetooth LE adapter in range of the mat. Windows only through WSL. Building from source needs Go 1.22+ with cgo.
---

# hotwatermat-ble

`hotwatermat-ble` talks to the mat directly over Bluetooth LE: connect → authenticate with the
device key (DeviceGid) → read status or send one command → disconnect. Every CLI call is one short
connection. Repository: https://github.com/minjun0219/hotwatermat-ble · protocol:
https://github.com/minjun0219/hotwatermat-ble/blob/main/PROTOCOL.md

## Before the first command

1. Check the binary: `hotwatermat-ble version`. If it is missing, install it:

   ```sh
   curl -fsSL https://raw.githubusercontent.com/minjun0219/hotwatermat-ble/main/install.sh | bash
   ```

   It installs into `~/.local/bin` (override with `INSTALL_DIR`). Release binaries exist for macOS
   amd64/arm64 and Linux amd64; elsewhere build from source:
   `go build -o hotwatermat-ble ./cmd/hotwatermat-ble/`.

2. Check that the device is configured. The CLI needs a BLE address and a DeviceGid (a 12-character
   hex authentication key). It resolves each in this order: flag (`--address`, `--device-gid`) →
   environment (`HOTWATERMAT_ADDRESS`, `HOTWATERMAT_DEVICE_GID`) → the config file saved by `setup`
   (`<user config dir>/hotwatermat-ble/device.json`) → for the address only, a 5-second scan that
   picks the mat if exactly one is found.

3. If there is no DeviceGid anywhere, the error says
   `no device GID specified. Run 'hotwatermat-ble setup' ...`. `setup` is interactive (it reads
   choices from stdin and may need the mat in pairing mode), so **ask the user to run
   `hotwatermat-ble setup` in their own terminal** rather than driving it yourself.

## Commands

| Goal | Command |
|---|---|
| Show status | `hotwatermat-ble status` |
| Set both sides | `hotwatermat-ble temp --left 35 --right 35` |
| Set one side (the other keeps its target) | `hotwatermat-ble temp --left 33.5` / `hotwatermat-ble temp --right 40` |
| Power on (the mat restores its previous targets) | `hotwatermat-ble on` |
| Power off | `hotwatermat-ble off` |
| Find mats nearby (no key needed) | `hotwatermat-ble scan` |
| Version | `hotwatermat-ble version` |

- Add `--verify` to `temp`, `on` and `off` to read the status again after the command and print it.
  Prefer it: the command lines only say the packet was sent (`Power ON sent.`,
  `Temperature set: left=35.0°C right=35.0°C`), not that the mat accepted it.
- Temperatures outside 28.0-48.0 are rejected before connecting. Only whole or half degrees are
  accepted (`33.3` fails after connecting), so round the user's request to the nearest 0.5.
- `--debug` prints the BLE exchange; use it only when diagnosing a failure.

## Reading the status

A powered-off mat prints one line:

```
Power: OFF
```

A running mat prints:

```
Mode:    HEAT
Side:    both
Water:   OK
Left:    31.5°C → 35.0°C
Right:   30.0°C → 35.0°C
```

- `Left` / `Right` are `current → target`.
- `Mode` is one of `HEAT`, `TIMER_OFF`, `SLEEP`, `POWER`, `FASTHEAT`, `IONCARE`.
- `Water` is `LOW` (tell the user to refill), `OK` or `FULL`.
- Lines such as `Using cached address: ...` go to stderr; parse stdout only.
- On failure the exit code is non-zero and stderr has an `Error: ...` line.

## Rules for agents

- Change power or temperature only when the user asked for it, and set exactly the temperature they
  asked for. This is a heating appliance someone may be lying on; do not raise the temperature on
  your own initiative.
- Run commands one at a time. The mat accepts a single BLE connection, so parallel calls (or the
  vendor app being open) make the next call fail to find the device.
- Each call takes a few seconds (connect, authenticate, a 2-second pause after a command, plus 5
  seconds of scanning when no address is known). A failing connect retries for up to about 40
  seconds, so give the command a timeout of at least 60 seconds.
- The DeviceGid is the mat's authentication key. Do not print it in full, write it into files you
  commit, or paste it into issues and logs. `setup` itself only shows it masked (`13****5A`).
- On macOS the address is a CoreBluetooth UUID (`FD319CFA-2E62-...`), not a MAC address, and it is
  different on every Mac. Do not reuse an address from another machine.

## Troubleshooting

| Symptom | Likely cause and fix |
|---|---|
| `device not found after 3 attempts` | The vendor app is still connected (force-close it), the mat is out of range, or it stopped advertising (press a button on its remote). Then retry once. |
| `multiple devices found, specify --address` | Run `hotwatermat-ble scan`, ask the user which one, pass `--address`. |
| `authentication timeout (wrong DeviceGid?)` | The key does not match this mat (the mat drops a wrong key after about 4.5 s). Ask the user to re-run `setup`. |
| `scan failed` / Bluetooth permission error on macOS | Allow Bluetooth for the terminal app: System Settings → Privacy & Security → Bluetooth. |
| macOS says the binary is from an unidentified developer | `xattr -d com.apple.quarantine "$(command -v hotwatermat-ble)"` |
| `invalid device-gid` | It must be 12 hex characters (6 bytes), e.g. `13CE3CC53E5A`. |

## Using the Go packages

`pkg/protocol` builds and parses the 20-byte packets and has no BLE dependency; `pkg/ble` wraps the
connection. A command must go out over the same connection that read the status, which `pkg/ble`
handles:

```go
import (
	"fmt"
	"os"
	"time"

	"github.com/minjun0219/hotwatermat-ble/pkg/ble"
	"github.com/minjun0219/hotwatermat-ble/pkg/protocol"
)

gid, err := protocol.ParseDeviceGidHex(os.Getenv("HOTWATERMAT_DEVICE_GID"))
if err != nil {
	return err
}
c := ble.NewClient(address, gid, false)
if err := c.Connect(); err != nil { // connect + handshake + wait for the auth response
	return err
}
defer c.Disconnect()

st, err := c.GetStatus(5 * time.Second)
if err != nil {
	return err
}
if !st.PoweredOff {
	fmt.Printf("left %.1f → %.1f°C\n", st.LeftCurrent, st.LeftTarget)
}
err = c.SetTemp(protocol.SideBoth, 35, 35) // or c.PowerOn() / c.PowerOff()
```

If you change the protocol code, keep these invariants (they are tested in `pkg/protocol`):
packets are always 20 bytes; the checksum is `sum(bytes[1:19]) & 0xFF` at byte 19; whole degrees
encode as themselves and `x.5` encodes as `temp + 127.5` (bit 7 is not a heating flag); status
arrives on the `…0100dd` characteristic and every write goes to `…0200dd`.
