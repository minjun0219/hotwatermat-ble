//go:build linux

package ble

import "tinygo.org/x/bluetooth"

// writeCharacteristic writes data to a BLE characteristic.
// Linux BlueZ WriteWithoutResponse internally calls org.bluez.GattCharacteristic1.WriteValue,
// which handles write type automatically.
func writeCharacteristic(c bluetooth.DeviceCharacteristic, data []byte) (int, error) {
	return c.WriteWithoutResponse(data)
}
