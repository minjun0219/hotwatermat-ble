//go:build !darwin && !linux

package ble

import (
	"fmt"
	"runtime"

	"tinygo.org/x/bluetooth"
)

// writeCharacteristic은 지원되지 않는 플랫폼에서 BLE 쓰기를 시도할 때 오류를 반환합니다.
//
// 현재 BLE 쓰기가 지원되는 플랫폼은 macOS(darwin)와 Linux뿐입니다.
// Windows 등 다른 OS에서 실행하면 이 함수가 호출되며, 항상 오류를 반환합니다.
//
// 지원 플랫폼 추가가 필요하면 write_<platform>.go 파일을 새로 만들고
// 빌드 태그를 적절히 설정하면 됩니다.
func writeCharacteristic(c bluetooth.DeviceCharacteristic, data []byte) (int, error) {
	return 0, fmt.Errorf("BLE write not supported on %s. Supported platforms: macOS (darwin), Linux", runtime.GOOS)
}
