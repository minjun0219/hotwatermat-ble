package main

import (
	"encoding/json"
	"errors"
	"os"

	"github.com/minjun0219/hotwatermat-ble/pkg/ble"
	"github.com/minjun0219/hotwatermat-ble/pkg/protocol"
)

// --json 오류 출력의 code 값입니다.
// 이 CLI를 띄워 쓰는 프로그램이 재시도할지, 사용자에게 물을지 판단할 수 있게 실패 원인을 나눕니다.
const (
	codeInvalidArgument      = "invalid_argument"      // 플래그나 값이 잘못됨 → 고쳐서 다시 실행
	codeNotConfigured        = "not_configured"        // DeviceGid가 없거나 매트가 여러 대라 고를 수 없음 → setup 또는 --address
	codeBluetoothUnavailable = "bluetooth_unavailable" // 블루투스가 꺼졌거나 권한이 없음 → 사용자 조치 필요
	codeDeviceNotFound       = "device_not_found"      // 매트가 광고하지 않음 (공식 앱 연결 중, 범위 밖 등) → 잠시 뒤 재시도
	codeAuthFailed           = "auth_failed"           // 인증 응답 없음 (DeviceGid가 다름) → setup 다시
	codeVerifyFailed         = "verify_failed"         // 명령은 보냈지만 --verify의 상태 확인에 실패
	codeError                = "error"                 // 그 밖의 실패 (연결 끊김, 쓰기 실패, 상태 수신 실패 등)
)

// codedError는 오류에 --json 출력용 code를 붙입니다. 메시지는 감싼 오류 그대로입니다.
type codedError struct {
	code string
	err  error
}

func (e *codedError) Error() string { return e.err.Error() }
func (e *codedError) Unwrap() error { return e.err }

// withCode는 err에 --json 출력용 code를 붙여 반환합니다.
func withCode(code string, err error) error {
	return &codedError{code: code, err: err}
}

// errorCode는 오류의 code를 정합니다. CLI가 직접 붙인 code가 우선이고, 없으면 pkg/ble 오류로 가립니다.
func errorCode(err error) string {
	var ce *codedError
	switch {
	case errors.As(err, &ce):
		return ce.code
	case errors.Is(err, ble.ErrAdapterUnavailable):
		return codeBluetoothUnavailable
	case errors.Is(err, ble.ErrDeviceNotFound):
		return codeDeviceNotFound
	case errors.Is(err, ble.ErrAuthTimeout):
		return codeAuthFailed
	default:
		return codeError
	}
}

// errorJSON은 --json에서 명령이 실패했을 때 stdout에 출력하는 내용입니다.
type errorJSON struct {
	Code  string `json:"code"`
	Error string `json:"error"`
}

// statusJSON은 --json으로 출력하는 매트 상태입니다. 전원이 꺼져 있으면 power만 채웁니다.
type statusJSON struct {
	Power string    `json:"power"`           // "on" 또는 "off"
	Mode  string    `json:"mode,omitempty"`  // HEAT, TIMER_OFF, SLEEP, POWER, FASTHEAT, IONCARE
	Side  string    `json:"side,omitempty"`  // left, right, both
	Water string    `json:"water,omitempty"` // LOW, OK, FULL
	Left  *sideJSON `json:"left,omitempty"`
	Right *sideJSON `json:"right,omitempty"`
}

// sideJSON은 한쪽 매트의 현재 온도와 목표 온도(°C)입니다.
// '운전 대기' 중이면 standby가 true이고, target은 매트에 남아 있는 마지막 설정값입니다.
type sideJSON struct {
	Current float64 `json:"current"`
	Target  float64 `json:"target"`
	Standby bool    `json:"standby,omitempty"`
}

// commandJSON은 temp, on, off 명령의 --json 출력입니다.
type commandJSON struct {
	Sent   string      `json:"sent"`             // 보낸 명령: "temp", "on", "off"
	Left   float64     `json:"left,omitempty"`   // temp: 보낸 왼쪽 목표 온도
	Right  float64     `json:"right,omitempty"`  // temp: 보낸 오른쪽 목표 온도
	Status *statusJSON `json:"status,omitempty"` // --verify: 명령 뒤 다시 읽은 상태
}

// scanJSON은 scan 명령의 --json 출력입니다. 기기를 찾지 못하면 devices는 빈 배열입니다.
type scanJSON struct {
	Devices []deviceJSON `json:"devices"`
}

// deviceJSON은 스캔으로 찾은 기기 하나입니다.
type deviceJSON struct {
	Address string `json:"address"`
	Name    string `json:"name"`
	RSSI    int16  `json:"rssi"`
}

// newStatusJSON은 매트 상태를 --json 출력 모양으로 바꿉니다.
func newStatusJSON(st *protocol.Status) *statusJSON {
	if st.PoweredOff {
		return &statusJSON{Power: "off"}
	}
	return &statusJSON{
		Power: "on",
		Mode:  st.ModeName,
		Side:  protocol.SideName(st.Side),
		Water: protocol.WaterLevelName(st.WaterLevel),
		Left:  &sideJSON{Current: st.LeftCurrent, Target: st.LeftTarget, Standby: st.LeftStandby},
		Right: &sideJSON{Current: st.RightCurrent, Target: st.RightTarget, Standby: st.RightStandby},
	}
}

// newScanJSON은 스캔 결과를 --json 출력 모양으로 바꿉니다.
func newScanJSON(devices []ble.ScanResult) scanJSON {
	out := scanJSON{Devices: make([]deviceJSON, 0, len(devices))}
	for _, d := range devices {
		out.Devices = append(out.Devices, deviceJSON{Address: d.Address, Name: d.Name, RSSI: d.RSSI})
	}
	return out
}

// printCommandResult는 temp, on, off 명령의 결과를 JSON으로 출력합니다.
// --verify면 상태를 다시 읽어 status에 담습니다.
func printCommandResult(client *ble.Client, result commandJSON) error {
	if verify {
		st, err := readStatusAfterCommand(client)
		if err != nil {
			return err
		}
		result.Status = newStatusJSON(st)
	}
	return printJSON(result)
}

// printJSON은 v를 JSON 한 줄로 stdout에 출력합니다.
func printJSON(v any) error {
	return json.NewEncoder(os.Stdout).Encode(v)
}
