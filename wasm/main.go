// WASM build of the protocol package for use in JavaScript/TypeScript.
// Build with TinyGo: tinygo build -o hotwatermat.wasm -target wasm ./wasm/
package main

import (
	"syscall/js"

	"github.com/minjun0219/hotwatermat-ble/pkg/protocol"
)

func main() {
	// Register exports
	js.Global().Set("hotwatermat", map[string]any{
		"encodeTemp":     js.FuncOf(encodeTemp),
		"decodeTemp":     js.FuncOf(decodeTemp),
		"calcChecksum":   js.FuncOf(calcChecksum),
		"buildHandshake": js.FuncOf(buildHandshake),
		"buildHeat":      js.FuncOf(buildHeat),
		"buildPowerOn":   js.FuncOf(buildPowerOn),
		"buildPowerOff":  js.FuncOf(buildPowerOff),
		"parseStatus":    js.FuncOf(parseStatus),
	})

	// Keep the Go runtime alive
	select {}
}

func encodeTemp(_ js.Value, args []js.Value) any {
	if len(args) < 1 {
		return js.ValueOf(map[string]any{"error": "missing temperature argument"})
	}
	temp := args[0].Float()
	b, err := protocol.EncodeTemp(temp)
	if err != nil {
		return js.ValueOf(map[string]any{"error": err.Error()})
	}
	return js.ValueOf(int(b))
}

func decodeTemp(_ js.Value, args []js.Value) any {
	if len(args) < 1 {
		return js.ValueOf(map[string]any{"error": "missing byte argument"})
	}
	b := byte(args[0].Int())
	return js.ValueOf(protocol.DecodeTemp(b))
}

func calcChecksum(_ js.Value, args []js.Value) any {
	if len(args) < 1 {
		return js.ValueOf(map[string]any{"error": "missing packet argument"})
	}
	arr := args[0]
	pkt := make([]byte, arr.Length())
	for i := 0; i < arr.Length(); i++ {
		pkt[i] = byte(arr.Index(i).Int())
	}
	return js.ValueOf(int(protocol.CalcChecksum(pkt)))
}

func buildHandshake(_ js.Value, args []js.Value) any {
	var pkt [protocol.PacketSize]byte
	if len(args) >= 1 {
		// With DeviceGid hex string
		gidHex := args[0].String()
		gid, err := protocol.ParseDeviceGidHex(gidHex)
		if err != nil {
			return js.ValueOf(map[string]any{"error": err.Error()})
		}
		pkt = protocol.BuildHandshakeWithKey(gid)
	} else {
		pkt = protocol.BuildHandshake()
	}
	return toJSArray(pkt[:])
}

func buildHeat(_ js.Value, args []js.Value) any {
	if len(args) < 5 {
		return js.ValueOf(map[string]any{"error": "need 5 args: side, leftCur, rightCur, leftTarget, rightTarget"})
	}
	side := byte(args[0].Int())
	leftCur := byte(args[1].Int())
	rightCur := byte(args[2].Int())
	leftTgt := byte(args[3].Int())
	rightTgt := byte(args[4].Int())
	pkt := protocol.BuildHeat(side, leftCur, rightCur, leftTgt, rightTgt)
	return toJSArray(pkt[:])
}

func buildPowerOn(_ js.Value, args []js.Value) any {
	var leftCur, rightCur byte
	if len(args) >= 2 {
		leftCur = byte(args[0].Int())
		rightCur = byte(args[1].Int())
	}
	pkt := protocol.BuildPowerOn(leftCur, rightCur)
	return toJSArray(pkt[:])
}

func buildPowerOff(_ js.Value, args []js.Value) any {
	var leftCur, rightCur byte
	if len(args) >= 2 {
		leftCur = byte(args[0].Int())
		rightCur = byte(args[1].Int())
	}
	pkt := protocol.BuildPowerOff(leftCur, rightCur)
	return toJSArray(pkt[:])
}

func parseStatus(_ js.Value, args []js.Value) any {
	if len(args) < 1 {
		return js.ValueOf(map[string]any{"error": "missing packet argument"})
	}
	arr := args[0]
	pkt := make([]byte, arr.Length())
	for i := 0; i < arr.Length(); i++ {
		pkt[i] = byte(arr.Index(i).Int())
	}

	st, err := protocol.ParseStatus(pkt)
	if err != nil {
		return js.ValueOf(map[string]any{"error": err.Error()})
	}

	return js.ValueOf(map[string]any{
		"mode":         int(st.Mode),
		"modeName":     st.ModeName,
		"side":         int(st.Side),
		"volume":       st.Volume,
		"waterLevel":   st.WaterLevel,
		"leftCurrent":  st.LeftCurrent,
		"rightCurrent": st.RightCurrent,
		"leftTarget":   st.LeftTarget,
		"rightTarget":  st.RightTarget,
		"leftHeating":  st.LeftHeating,
		"rightHeating": st.RightHeating,
		"poweredOff":   st.PoweredOff,
	})
}

func toJSArray(data []byte) js.Value {
	arr := js.Global().Get("Uint8Array").New(len(data))
	js.CopyBytesToJS(arr, data)
	return arr
}
