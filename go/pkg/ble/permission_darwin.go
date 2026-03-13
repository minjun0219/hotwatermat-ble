//go:build darwin

package ble

import "fmt"

// wrapBLEEnableError wraps adapter.Enable() errors with macOS-specific guidance.
func wrapBLEEnableError(err error) error {
	return fmt.Errorf(`enable BLE adapter: %w

Bluetooth permission denied or adapter unavailable.
Please allow Bluetooth access for your terminal app:
  System Settings → Privacy & Security → Bluetooth → Allow your terminal

To open System Settings directly, run:
  open "x-apple.systempreferences:com.apple.preference.security?Privacy_Bluetooth"`, err)
}
