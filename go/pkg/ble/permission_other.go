//go:build !darwin

package ble

import "fmt"

// wrapBLEEnableError는 macOS 이외의 플랫폼에서 BLE 어댑터 활성화 실패 시 오류 메시지를 보강합니다.
//
// Linux의 경우 일반적으로 다음을 확인해야 합니다:
//   - 블루투스가 시스템에서 활성화되어 있는지 (systemctl status bluetooth)
//   - 필요한 권한이 있는지 (Linux에서 BlueZ는 보통 root 권한이 필요)
//   - hci0 등 블루투스 어댑터가 장착되어 있는지 (hciconfig)
func wrapBLEEnableError(err error) error {
	return fmt.Errorf(`enable BLE adapter: %w

Bluetooth adapter unavailable. Please check:
  - Bluetooth is enabled on your system
  - Required permissions are granted (e.g., running as root for BlueZ on Linux)`, err)
}
