//go:build darwin

package ble

import "fmt"

// wrapBLEEnableError는 macOS에서 BLE 어댑터 활성화 실패 시 오류 메시지를 보강합니다.
//
// macOS의 경우 블루투스 권한이 앱 단위로 관리되므로,
// 터미널 앱에 블루투스 접근 권한을 부여해야 합니다.
//
// 해결 방법:
//
//	시스템 설정 → 개인 정보 보호 및 보안 → Bluetooth → 사용 중인 터미널 앱 허용
func wrapBLEEnableError(err error) error {
	return fmt.Errorf(`enable BLE adapter: %w

Bluetooth permission denied or adapter unavailable.
Please allow Bluetooth access for your terminal app:
  System Settings → Privacy & Security → Bluetooth → Allow your terminal

To open System Settings directly, run:
  open "x-apple.systempreferences:com.apple.preference.security?Privacy_Bluetooth"`, err)
}
