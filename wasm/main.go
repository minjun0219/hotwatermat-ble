// 온수매트 프로토콜 패키지의 WASM(WebAssembly) 빌드입니다.
//
// JavaScript/TypeScript 환경에서 온수매트 BLE 프로토콜을 사용할 수 있도록
// Go 프로토콜 패키지를 WASM으로 컴파일합니다.
//
// 빌드 방법 (TinyGo 사용):
//
//	tinygo build -o hotwatermat.wasm -target wasm ./wasm/
//
// 사용 방법 (JavaScript):
//
//	// wasm_exec.js 로드 후
//	const go = new Go();
//	const result = await WebAssembly.instantiate(wasmBuffer, go.importObject);
//	go.run(result.instance);
//
//	// 전역 hotwatermat 객체를 통해 함수 호출
//	const encoded = hotwatermat.encodeTemp(33.5);  // → 161
//	const decoded = hotwatermat.decodeTemp(161);     // → 33.5
//	const handshake = hotwatermat.buildHandshake("13CE3CC53E5A");
//
// WASM으로 노출되는 함수:
//   - encodeTemp(temp) → 인코딩된 바이트 값
//   - decodeTemp(byte) → 디코딩된 온도 값
//   - calcChecksum(packet) → 체크섬 값
//   - buildHandshake(gidHex?) → 핸드셰이크 패킷 (Uint8Array)
//   - buildHeat(side, leftCur, rightCur, leftTgt, rightTgt) → 난방 명령 패킷
//   - buildPowerOn(leftCur?, rightCur?) → 전원 ON 패킷
//   - buildPowerOff(leftCur?, rightCur?) → 전원 OFF 패킷
//   - parseStatus(packet) → 상태 객체
package main

import (
	"syscall/js"

	"github.com/minjun0219/hotwatermat-ble/pkg/protocol"
)

// jsFuncs는 등록된 js.Func 참조를 보관합니다.
// Go의 가비지 컬렉터가 JavaScript에 등록된 함수를 해제하지 않도록 참조를 유지합니다.
var jsFuncs []js.Func

// registerFunc는 Go 함수를 JavaScript의 전역 hotwatermat 객체에 등록합니다.
//
// 매개변수:
//   - name: JavaScript에서 호출할 함수 이름 (예: "encodeTemp")
//   - fn: Go 함수 (js.Value 인자를 받아 any를 반환)
//
// 등록 후 JavaScript에서 hotwatermat.encodeTemp(33.5) 형태로 호출할 수 있습니다.
func registerFunc(name string, fn func(js.Value, []js.Value) any) {
	f := js.FuncOf(fn)
	jsFuncs = append(jsFuncs, f) // GC 방지를 위해 참조 유지
	js.Global().Get("hotwatermat").Set(name, f)
}

// main은 WASM 모듈의 진입점입니다.
//
// 전역 hotwatermat 객체를 생성하고 모든 프로토콜 함수를 등록합니다.
// select {}로 Go 런타임을 영구히 살려두어 JavaScript에서 함수를 계속 호출할 수 있게 합니다.
func main() {
	// JavaScript 전역에 hotwatermat 객체 생성
	js.Global().Set("hotwatermat", map[string]any{})

	// 프로토콜 함수들을 JavaScript에 등록
	registerFunc("encodeTemp", encodeTemp)
	registerFunc("decodeTemp", decodeTemp)
	registerFunc("calcChecksum", calcChecksum)
	registerFunc("buildHandshake", buildHandshake)
	registerFunc("buildHeat", buildHeat)
	registerFunc("buildPowerOn", buildPowerOn)
	registerFunc("buildPowerOff", buildPowerOff)
	registerFunc("parseStatus", parseStatus)

	// Go 런타임을 영구히 유지 (WASM 모듈이 종료되지 않도록)
	// 이 select는 영원히 블록되며, JavaScript에서 함수를 계속 호출할 수 있게 합니다.
	select {}
}

// encodeTemp는 온도(float64)를 BLE 바이트로 인코딩합니다.
//
// JavaScript 호출: hotwatermat.encodeTemp(33.5)
// 성공 시 인코딩된 바이트 값(숫자)을 반환합니다.
// 실패 시 {error: "에러 메시지"} 객체를 반환합니다.
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

// decodeTemp는 BLE 바이트를 온도(float64)로 디코딩합니다.
//
// JavaScript 호출: hotwatermat.decodeTemp(161) → 33.5
func decodeTemp(_ js.Value, args []js.Value) any {
	if len(args) < 1 {
		return js.ValueOf(map[string]any{"error": "missing byte argument"})
	}
	b := byte(args[0].Int())
	return js.ValueOf(protocol.DecodeTemp(b))
}

// calcChecksum은 20바이트 패킷의 체크섬을 계산합니다.
//
// JavaScript 호출: hotwatermat.calcChecksum(new Uint8Array([...]))
// 입력: Uint8Array (20바이트 패킷)
// 반환: 체크섬 바이트 값 (숫자)
func calcChecksum(_ js.Value, args []js.Value) any {
	if len(args) < 1 {
		return js.ValueOf(map[string]any{"error": "missing packet argument"})
	}
	arr := args[0]
	// JavaScript Uint8Array를 Go []byte로 복사
	pkt := make([]byte, arr.Length())
	js.CopyBytesToGo(pkt, arr)
	return js.ValueOf(int(protocol.CalcChecksum(pkt)))
}

// buildHandshake는 핸드셰이크 패킷을 생성합니다.
//
// JavaScript 호출:
//   - hotwatermat.buildHandshake()                    → 최초 페어링 핸드셰이크
//   - hotwatermat.buildHandshake("13CE3CC53E5A")      → DeviceGid 포함 인증 핸드셰이크
//
// 반환: Uint8Array (20바이트 패킷)
func buildHandshake(_ js.Value, args []js.Value) any {
	var pkt [protocol.PacketSize]byte
	if len(args) >= 1 {
		// DeviceGid가 제공된 경우: 인증 핸드셰이크 생성
		gidHex := args[0].String()
		gid, err := protocol.ParseDeviceGidHex(gidHex)
		if err != nil {
			return js.ValueOf(map[string]any{"error": err.Error()})
		}
		pkt = protocol.BuildHandshakeWithKey(gid)
	} else {
		// DeviceGid 없음: 최초 페어링 핸드셰이크 생성
		pkt = protocol.BuildHandshake()
	}
	return toJSArray(pkt[:])
}

// buildHeat는 난방(HEAT) 명령 패킷을 생성합니다.
//
// JavaScript 호출:
//
//	hotwatermat.buildHeat(side, leftCur, rightCur, leftTarget, rightTarget)
//
// 매개변수 (5개 필수):
//   - side: 좌/우 선택 (0x02=왼쪽, 0x04=오른쪽, 0x06=양쪽)
//   - leftCur: 왼쪽 현재 온도 (STATUS에서 받은 원시 바이트)
//   - rightCur: 오른쪽 현재 온도 (STATUS에서 받은 원시 바이트)
//   - leftTarget: 왼쪽 목표 온도 (encodeTemp로 인코딩한 값)
//   - rightTarget: 오른쪽 목표 온도 (encodeTemp로 인코딩한 값)
//
// 반환: Uint8Array (20바이트 패킷)
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

// buildPowerOn은 전원 켜기 패킷을 생성합니다.
//
// JavaScript 호출:
//   - hotwatermat.buildPowerOn()              → 현재 온도 없이 (0, 0)
//   - hotwatermat.buildPowerOn(leftCur, rightCur) → 현재 온도 포함
//
// 반환: Uint8Array (20바이트 패킷)
func buildPowerOn(_ js.Value, args []js.Value) any {
	var leftCur, rightCur byte
	if len(args) >= 2 {
		leftCur = byte(args[0].Int())
		rightCur = byte(args[1].Int())
	}
	pkt := protocol.BuildPowerOn(leftCur, rightCur)
	return toJSArray(pkt[:])
}

// buildPowerOff는 전원 끄기 패킷을 생성합니다.
//
// 전원을 끈 후에도 BLE를 통해 다시 켤 수 있습니다.
//
// JavaScript 호출:
//   - hotwatermat.buildPowerOff()              → 현재 온도 없이 (0, 0)
//   - hotwatermat.buildPowerOff(leftCur, rightCur) → 현재 온도 포함
//
// 반환: Uint8Array (20바이트 패킷)
func buildPowerOff(_ js.Value, args []js.Value) any {
	var leftCur, rightCur byte
	if len(args) >= 2 {
		leftCur = byte(args[0].Int())
		rightCur = byte(args[1].Int())
	}
	pkt := protocol.BuildPowerOff(leftCur, rightCur)
	return toJSArray(pkt[:])
}

// parseStatus는 STATUS 알림 패킷을 파싱하여 JavaScript 객체로 반환합니다.
//
// JavaScript 호출:
//
//	const status = hotwatermat.parseStatus(new Uint8Array([0xB2, 0x00, ...]));
//	console.log(status.modeName);     // "HEAT"
//	console.log(status.leftCurrent);  // 33.0
//	console.log(status.poweredOff);   // false
//
// 입력: Uint8Array (20바이트 STATUS 패킷)
// 반환: JavaScript 객체 {mode, modeName, side, volume, waterLevel, leftCurrent, rightCurrent, leftTarget, rightTarget, poweredOff}
// 오류 시: {error: "에러 메시지"}
func parseStatus(_ js.Value, args []js.Value) any {
	if len(args) < 1 {
		return js.ValueOf(map[string]any{"error": "missing packet argument"})
	}
	arr := args[0]
	// JavaScript Uint8Array를 Go []byte로 복사
	pkt := make([]byte, arr.Length())
	js.CopyBytesToGo(pkt, arr)

	st, err := protocol.ParseStatus(pkt)
	if err != nil {
		return js.ValueOf(map[string]any{"error": err.Error()})
	}

	// Go Status 구조체를 JavaScript 객체로 변환
	return js.ValueOf(map[string]any{
		"mode":         int(st.Mode),      // 모드 바이트 값
		"modeName":     st.ModeName,       // 모드 이름 (예: "HEAT")
		"side":         int(st.Side),      // 좌/우 선택 바이트 값
		"volume":       st.Volume,         // 볼륨 값
		"waterLevel":   st.WaterLevel,     // 수위 값 (1=LOW, 2=OK, 3=FULL)
		"leftCurrent":  st.LeftCurrent,    // 왼쪽 현재 온도 (°C)
		"rightCurrent": st.RightCurrent,   // 오른쪽 현재 온도 (°C)
		"leftTarget":   st.LeftTarget,     // 왼쪽 목표 온도 (°C)
		"rightTarget":  st.RightTarget,    // 오른쪽 목표 온도 (°C)
		"poweredOff":   st.PoweredOff,     // 전원 꺼짐 여부
	})
}

// toJSArray는 Go []byte를 JavaScript Uint8Array로 변환합니다.
//
// WASM에서 바이트 배열(패킷 등)을 JavaScript로 전달할 때 사용합니다.
// js.CopyBytesToJS를 통해 Go 메모리의 바이트를 JavaScript Uint8Array로 복사합니다.
func toJSArray(data []byte) js.Value {
	arr := js.Global().Get("Uint8Array").New(len(data))
	js.CopyBytesToJS(arr, data)
	return arr
}
