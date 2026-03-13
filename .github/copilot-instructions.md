# Copilot Instructions — hotwatermat-ble

## 응답 언어

- 모든 코드 리뷰 코멘트와 PR 리뷰는 **한국어(Korean)** 로 작성하세요.
- 코드 예시나 변수명 등 코드 자체는 영어를 유지하되, 설명과 제안은 한국어로 작성합니다.

## 프로젝트 개요

BLE 온수매트 컨트롤러 프로젝트입니다. CLI, MCP 서버, npm/WASM 패키지를 Go 프로토콜 라이브러리 기반으로 제공합니다.

## 코드 리뷰 시 중점 사항

- **프로토콜 정확성**: 패킷은 항상 20바이트, 체크섬은 `sum(bytes[1:19]) & 0xFF`
- **온도 인코딩**: 온도가 정수이면 그대로 인코딩하고, `x.5°C`인 경우에만 `encoded = temp + 127.5` 규칙을 적용합니다. Bit7은 난방 플래그가 아닙니다.
- **BLE 특성**: CHAR1(`0100dd`)은 STATUS 수신 전용, CHAR2(`0200dd`)는 모든 쓰기 작업용
- **Write 모드**: `go/pkg/ble` 내부에서 BLE 특성 쓰기를 추가·수정할 때는 `writeCharacteristic()` 래퍼를 사용하세요. 직접 `WriteWithoutResponse()`를 호출하지 마세요. 외부 패키지(예: `cmd/`)는 `ble.Client`의 공개 메서드(`SetTemp`, `PowerOn` 등)를 통해 쓰기를 수행합니다.
  - macOS(darwin): `c.Write()` — write-with-response (`go/pkg/ble/write_darwin.go`)
  - Linux: `c.WriteWithoutResponse()` — tinygo bluetooth 제약으로 인한 예외 (`go/pkg/ble/write_linux.go`)
- **보안**: 하드코딩된 시크릿 없음, 적절한 입력 검증

## 상표 정책

사용자 노출 텍스트(UI, 문서, 커밋 메시지 등)에서 특정 제조사 브랜드/광고명을 사용하지 마세요.
- **일반 용어 사용**: "hot water mat", "온수매트", "mat" 등
- **프로토콜 식별자는 예외**: `KDO_HotWaterMat`은 BLE 광고명(protocol.BLEDeviceName)으로 프로토콜 스펙에 정의된 식별자이므로 코드 내에서는 사용 가능합니다. 단, 사용자에게 직접 노출되는 안내문이나 마케팅 텍스트에는 쓰지 않습니다.

## 코딩 컨벤션

- Go 코드는 표준 Go 스타일(`gofmt`, `go vet`)을 따릅니다
- 코드 주석은 한국어로 작성합니다
- 커밋 메시지는 영어 접두사(`feat:`, `fix:`, `chore:`, `docs:`) + 한국어 또는 영어 본문
