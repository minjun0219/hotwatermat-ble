//go:build darwin

package ble

import "tinygo.org/x/bluetooth"

// writeCharacteristic writes data to a BLE characteristic with response.
// macOS CoreBluetooth supports Write() (write with response).
func writeCharacteristic(c bluetooth.DeviceCharacteristic, data []byte) (int, error) {
	return c.Write(data)
}
