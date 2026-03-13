//go:build linux

package ble

import "tinygo.org/x/bluetooth"

// writeCharacteristic writes data to a BLE characteristic.
// Note: tinygo bluetooth v0.14 does not expose Write() (write with response) on Linux.
// WriteWithoutResponse internally calls org.bluez.GattCharacteristic1.WriteValue via D-Bus,
// and BlueZ handles the write type based on characteristic properties.
// In practice, this works reliably for the KDO_HotWaterMat protocol, but write-with-response
// semantics (ACK from peripheral) are not guaranteed at the application level.
func writeCharacteristic(c bluetooth.DeviceCharacteristic, data []byte) (int, error) {
	return c.WriteWithoutResponse(data)
}
