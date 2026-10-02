# hotwatermat-ble

BLE 온수매트(KDO_HotWaterMat / EQM555)를 커맨드라인으로 제어하는 오픈소스 도구입니다.

WiFi나 클라우드 없이 BLE만으로 직접 제어합니다. 전용 앱 없이 터미널에서 온도 설정, 전원 제어, 상태 확인이 가능합니다.

온수매트 앱의 불편함을 해소하고자 AI를 활용해 블루투스 통신을 리버스 엔지니어링하여, 바이브 코딩으로 CLI를 개발해 AI 에이전트로 제어할 수 있게 만든 프로젝트 입니다. 

## 아키텍처

```
┌─────────────────────────────────────────────┐
│  CLI (cobra)                                │
│  hotwatermat-ble scan/status/temp/on/off    │
├─────────────────────────────────────────────┤
│  pkg/ble/            BLE 클라이언트          │
│  (tinygo bluetooth, CoreBluetooth on macOS) │
├─────────────────────────────────────────────┤
│  pkg/protocol/       패킷 생성/파싱          │
│  온도 인코딩, 체크섬, 핸드쉐이크, STATUS 파서  │
└─────────────────────────────────────────────┘
```

## 설치

### 릴리즈 다운로드

[Releases](https://github.com/minjun0219/hotwatermat-ble/releases) 페이지에서 플랫폼에 맞는 tar.gz를 다운로드합니다.

```bash
# 예: macOS ARM64 (Apple Silicon)
VERSION=v0.1.0  # 원하는 버전으로 변경
curl -L "https://github.com/minjun0219/hotwatermat-ble/releases/download/${VERSION}/hotwatermat-ble_${VERSION}_darwin_arm64.tar.gz" | tar xzf - hotwatermat-ble
sudo mv hotwatermat-ble /usr/local/bin/
```

### 지원 플랫폼

| OS | 아키텍처 | 파일명 |
|----|---------|--------|
| macOS | amd64 (Intel) | `hotwatermat-ble_VERSION_darwin_amd64.tar.gz` |
| macOS | arm64 (Apple Silicon) | `hotwatermat-ble_VERSION_darwin_arm64.tar.gz` |
| Linux | amd64 | `hotwatermat-ble_VERSION_linux_amd64.tar.gz` |

### macOS 설정

릴리즈 바이너리를 macOS에서 실행하려면 추가 설정이 필요합니다.

**1. 개발자 미확인 경고 해제:**

```bash
sudo xattr -d com.apple.quarantine /usr/local/bin/hotwatermat-ble
```

**2. Bluetooth 권한:**

BLE 통신을 위해 터미널에 Bluetooth 접근 권한이 필요합니다:

- 첫 실행 시 macOS가 Bluetooth 권한 팝업을 표시합니다 → **허용**을 선택하세요.
- 팝업이 나타나지 않거나 거부한 경우: 시스템 설정 → 개인정보 보호 및 보안 → Bluetooth → 터미널 앱 허용

### 소스 빌드

Go 1.22+ 필요:

```bash
go build -o hotwatermat-ble ./cmd/hotwatermat-ble/
```

## 초기 설정 (setup)

처음 사용할 때 `setup` 명령으로 기기 페어링과 설정을 한 번에 완료할 수 있습니다.

```bash
hotwatermat-ble setup
```

**진행 과정:**

1. **[1/4] 스캔** — 주변 BLE 온수매트를 검색합니다 (5초). 여러 기기가 발견되면 선택합니다.
2. **[2/4] DeviceGid 취득** — 두 가지 방식 중 선택:
   - `[1]` 기존 GID 입력 (공식 앱에서 확인한 12자리 hex)
   - `[2]` 새 페어링 (매트가 페어링 모드일 때)
3. **[3/4] 연결 검증** — 입력한 정보로 실제 연결하여 상태를 확인합니다.
4. **[4/4] 설정 저장** — 플랫폼별 설정 디렉토리에 저장합니다.

설정 완료 후에는 `--address`와 `--device-gid` 없이 명령어를 사용할 수 있습니다.

```bash
# 저장된 설정 초기화
hotwatermat-ble setup --reset
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
hotwatermat-ble temp --left 33.5        # 왼쪽만
hotwatermat-ble temp --right 40         # 오른쪽만

# 전원 켜기/끄기
hotwatermat-ble on
hotwatermat-ble off
```

### 설정 우선순위

기기 주소와 인증 키는 아래 우선순위로 결정됩니다:

1. CLI 플래그 (`--address`, `--device-gid`)
2. 환경변수 (`HOTWATERMAT_ADDRESS`, `HOTWATERMAT_DEVICE_GID`)
3. 설정 파일 (`setup` 명령으로 저장된 값)
4. 자동 스캔 (주소만 — 기기 1개 발견 시 자동 선택)

```bash
# 플래그로 직접 지정
hotwatermat-ble --address <ADDRESS> --device-gid <GID> status

# 환경변수로 설정
export HOTWATERMAT_ADDRESS=FD319CFA-2E62-116D-D348-5B9FEEE95D2F
export HOTWATERMAT_DEVICE_GID=13CE3CC53E5A
hotwatermat-ble status

# 디버그 출력
hotwatermat-ble --debug status
```

## 코딩 에이전트에게

이 저장소에는 [에이전트 스킬](skills/hotwatermat-ble/SKILL.md)이 들어 있습니다. 코딩 에이전트에게 CLI 설치와 초기 설정 확인,
명령과 상태 출력 읽는 법, 지켜야 할 규칙(요청받은 온도만 설정, 명령은 한 번에 하나씩, DeviceGid 노출 금지)과
문제 해결 방법을 알려 줍니다. Agent Skills 형식을 따르므로 `SKILL.md` 를 읽는 에이전트는 그대로 쓸 수 있습니다.
Claude Code 에서는 플러그인으로 설치합니다.

```sh
/plugin marketplace add minjun0219/hotwatermat-ble
/plugin install hotwatermat-ble@hotwatermat-ble
```

`setup` 은 대화형이라 에이전트가 대신 실행하지 않습니다. 처음 한 번은 직접 터미널에서 `hotwatermat-ble setup` 을 실행해 주세요.

[Context7](https://context7.com) 색인 설정(`context7.json`)도 들어 있습니다. Context7 에서 이 저장소를 색인하면, Context7 을 쓰는 에이전트가 문서와 함께 위 규칙을 받습니다.

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
├── pkg/protocol/          # 패킷 생성/파싱 (순수 Go, BLE 의존성 없음)
├── pkg/ble/               # BLE 클라이언트 (tinygo bluetooth)
├── pkg/config/            # 기기 설정 저장/로드
├── cmd/hotwatermat-ble/   # CLI 바이너리
├── skills/hotwatermat-ble/ # 코딩 에이전트용 스킬 (Agent Skills 형식)
├── .claude-plugin/        # Claude Code 플러그인 마켓플레이스 설정
├── context7.json          # Context7 색인 설정
├── PROTOCOL.md            # BLE 프로토콜 사양서
└── .github/workflows/     # CI/CD (테스트 + 릴리즈)
```

## 개발

```bash
# 프로토콜 테스트
go test -v -race ./pkg/protocol/...

# CLI 빌드
go build -o hotwatermat-ble ./cmd/hotwatermat-ble/
```

## 라이선스

MIT
