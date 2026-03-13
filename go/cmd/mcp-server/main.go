// 온수매트 BLE MCP(Model Context Protocol) 서버입니다.
//
// JSON-RPC 2.0 프로토콜을 사용하여 표준 입력/출력(stdio)을 통해 통신합니다.
// AI 어시스턴트(Claude 등)가 온수매트를 제어할 수 있도록 MCP 도구를 제공합니다.
//
// 제공하는 MCP 도구:
//   - scan: BLE 기기 검색
//   - status: 현재 상태 조회 (온도, 모드, 수위)
//   - set_temp: 목표 온도 설정 (28.0~48.0°C, 0.5°C 단위)
//   - power_on: 전원 켜기
//   - power_off: 전원 끄기
//
// 실행 방법:
//
//	# 환경변수 설정 후 실행
//	export HOTWATERMAT_ADDRESS=AA:BB:CC:DD:EE:FF
//	export HOTWATERMAT_DEVICE_GID=13CE3CC53E5A
//	hotwatermat-ble-mcp
//
// MCP 프로토콜 흐름:
//  1. 클라이언트 → "initialize" 요청
//  2. 서버 → 프로토콜 버전, 기능(capabilities), 서버 정보 응답
//  3. 클라이언트 → "notifications/initialized" 알림
//  4. 클라이언트 → "tools/list" 요청
//  5. 서버 → 사용 가능한 도구 목록 응답
//  6. 클라이언트 → "tools/call" 요청 (도구 실행)
//  7. 서버 → 실행 결과 응답
package main

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"time"

	"github.com/minjun0219/hotwatermat-ble/pkg/ble"
	"github.com/minjun0219/hotwatermat-ble/pkg/protocol"
)

// --- JSON-RPC 2.0 타입 정의 ---

// jsonRPCRequest는 JSON-RPC 2.0 요청 메시지입니다.
type jsonRPCRequest struct {
	JSONRPC string          `json:"jsonrpc"` // 항상 "2.0"
	ID      any             `json:"id,omitempty"` // 요청 ID (알림이면 생략)
	Method  string          `json:"method"` // 호출할 메서드 이름
	Params  json.RawMessage `json:"params,omitempty"` // 메서드 매개변수 (JSON)
}

// jsonRPCResponse는 JSON-RPC 2.0 응답 메시지입니다.
type jsonRPCResponse struct {
	JSONRPC string    `json:"jsonrpc"` // 항상 "2.0"
	ID      any       `json:"id,omitempty"` // 요청의 ID와 동일
	Result  any       `json:"result,omitempty"` // 성공 시 결과
	Error   *rpcError `json:"error,omitempty"` // 실패 시 오류 정보
}

// rpcError는 JSON-RPC 2.0 오류 객체입니다.
type rpcError struct {
	Code    int    `json:"code"` // 오류 코드 (예: -32700=파싱오류, -32601=메서드없음)
	Message string `json:"message"` // 오류 메시지
}

// --- MCP 프로토콜 타입 정의 ---

// serverInfo는 MCP 서버의 이름과 버전 정보입니다.
type serverInfo struct {
	Name    string `json:"name"`
	Version string `json:"version"`
}

// initializeResult는 "initialize" 요청에 대한 응답 데이터입니다.
// MCP 프로토콜 버전, 서버 기능, 서버 정보를 포함합니다.
type initializeResult struct {
	ProtocolVersion string     `json:"protocolVersion"` // MCP 프로토콜 버전
	Capabilities    any        `json:"capabilities"` // 서버 기능 (도구, 리소스 등)
	ServerInfo      serverInfo `json:"serverInfo"` // 서버 이름/버전
}

// tool은 MCP 도구 정의입니다.
// AI 어시스턴트에게 어떤 도구를 사용할 수 있는지 알려줍니다.
type tool struct {
	Name        string `json:"name"` // 도구 이름 (예: "scan", "status")
	Description string `json:"description"` // 도구 설명
	InputSchema any    `json:"inputSchema"` // 입력 매개변수 스키마 (JSON Schema 형식)
}

// toolsResult는 "tools/list" 요청에 대한 응답입니다.
type toolsResult struct {
	Tools []tool `json:"tools"`
}

// contentItem은 도구 실행 결과의 콘텐츠 항목입니다.
type contentItem struct {
	Type string `json:"type"` // 콘텐츠 타입 (현재 "text"만 사용)
	Text string `json:"text"` // 텍스트 내용
}

// callToolResult는 "tools/call" 요청에 대한 응답입니다.
type callToolResult struct {
	Content []contentItem `json:"content"` // 결과 콘텐츠 목록
	IsError bool          `json:"isError,omitempty"` // true이면 오류 결과
}

// callToolParams는 "tools/call" 요청의 매개변수입니다.
type callToolParams struct {
	Name      string          `json:"name"` // 실행할 도구 이름
	Arguments json.RawMessage `json:"arguments"` // 도구에 전달할 인자 (JSON)
}

// main은 MCP 서버의 진입점입니다.
//
// 표준 입력에서 JSON-RPC 메시지를 한 줄씩 읽어 처리합니다.
// 각 줄은 하나의 완전한 JSON-RPC 요청이어야 합니다.
// 빈 줄은 무시합니다.
func main() {
	scanner := bufio.NewScanner(os.Stdin)
	// 최대 1MB 크기의 입력 버퍼 설정
	scanner.Buffer(make([]byte, 1024*1024), 1024*1024)

	// 표준 입력에서 한 줄씩 읽어 JSON-RPC 요청 처리
	for scanner.Scan() {
		line := scanner.Text()
		if line == "" {
			continue // 빈 줄 무시
		}

		// JSON 파싱
		var req jsonRPCRequest
		if err := json.Unmarshal([]byte(line), &req); err != nil {
			writeError(nil, -32700, "Parse error")
			continue
		}

		// 요청 처리
		handleRequest(&req)
	}
}

// handleRequest는 MCP 프로토콜의 메서드를 라우팅합니다.
//
// 지원하는 메서드:
//   - "initialize": 서버 초기화 (프로토콜 버전, 기능 응답)
//   - "notifications/initialized": 초기화 완료 알림 (응답 불필요)
//   - "tools/list": 사용 가능한 도구 목록 반환
//   - "tools/call": 지정된 도구 실행
func handleRequest(req *jsonRPCRequest) {
	switch req.Method {
	case "initialize":
		// MCP 초기화: 프로토콜 버전과 서버 기능 반환
		writeResult(req.ID, initializeResult{
			ProtocolVersion: "2024-11-05",
			Capabilities: map[string]any{
				"tools": map[string]any{}, // 도구 기능 지원
			},
			ServerInfo: serverInfo{
				Name:    "hotwatermat-ble",
				Version: "0.1.0",
			},
		})

	case "notifications/initialized":
		// 클라이언트가 초기화 완료를 알림 — 응답 불필요

	case "tools/list":
		// 사용 가능한 도구 목록 반환
		writeResult(req.ID, toolsResult{
			Tools: getTools(),
		})

	case "tools/call":
		// 도구 실행 요청 처리
		var params callToolParams
		if err := json.Unmarshal(req.Params, &params); err != nil {
			writeError(req.ID, -32602, "Invalid params")
			return
		}
		result := handleToolCall(params)
		writeResult(req.ID, result)

	default:
		// 알 수 없는 메서드
		writeError(req.ID, -32601, fmt.Sprintf("Method not found: %s", req.Method))
	}
}

// getTools는 MCP 서버가 제공하는 도구 목록을 반환합니다.
//
// 각 도구에는 이름, 설명, 입력 스키마(JSON Schema)가 포함됩니다.
// AI 어시스턴트는 이 정보를 바탕으로 적절한 도구를 선택하고 호출합니다.
func getTools() []tool {
	return []tool{
		{
			Name:        "scan",
			Description: "Scan for BLE hot water mat devices",
			InputSchema: map[string]any{
				"type":       "object",
				"properties": map[string]any{},
			},
		},
		{
			Name:        "status",
			Description: "Get current mat status (temperature, mode, water level)",
			InputSchema: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"address": map[string]any{
						"type":        "string",
						"description": "BLE device address",
					},
					"device_gid": map[string]any{
						"type":        "string",
						"description": "Device authentication key (hex)",
					},
				},
			},
		},
		{
			Name:        "set_temp",
			Description: "Set target temperature (28.0-48.0°C, 0.5°C steps)",
			InputSchema: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"left": map[string]any{
						"type":        "number",
						"description": "Left side target temperature",
					},
					"right": map[string]any{
						"type":        "number",
						"description": "Right side target temperature",
					},
					"address": map[string]any{
						"type":        "string",
						"description": "BLE device address",
					},
					"device_gid": map[string]any{
						"type":        "string",
						"description": "Device authentication key (hex)",
					},
				},
				"required": []string{"left", "right"}, // 양쪽 온도 모두 필수
			},
		},
		{
			Name:        "power_on",
			Description: "Turn on the hot water mat",
			InputSchema: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"address": map[string]any{
						"type":        "string",
						"description": "BLE device address",
					},
					"device_gid": map[string]any{
						"type":        "string",
						"description": "Device authentication key (hex)",
					},
				},
			},
		},
		{
			Name:        "power_off",
			Description: "Turn off the hot water mat",
			InputSchema: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"address": map[string]any{
						"type":        "string",
						"description": "BLE device address",
					},
					"device_gid": map[string]any{
						"type":        "string",
						"description": "Device authentication key (hex)",
					},
				},
			},
		},
	}
}

// handleToolCall은 도구 이름에 따라 적절한 핸들러를 호출합니다.
func handleToolCall(params callToolParams) callToolResult {
	// 인자를 map으로 파싱 (인자가 없을 수도 있음)
	var args map[string]any
	if len(params.Arguments) > 0 {
		if err := json.Unmarshal(params.Arguments, &args); err != nil {
			return errorResult(fmt.Sprintf("Invalid arguments JSON: %v", err))
		}
	}

	switch params.Name {
	case "scan":
		return handleScan()
	case "status":
		return handleStatus(args)
	case "set_temp":
		return handleSetTemp(args)
	case "power_on":
		return handlePowerOn(args)
	case "power_off":
		return handlePowerOff(args)
	default:
		return errorResult(fmt.Sprintf("Unknown tool: %s", params.Name))
	}
}

// handleScan은 BLE 기기 검색을 수행합니다.
//
// 5초 동안 "KDO_HotWaterMat" 기기를 검색하고 결과를 텍스트로 반환합니다.
// 인자가 필요 없으며, 연결이나 인증도 수행하지 않습니다.
func handleScan() callToolResult {
	results, err := ble.Scan(5*time.Second, protocol.BLEDeviceName)
	if err != nil {
		return errorResult(fmt.Sprintf("Scan failed: %v", err))
	}
	if len(results) == 0 {
		return textResult("No devices found.")
	}
	var text string
	for _, r := range results {
		text += fmt.Sprintf("Address: %s  Name: %s  RSSI: %d\n", r.Address, r.Name, r.RSSI)
	}
	return textResult(text)
}

// handleStatus는 매트 상태를 조회합니다.
//
// 필요한 인자: address(선택), device_gid(선택) — 환경변수로 대체 가능
// 반환: 모드, 좌/우 선택, 수위, 좌/우 온도 (현재→목표)
func handleStatus(args map[string]any) callToolResult {
	client, err := connectFromArgs(args)
	if err != nil {
		return errorResult(err.Error())
	}
	defer client.Disconnect()

	st, err := client.GetStatus(5 * time.Second)
	if err != nil {
		return errorResult(fmt.Sprintf("Failed to get status: %v", err))
	}

	// 전원 꺼짐 상태
	if st.PoweredOff {
		return textResult("Power: OFF")
	}

	// 현재 상태를 텍스트로 포맷팅
	text := fmt.Sprintf(
		"Mode: %s\nSide: %s\nWater: %s\nLeft: %.1f°C → %.1f°C\nRight: %.1f°C → %.1f°C",
		st.ModeName,
		protocol.SideName(st.Side),
		protocol.WaterLevelName(st.WaterLevel),
		st.LeftCurrent, st.LeftTarget,
		st.RightCurrent, st.RightTarget,
	)
	return textResult(text)
}

// handleSetTemp는 목표 온도를 설정합니다.
//
// 필수 인자: left(왼쪽 온도), right(오른쪽 온도)
// 선택 인자: address, device_gid
// 양쪽 온도 모두 28.0~48.0°C 범위여야 하며, 0.5°C 단위여야 합니다.
func handleSetTemp(args map[string]any) callToolResult {
	// 필수 인자 확인 (left, right 모두 필요)
	left, ok1 := args["left"].(float64)
	right, ok2 := args["right"].(float64)
	if !ok1 || !ok2 {
		return errorResult("Both 'left' and 'right' temperatures are required")
	}

	client, err := connectFromArgs(args)
	if err != nil {
		return errorResult(err.Error())
	}
	defer client.Disconnect()

	// 양쪽(both) 온도 설정 명령 전송
	err = client.SetTemp(protocol.SideBoth, left, right)
	if err != nil {
		return errorResult(fmt.Sprintf("Failed to set temperature: %v", err))
	}
	return textResult(fmt.Sprintf("Temperature set: left=%.1f°C right=%.1f°C", left, right))
}

// handlePowerOn은 매트 전원을 켭니다.
//
// 선택 인자: address, device_gid
func handlePowerOn(args map[string]any) callToolResult {
	client, err := connectFromArgs(args)
	if err != nil {
		return errorResult(err.Error())
	}
	defer client.Disconnect()

	if err := client.PowerOn(); err != nil {
		return errorResult(fmt.Sprintf("Failed to power on: %v", err))
	}
	return textResult("Power ON sent.")
}

// handlePowerOff는 매트 전원을 끕니다.
//
// 전원을 끈 후에도 BLE를 통해 다시 켤 수 있습니다.
// 선택 인자: address, device_gid
func handlePowerOff(args map[string]any) callToolResult {
	client, err := connectFromArgs(args)
	if err != nil {
		return errorResult(err.Error())
	}
	defer client.Disconnect()

	if err := client.PowerOff(); err != nil {
		return errorResult(fmt.Sprintf("Failed to power off: %v", err))
	}
	return textResult("Power OFF sent.")
}

// connectFromArgs는 도구 인자와 환경변수를 사용하여 BLE 기기에 연결합니다.
//
// 기기 주소 결정 우선순위:
//  1. 도구 인자의 "address" 필드
//  2. HOTWATERMAT_ADDRESS 환경변수
//  3. 둘 다 없으면 오류 반환
//
// DeviceGid 결정 우선순위:
//  1. 도구 인자의 "device_gid" 필드
//  2. HOTWATERMAT_DEVICE_GID 환경변수
//  3. 둘 다 없으면 오류 반환
func connectFromArgs(args map[string]any) (*ble.Client, error) {
	// 기기 주소 결정
	address := getStringArg(args, "address")
	if address == "" {
		address = os.Getenv("HOTWATERMAT_ADDRESS")
	}
	if address == "" {
		return nil, fmt.Errorf("no device address specified. Provide address argument or set HOTWATERMAT_ADDRESS env var")
	}

	// DeviceGid 결정
	var gid [6]byte
	if gidHex := getStringArg(args, "device_gid"); gidHex != "" {
		var err error
		gid, err = protocol.ParseDeviceGidHex(gidHex)
		if err != nil {
			return nil, fmt.Errorf("invalid device_gid: %w", err)
		}
	} else if envGid := os.Getenv("HOTWATERMAT_DEVICE_GID"); envGid != "" {
		var err error
		gid, err = protocol.ParseDeviceGidHex(envGid)
		if err != nil {
			return nil, fmt.Errorf("invalid HOTWATERMAT_DEVICE_GID: %w", err)
		}
	} else {
		return nil, fmt.Errorf("no device GID specified. Provide device_gid argument or set HOTWATERMAT_DEVICE_GID env var")
	}

	// BLE 연결 수립 (디버그 모드 OFF)
	client := ble.NewClient(address, gid, false)
	if err := client.Connect(); err != nil {
		return nil, err
	}
	return client, nil
}

// getStringArg는 도구 인자 맵에서 문자열 값을 안전하게 추출합니다.
// 키가 없거나 문자열이 아닌 경우 빈 문자열을 반환합니다.
func getStringArg(args map[string]any, key string) string {
	if args == nil {
		return ""
	}
	v, ok := args[key].(string)
	if !ok {
		return ""
	}
	return v
}

// textResult는 성공적인 텍스트 결과를 생성하는 헬퍼 함수입니다.
func textResult(text string) callToolResult {
	return callToolResult{
		Content: []contentItem{{Type: "text", Text: text}},
	}
}

// errorResult는 오류 결과를 생성하는 헬퍼 함수입니다.
// IsError가 true로 설정되어 AI 어시스턴트가 오류를 인식할 수 있습니다.
func errorResult(msg string) callToolResult {
	return callToolResult{
		Content: []contentItem{{Type: "text", Text: msg}},
		IsError: true,
	}
}

// writeResult는 JSON-RPC 성공 응답을 표준 출력에 씁니다.
func writeResult(id any, result any) {
	resp := jsonRPCResponse{
		JSONRPC: "2.0",
		ID:      id,
		Result:  result,
	}
	data, _ := json.Marshal(resp)
	fmt.Println(string(data))
}

// writeError는 JSON-RPC 오류 응답을 표준 출력에 씁니다.
func writeError(id any, code int, message string) {
	resp := jsonRPCResponse{
		JSONRPC: "2.0",
		ID:      id,
		Error:   &rpcError{Code: code, Message: message},
	}
	data, _ := json.Marshal(resp)
	fmt.Println(string(data))
}
