// MCP server for controlling a BLE hot water mat via JSON-RPC over stdio.
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

// JSON-RPC types
type jsonRPCRequest struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      any             `json:"id,omitempty"`
	Method  string          `json:"method"`
	Params  json.RawMessage `json:"params,omitempty"`
}

type jsonRPCResponse struct {
	JSONRPC string `json:"jsonrpc"`
	ID      any    `json:"id,omitempty"`
	Result  any    `json:"result,omitempty"`
	Error   *rpcError `json:"error,omitempty"`
}

type rpcError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

// MCP protocol types
type serverInfo struct {
	Name    string `json:"name"`
	Version string `json:"version"`
}

type initializeResult struct {
	ProtocolVersion string     `json:"protocolVersion"`
	Capabilities    any        `json:"capabilities"`
	ServerInfo      serverInfo `json:"serverInfo"`
}

type tool struct {
	Name        string `json:"name"`
	Description string `json:"description"`
	InputSchema any    `json:"inputSchema"`
}

type toolsResult struct {
	Tools []tool `json:"tools"`
}

type contentItem struct {
	Type string `json:"type"`
	Text string `json:"text"`
}

type callToolResult struct {
	Content []contentItem `json:"content"`
	IsError bool          `json:"isError,omitempty"`
}

type callToolParams struct {
	Name      string          `json:"name"`
	Arguments json.RawMessage `json:"arguments"`
}

func main() {
	scanner := bufio.NewScanner(os.Stdin)
	scanner.Buffer(make([]byte, 1024*1024), 1024*1024)

	for scanner.Scan() {
		line := scanner.Text()
		if line == "" {
			continue
		}

		var req jsonRPCRequest
		if err := json.Unmarshal([]byte(line), &req); err != nil {
			writeError(nil, -32700, "Parse error")
			continue
		}

		handleRequest(&req)
	}
}

func handleRequest(req *jsonRPCRequest) {
	switch req.Method {
	case "initialize":
		writeResult(req.ID, initializeResult{
			ProtocolVersion: "2024-11-05",
			Capabilities: map[string]any{
				"tools": map[string]any{},
			},
			ServerInfo: serverInfo{
				Name:    "hotwatermat-ble",
				Version: "0.1.0",
			},
		})

	case "notifications/initialized":
		// No response needed

	case "tools/list":
		writeResult(req.ID, toolsResult{
			Tools: getTools(),
		})

	case "tools/call":
		var params callToolParams
		if err := json.Unmarshal(req.Params, &params); err != nil {
			writeError(req.ID, -32602, "Invalid params")
			return
		}
		result := handleToolCall(params)
		writeResult(req.ID, result)

	default:
		writeError(req.ID, -32601, fmt.Sprintf("Method not found: %s", req.Method))
	}
}

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
				"required": []string{"left", "right"},
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
			Description: "Turn off the hot water mat (requires physical button to restart)",
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

func handleToolCall(params callToolParams) callToolResult {
	var args map[string]any
	if len(params.Arguments) > 0 {
		json.Unmarshal(params.Arguments, &args)
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

func handleScan() callToolResult {
	results, err := ble.Scan(5*time.Second, "KDO_HotWaterMat")
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

	if st.PoweredOff {
		return textResult("Power: OFF")
	}

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

func handleSetTemp(args map[string]any) callToolResult {
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

	err = client.SetTemp(protocol.SideBoth, left, right)
	if err != nil {
		return errorResult(fmt.Sprintf("Failed to set temperature: %v", err))
	}
	return textResult(fmt.Sprintf("Temperature set: left=%.1f°C right=%.1f°C", left, right))
}

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

func handlePowerOff(args map[string]any) callToolResult {
	client, err := connectFromArgs(args)
	if err != nil {
		return errorResult(err.Error())
	}
	defer client.Disconnect()

	if err := client.PowerOff(); err != nil {
		return errorResult(fmt.Sprintf("Failed to power off: %v", err))
	}
	return textResult("Power OFF sent. Physical button required to restart.")
}

func connectFromArgs(args map[string]any) (*ble.Client, error) {
	address := getStringArg(args, "address")
	if address == "" {
		address = os.Getenv("HOTWATERMAT_ADDRESS")
	}
	if address == "" {
		return nil, fmt.Errorf("no device address specified. Set HOTWATERMAT_ADDRESS or use --address flag")
	}

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
		return nil, fmt.Errorf("no device GID specified. Set HOTWATERMAT_DEVICE_GID env var")
	}

	client := ble.NewClient(address, gid, false)
	if err := client.Connect(); err != nil {
		return nil, err
	}
	return client, nil
}

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

func textResult(text string) callToolResult {
	return callToolResult{
		Content: []contentItem{{Type: "text", Text: text}},
	}
}

func errorResult(msg string) callToolResult {
	return callToolResult{
		Content: []contentItem{{Type: "text", Text: msg}},
		IsError: true,
	}
}

func writeResult(id any, result any) {
	resp := jsonRPCResponse{
		JSONRPC: "2.0",
		ID:      id,
		Result:  result,
	}
	data, _ := json.Marshal(resp)
	fmt.Println(string(data))
}

func writeError(id any, code int, message string) {
	resp := jsonRPCResponse{
		JSONRPC: "2.0",
		ID:      id,
		Error:   &rpcError{Code: code, Message: message},
	}
	data, _ := json.Marshal(resp)
	fmt.Println(string(data))
}
