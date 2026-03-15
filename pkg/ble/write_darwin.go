//go:build darwin

package ble

import "tinygo.org/x/bluetooth"

// writeCharacteristic은 BLE 특성에 데이터를 쓰기-응답(write with response) 모드로 전송합니다.
//
// macOS의 CoreBluetooth는 Write() 메서드를 지원하며,
// 이는 BLE 프로토콜의 "Write Request" (응답 있는 쓰기)에 해당합니다.
//
// 온수매트 프로토콜은 반드시 write-with-response를 사용해야 합니다.
// WriteWithoutResponse를 사용하면 패킷이 손실될 수 있습니다.
//
// macOS에서는 CoreBluetooth가 자동으로 Write Request를 처리하므로
// 별도의 처리 없이 Write()를 호출하면 됩니다.
func writeCharacteristic(c bluetooth.DeviceCharacteristic, data []byte) (int, error) {
	return c.Write(data)
}
