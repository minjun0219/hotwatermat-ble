# hotwatermat-ble

BLE 온수매트(KDO_HotWaterMat / EQM555) CLI 및 MCP 서버.

이 npm 패키지는 [GitHub Releases](https://github.com/minjun0219/hotwatermat-ble/releases)에서 플랫폼에 맞는 네이티브 바이너리를 자동으로 다운로드합니다.

## 설치

```bash
npm install -g hotwatermat-ble
```

`postinstall` 스크립트가 자동으로 플랫폼에 맞는 바이너리를 다운로드합니다.

### 바이너리 다운로드 건너뛰기

```bash
HOTWATERMAT_SKIP_BINARY=1 npm install -g hotwatermat-ble
```

## 사용법

### CLI

```bash
hotwatermat-ble --help
hotwatermat-ble scan
hotwatermat-ble status
hotwatermat-ble temp --left 36 --right 36
hotwatermat-ble on
hotwatermat-ble off
```

### MCP 서버

```bash
hotwatermat-ble-mcp
```

#### Claude Desktop 설정

```json
{
  "mcpServers": {
    "hotwatermat-ble": {
      "command": "npx",
      "args": ["-y", "--package", "hotwatermat-ble", "hotwatermat-ble-mcp"],
      "env": {
        "HOTWATERMAT_DEVICE_GID": "your-device-gid"
      }
    }
  }
}
```

## 지원 플랫폼

| OS    | 아키텍처 |
|-------|---------|
| macOS | x64, arm64 |
| Linux | x64 |

> Windows는 현재 미지원입니다. 소스에서 직접 빌드하세요.

## 소스에서 빌드

지원되지 않는 플랫폼이거나 직접 빌드하려면:

```bash
git clone https://github.com/minjun0219/hotwatermat-ble.git
cd hotwatermat-ble/go
go build -o hotwatermat-ble ./cmd/hotwatermat-ble
go build -o hotwatermat-ble-mcp ./cmd/mcp-server
```

## 라이선스

MIT
