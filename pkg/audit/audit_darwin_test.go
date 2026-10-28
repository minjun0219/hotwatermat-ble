//go:build darwin && cgo

package audit

import (
	"os"
	"testing"
)

// TestEmitDarwin은 cgo 코드가 macOS에서 빌드되고 os_log 호출이 멈추지 않는지 확인합니다.
func TestEmitDarwin(t *testing.T) {
	Emit(Record{Req: NewRequestID(), Phase: PhaseAttempt, Op: "test"})
}

func TestProcessPathDarwin(t *testing.T) {
	if p := processPath(os.Getpid()); p == "" {
		t.Error("processPath(own pid) is empty")
	}
}
