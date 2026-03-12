# hotwatermat-ble

> Open-source BLE controller for KDO_HotWaterMat (EQM555) — CLI, MCP server, and npm/WASM package.

BLE 온수매트(KDO_HotWaterMat / EQM555)를 커맨드라인, MCP 서버, 또는 JavaScript/TypeScript로 제어하는 오픈소스 도구입니다.

WiFi나 클라우드 없이 BLE만으로 직접 제어합니다.

## 아키텍처

```
┌─────────────────────────────────────────────────┐
│                    애플리케이션                    │
│  ┌──────────┐  ┌───────────┐  ┌──────────────┐  │
│  │   CLI    │  │MCP 서버   │  │  npm 패키지  │  │
│  │  (cobra) │  │ (JSON-RPC)│  │  (TypeScript)│  │
│  └────┬─────┘  └─────┬─────┘  └──────┬───────┘  │
│       │              │               │           │
│  ┌────┴──────────────┴───┐    ┌──────┴───────┐  │
│  │     go/pkg/ble/       │    │  WASM 빌드   │  │
│  │   (tinygo bluetooth)  │    │  (TinyGo)    │  │
│  └────────────┬──────────┘    └──────┬───────┘  │
│               │                      │           │
│  ┌────────────┴──────────────────────┴───────┐  │
│  │          go/pkg/protocol/                  │  │
│  │    패킷 생성/파싱, 온도 인코딩, 체크섬,      │  │
│  │    핸드쉐이크, STATUS 파서                    │  │
│  └────────────────────────────────────────────┘  │
└─────────────────────────────────────────────────┘
```

## 설치

### CLI (릴리즈 다운로드)

[Releases](https://github.com/minjun0219/hotwatermat-ble/releases)에서 다운로드:

```bash
# macOS ARM
curl -L -o hotwatermat-ble \
  https://github.com/minjun0219/hotwatermat-ble/releases/latest/download/hotwatermat-ble-darwin-arm64
chmod +x hotwatermat-ble
sudo mv hotwatermat-ble /usr/local/bin/
```

### CLI (소스 빌드)

Go 1.22+ 필요:

```bash
cd go
go build -o hotwatermat-ble ./cmd/hotwatermat-ble/
```

### npm 패키지

```bash
npm install hotwatermat-ble
```

## 사용법

### CLI 명령어

```bash
# 기기 스캔
hotwatermat-ble scan

# 현재 상태 확인
hotwatermat-ble status

# 온도 설정 (28.0~48.0°C, 0.5°C 단위)
hotwatermat-ble temp --left 35 --right 35
hotwatermat-ble temp --left 33.5

# 전원 켜기
hotwatermat-ble on

# 전원 끄기
hotwatermat-ble off
```

### CLI 옵션

```bash
# BLE 주소 지정
hotwatermat-ble --address <ADDRESS> status

# 기기 인증 키 지정
hotwatermat-ble --device-gid 13CE3CC53E5A status

# 디버그 출력
hotwatermat-ble --debug status

# 환경 변수로도 설정 가능
export HOTWATERMAT_ADDRESS=FD319CFA-2E62-116D-D348-5B9FEEE95D2R
export HOTWATERMAT_DEVICE_GID=13CE3CC53E5A
```

### MCP 서버

AI 어시스턴트에서 도구 기반 제어가 가능합니다:

```bash
# 빌드 및 실행
cd go && go build -o hotwatermat-ble-mcp ./cmd/mcp-server/
```

Claude Code 설정 (`settings.json`):

```json
{
  "mcpServers": {
    "hotwatermat": {
      "command": "/path/to/hotwatermat-ble-mcp"
    }
  }
}
```

사용 가능한 MCP 도구: `scan`, `status`, `set_temp`, `power_on`, `power_off`

### npm 패키지

```typescript
import { init, encodeTemp, buildHeat, parseStatus, SIDE_BOTH } from 'hotwatermat-ble';

await init(); // WASM 로드

const target = encodeTemp(35.0);
const packet = buildHeat(SIDE_BOTH, 0x21, 0x1C, target, target);
```

## 프로토콜

BLE 프로토콜 전체 사양은 [PROTOCOL.md](PROTOCOL.md)를 참고하세요.

주요 사항:
- BLE 이름: `KDO_HotWaterMat`
- 서비스 UUID: `00001c0d-d102-11e1-9b23-2ce2a80000dd`
- 20바이트 고정 패킷 + 체크섬
- 온도 범위: 28.0~48.0°C, 0.5°C 단위
- 6바이트 DeviceGid 핸드쉐이크 기반 인증 (BLE 본딩 없음)
- BLE 연결은 1개만 가능 (연결 시 매트가 광고 중단)

## 프로젝트 구조

```
hotwatermat-ble/
├── go/
│   ├── pkg/protocol/          # 패킷 생성/파싱 (순수 Go, BLE 의존성 없음)
│   ├── pkg/ble/               # BLE 클라이언트 (tinygo bluetooth)
│   ├── cmd/hotwatermat-ble/   # CLI 바이너리
│   └── cmd/mcp-server/        # MCP 서버 (JSON-RPC stdio)
├── wasm/                      # TinyGo WASM 빌드
├── npm/                       # npm 패키지 (TypeScript 래퍼)
├── skill/                     # OpenClaw 스킬 정의
├── PROTOCOL.md                # BLE 프로토콜 사양서
└── .github/workflows/         # CI/CD (테스트 + 릴리즈)
```

## 개발

```bash
# Go 테스트 실행
cd go && go test -v ./pkg/protocol/...

# WASM 빌드
chmod +x wasm/build.sh && ./wasm/build.sh

# npm 패키지 빌드
cd npm && npm run build
```

## 라이선스

MIT
