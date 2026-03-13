# Copilot Instructions — hotwatermat-ble

## 응답 언어

- 모든 코드 리뷰 코멘트와 PR 리뷰는 **한국어(Korean)** 로 작성하세요.
- 코드 예시나 변수명 등 코드 자체는 영어를 유지하되, 설명과 제안은 한국어로 작성합니다.

## 프로젝트 개요

BLE 온수매트 컨트롤러 프로젝트입니다. CLI, MCP 서버, npm/WASM 패키지를 Go 프로토콜 라이브러리 기반으로 제공합니다.

## 코드 리뷰 시 중점 사항

- **프로토콜 정확성**: 패킷은 항상 20바이트, 체크섬은 `sum(bytes[1:19]) & 0xFF`
- **온도 인코딩**: `encoded = temp + 127.5` (0.5°C 정밀도). Bit7은 난방 플래그가 아닙니다.
- **BLE 특성**: CHAR1(`0100dd`)은 STATUS 수신 전용, CHAR2(`0200dd`)는 모든 쓰기 작업용
- **Write 모드**: 항상 write-with-response 사용 (WriteWithoutResponse 사용 금지)
- **보안**: 하드코딩된 시크릿 없음, 적절한 입력 검증

## 상표 정책

코드, 주석, 문서, 커밋 메시지에 "Navien" 또는 "나비엔" 브랜드명을 사용하지 마세요.
"hot water mat", "온수매트", "KDO_HotWaterMat" 등 일반 용어를 사용합니다.

## 코딩 컨벤션

- Go 코드는 표준 Go 스타일(`gofmt`, `go vet`)을 따릅니다
- 코드 주석은 한국어로 작성합니다
- 커밋 메시지는 영어 접두사(`feat:`, `fix:`, `chore:`, `docs:`) + 한국어 또는 영어 본문
