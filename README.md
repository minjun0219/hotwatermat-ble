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

---

## 실기기 테스트 결과 (2026-03-11)

### 확인된 동작
- ✅ **BLE 상태 수신**: 온도, 물 수위, 가열 여부 정상 수신
- ✅ **연결 방법**: 앱 force-stop → BleakScanner → connect → handshake → notify
- ✅ **핸드쉐이크**: `B2 01 4F 5F C6 10 B1 69 FE...89` (고정값 확인)
- ✅ **연결 초기화 시퀀스**: CCCD subscribe → handshake → B2F1 응답 → 상태 스트리밍

### macOS 연결 제약 (진행 중)
- 매트는 **단일 BLE 연결**만 지원 (연결 중엔 advertising 안 함)
- bleak UUID 캐시로 non-advertising 기기 연결 가능 (앱 force-stop 직후 타이밍)
- **상태 읽기**: 정상 동작 ✅
- **명령 쓰기**: B2F1 인증 완료 필요 (macOS CoreBluetooth 이슈 조사 중)

### 연결 시퀀스
```python
# 앱 연결 상태에서:
# 1. adb shell am force-stop <pkg>
# 2. BleakScanner로 매트 발견 (advertising 시작)
# 3. BleakClient.connect() → start_notify → handshake 전송
# 4. 상태 notify 수신 시작 (좌/우 온도, 물 수위 등)
```
