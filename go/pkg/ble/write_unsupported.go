//go:build !darwin && !linux

package ble

import (
	"fmt"
	"runtime"

	"tinygo.org/x/bluetooth"
)

// writeCharacteristic is not supported on this platform.
func writeCharacteristic(c bluetooth.DeviceCharacteristic, data []byte) (int, error) {
	return 0, fmt.Errorf("BLE write not supported on %s. Supported platforms: macOS (darwin), Linux", runtime.GOOS)
}
