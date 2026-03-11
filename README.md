# hotwatermat-ble

Python CLI to control a BLE hot water mat (KDO_HotWaterMat / EQM555).

## Installation

```bash
pip install -e .
```

Or install dependencies directly:

```bash
pip install bleak click
```

## Usage

### CLI Commands

```bash
# Scan for devices
hotwatermat-ble scan

# Show current status
hotwatermat-ble status

# Set temperature (28-48°C)
hotwatermat-ble set-temp --left 35 --right 35

# Power on/off
hotwatermat-ble power on
hotwatermat-ble power off

# Fast heat on/off
hotwatermat-ble fastheat on
hotwatermat-ble fastheat off

# Standby mode
hotwatermat-ble standby
```

### Options

```bash
# Specify BLE address
hotwatermat-ble --address <ADDRESS> status

# Or use environment variable
export HOTWATERMAT_ADDRESS=FD319CFA-2E62-116D-D348-5B9FEEE95D2F
hotwatermat-ble status
```

### Run as module

```bash
python3 -m hotwatermat_ble status
```

## BLE Protocol

- Service UUID: `00001c0d-d102-11e1-9b23-2ce2a80000dd`
- Characteristic UUID: `00001c0d-d102-11e1-9b23-2ce2a80100dd`
- Packet size: 20 bytes fixed, write without response
- Device sends periodic notify packets on connect

## License

MIT
