//go:build windows

package ble

import "tinygo.org/x/bluetooth"

// writeCharacteristic은 Windows에서 WinRT 기반 write-with-response로 BLE 쓰기를 수행합니다.
//
// Windows의 WinRT BLE API는 Write() 메서드를 지원하며,
// 이는 BLE 프로토콜의 "Write Request" (응답 있는 쓰기)에 해당합니다.
//
// 온수매트 프로토콜은 반드시 write-with-response를 사용해야 합니다.
// WriteWithoutResponse를 사용하면 패킷이 손실될 수 있습니다.
func writeCharacteristic(c bluetooth.DeviceCharacteristic, data []byte) (int, error) {
	return c.Write(data)
}
