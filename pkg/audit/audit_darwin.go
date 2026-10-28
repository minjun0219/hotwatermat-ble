//go:build darwin && cgo

package audit

/*
#include <stdlib.h>
#include <unistd.h>
#include <libproc.h>
#include <os/log.h>

static os_log_t audit_log;

static void audit_init(const char *subsystem, const char *category) {
	audit_log = os_log_create(subsystem, category);
}

// Default 레벨은 디스크에 남습니다 (Info와 Debug는 기본 설정에서 메모리에만 남음).
// %{public}s가 아니면 통합 로그가 내용을 <private>로 가립니다.
static void audit_write(const char *msg) {
	os_log(audit_log, "%{public}s", msg);
}

static int audit_pid_path(int pid, char *buf, unsigned int size) {
	return proc_pidpath(pid, buf, size);
}
*/
import "C"

import (
	"sync"
	"unsafe"
)

var initOnce sync.Once

// write는 기록 한 줄을 통합 로그로 내보냅니다.
func write(line string) {
	initOnce.Do(func() {
		subsystem := C.CString(Subsystem)
		category := C.CString(Category)
		C.audit_init(subsystem, category)
		// os_log_create가 문자열을 복사하므로 해제해도 됨
		C.free(unsafe.Pointer(subsystem))
		C.free(unsafe.Pointer(category))
	})
	msg := C.CString(line)
	defer C.free(unsafe.Pointer(msg))
	C.audit_write(msg)
}

// processPath는 pid의 실행 파일 경로를 반환합니다. 알 수 없으면 빈 문자열입니다.
func processPath(pid int) string {
	buf := make([]byte, C.PROC_PIDPATHINFO_MAXSIZE)
	n := C.audit_pid_path(C.int(pid), (*C.char)(unsafe.Pointer(&buf[0])), C.uint(len(buf)))
	if n <= 0 {
		return ""
	}
	return string(buf[:n])
}

// stdinIsTerminal은 표준 입력이 터미널인지 확인합니다.
func stdinIsTerminal() bool {
	return C.isatty(0) == 1
}
