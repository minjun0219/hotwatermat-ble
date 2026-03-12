# hotwatermat-ble

BLE 온수매트(KDO_HotWaterMat)를 Claude Code에서 제어하는 스킬입니다.

## 개요

KDO_HotWaterMat BLE 온수매트의 상태 확인, 온도 설정, 전원 제어를 CLI 또는 MCP 서버로 수행합니다.

## 사전 조건

- `hotwatermat-ble` CLI 바이너리가 설치되어 PATH에 있어야 합니다
- 호스트 머신에 BLE 어댑터가 필요합니다
- 기기 주소: `HOTWATERMAT_ADDRESS` 환경 변수로 설정
- 인증 키: `HOTWATERMAT_DEVICE_GID` 환경 변수로 설정

## 사용 가능한 명령어

### 상태 확인
```bash
hotwatermat-ble status
```
현재 모드, 좌/우 온도, 수위, 가열 상태를 표시합니다.

### 온도 설정
```bash
# 양쪽 35°C로 설정
hotwatermat-ble temp --left 35 --right 35

# 왼쪽만 설정
hotwatermat-ble temp --left 33.5

# 오른쪽만 설정
hotwatermat-ble temp --right 40
```
온도 범위: 28.0°C ~ 48.0°C (0.5°C 단위)

### 전원 켜기
```bash
hotwatermat-ble on
```
매트 전원을 켭니다. 이전 온도 설정이 자동 복원됩니다.

### 전원 끄기
```bash
hotwatermat-ble off
```
**주의:** 전원 끄면 BLE 광고가 중단됩니다. 다시 켜려면 물리 버튼을 눌러야 할 수 있습니다.

### 기기 스캔
```bash
hotwatermat-ble scan
```

## MCP 서버

Claude Code MCP 서버로 직접 도구 통합이 가능합니다:

```json
{
  "mcpServers": {
    "hotwatermat": {
      "command": "hotwatermat-ble-mcp",
      "args": []
    }
  }
}
```

### MCP 도구

| 도구 | 설명 |
|------|------|
| `scan` | BLE 온수매트 기기 스캔 |
| `status` | 현재 매트 상태 조회 |
| `set_temp` | 목표 온도 설정 (좌/우) |
| `power_on` | 매트 전원 켜기 |
| `power_off` | 매트 전원 끄기 |

## 환경 설정

| 환경 변수 | 설명 |
|-----------|------|
| `HOTWATERMAT_ADDRESS` | BLE 기기 주소 (macOS에서는 UUID 형식) |
| `HOTWATERMAT_DEVICE_GID` | 12자리 16진수 인증 키 |

## 팁

- 매트는 BLE 연결 1개만 지원합니다
- 공식 앱이 연결 중이면 먼저 강제 종료하세요
- 연결 중 상태 업데이트가 ~1초마다 스트리밍됩니다
- 연결은 짧게 유지합니다 — 연결 → 명령 전송 → 해제
