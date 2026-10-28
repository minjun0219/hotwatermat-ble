//go:build !darwin || !cgo

package audit

// write는 macOS 밖에서는 아직 아무것도 남기지 않습니다.
func write(line string) {}

// processPath는 macOS 밖에서는 경로를 알아내지 않습니다.
func processPath(pid int) string { return "" }

// stdinIsTerminal은 macOS 밖에서는 확인하지 않습니다.
func stdinIsTerminal() bool { return false }
