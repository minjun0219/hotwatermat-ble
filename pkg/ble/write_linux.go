//go:build linux

package ble

import "tinygo.org/x/bluetooth"

// writeCharacteristic은 BLE 특성에 데이터를 전송합니다.
//
// Linux(BlueZ) 환경에서의 제한사항:
// tinygo bluetooth v0.14에서는 Linux용 Write() (쓰기-응답 모드)가 노출되지 않습니다.
// 대신 WriteWithoutResponse()를 사용하는데, 이는 내부적으로
// D-Bus를 통해 org.bluez.GattCharacteristic1.WriteValue를 호출합니다.
//
// BlueZ는 특성의 속성(properties)에 따라 적절한 쓰기 타입을 자동 선택하므로,
// 실제로는 온수매트 프로토콜에서 안정적으로 동작합니다.
// 다만, 응용 프로그램 수준에서 쓰기-응답(ACK) 의미론이 보장되지는 않습니다.
//
// 향후 tinygo bluetooth가 Linux에서 Write()를 지원하면
// write_darwin.go처럼 Write()를 사용하도록 변경해야 합니다.
func writeCharacteristic(c bluetooth.DeviceCharacteristic, data []byte) (int, error) {
	return c.WriteWithoutResponse(data)
}
