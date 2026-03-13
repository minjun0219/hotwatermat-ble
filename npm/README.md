# hotwatermat-ble

BLE 온수매트(KDO_HotWaterMat / EQM555) 프로토콜의 JavaScript/TypeScript 구현체입니다.

Go 프로토콜 라이브러리를 WASM으로 컴파일하여 브라우저 및 Node.js에서 사용할 수 있습니다.

## 설치

```bash
npm install hotwatermat-ble
```

## CLI 사용법

설치 없이 `npx`로 바로 실행하거나, 글로벌 설치 후 사용할 수 있습니다.

```bash
# 설치 없이 바로 실행
npx hotwatermat-ble --help

# 글로벌 설치
npm install -g hotwatermat-ble
hotwatermat-ble --help
```

### BLE 명령어 (네이티브 바이너리 필요)

설치 시 OS/arch에 맞는 네이티브 바이너리를 자동으로 다운로드합니다.

```bash
# 기기 스캔
npx hotwatermat-ble scan

# 현재 상태 확인
npx hotwatermat-ble status

# 온도 설정
npx hotwatermat-ble temp --left 35 --right 35

# 전원 켜기/끄기
npx hotwatermat-ble on
npx hotwatermat-ble off
```

### 프로토콜 명령어 (WASM, BLE 불필요)

네이티브 바이너리 없이도 프로토콜 패킷 생성/파싱이 가능합니다.

```bash
# 온도 인코딩/디코딩
npx hotwatermat-ble encode-temp 35.0    # → 163
npx hotwatermat-ble decode-temp 163     # → 35

# 패킷 생성 (hex 출력)
npx hotwatermat-ble build-handshake
npx hotwatermat-ble build-heat both 35.0 35.0
npx hotwatermat-ble build-power-on
npx hotwatermat-ble build-power-off

# 상태 패킷 파싱 (20바이트 hex 입력)
npx hotwatermat-ble parse-status <40자리hex>

# 체크섬 계산
npx hotwatermat-ble checksum <hex>
```

## 라이브러리 사용법

Node.js/TypeScript에서 라이브러리로 직접 사용할 수 있습니다.

```typescript
import {
  init,
  encodeTemp,
  decodeTemp,
  buildHeat,
  buildHandshake,
  buildPowerOn,
  buildPowerOff,
  parseStatus,
  SIDE_BOTH,
  SIDE_LEFT,
  SIDE_RIGHT
} from 'hotwatermat-ble';

// WASM 초기화 (최초 1회)
await init();

// 온도 인코딩/디코딩
const encoded = encodeTemp(35.0);   // 163
const decoded = decodeTemp(163);     // 35.0

// 온도 설정 패킷 생성
const packet = buildHeat(SIDE_BOTH, 0x21, 0x1C, encoded, encoded);
// → Uint8Array(20) — BLE로 전송할 패킷

// 핸드쉐이크 패킷 생성
const handshake = buildHandshake('13CE3CC53E5A');

// 전원 제어
const powerOnPkt = buildPowerOn(0x21, 0x1C);
const powerOffPkt = buildPowerOff(0x21, 0x1C);

// 상태 파싱 (BLE에서 수신한 20바이트 패킷)
const status = parseStatus(statusPacket);
console.log(status);
// {
//   mode: 1,
//   modeName: "HEAT",
//   leftCurrent: 33.0,
//   rightCurrent: 30.0,
//   leftTarget: 35.0,
//   rightTarget: 35.0,
//   waterLevel: 3,
//   poweredOff: false
// }
```

## API

### `init(): Promise<void>`
WASM 모듈을 로드합니다. 다른 함수 호출 전에 반드시 실행해야 합니다.

### `encodeTemp(temp: number): number`
온도(°C)를 프로토콜 바이트로 인코딩합니다. 범위: 28.0~48.0°C, 0.5°C 단위.

### `decodeTemp(byte: number): number`
프로토콜 바이트를 온도(°C)로 디코딩합니다.

### `buildHeat(side, leftCur, rightCur, leftTarget, rightTarget): Uint8Array`
온도 설정 패킷(20바이트)을 생성합니다.

### `buildHandshake(deviceGid?: string): Uint8Array`
인증 핸드쉐이크 패킷을 생성합니다.

### `buildPowerOn(leftCur, rightCur): Uint8Array`
전원 켜기 패킷을 생성합니다.

### `buildPowerOff(leftCur, rightCur): Uint8Array`
전원 끄기 패킷을 생성합니다.

### `parseStatus(packet: Uint8Array): Status`
20바이트 상태 패킷을 파싱합니다.

### 상수

| 상수 | 값 | 설명 |
|------|---|------|
| `SIDE_LEFT` | `0x01` | 왼쪽만 |
| `SIDE_RIGHT` | `0x02` | 오른쪽만 |
| `SIDE_BOTH` | `0x03` | 양쪽 |

## 프로토콜

전체 BLE 프로토콜 사양: [PROTOCOL.md](https://github.com/minjun0219/hotwatermat-ble/blob/main/PROTOCOL.md)

## 라이선스

MIT
