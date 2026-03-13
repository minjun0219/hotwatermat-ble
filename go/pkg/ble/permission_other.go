//go:build !darwin

package ble

import "fmt"

// wrapBLEEnableError wraps adapter.Enable() errors with generic guidance.
func wrapBLEEnableError(err error) error {
	return fmt.Errorf(`%w

Bluetooth adapter unavailable. Please check:
  - Bluetooth is enabled on your system
  - Required permissions are granted (e.g., running as root for BlueZ on Linux)`, err)
}

// openBLESettings is a no-op on non-macOS platforms.
func openBLESettings() {}
