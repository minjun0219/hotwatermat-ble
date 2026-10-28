// Package audit는 매트에 닿는 조작을 감사 기록으로 남깁니다.
//
// 기록은 JSON 한 줄이며 macOS에서는 통합 로그(os_log)로 내보냅니다.
// 통합 로그는 시스템(logd)이 관리하므로 같은 사용자 계정의 프로세스가 지우거나 고칠 수 없고,
// 줄마다 보낸 프로세스의 경로가 붙어 다른 프로그램이 쓴 가짜 줄과 구분됩니다.
// 그 밖의 플랫폼에서는 아직 아무것도 남기지 않습니다.
//
// 기록 보기 (macOS):
//
//	log show --style ndjson --predicate 'subsystem == "io.github.minjun0219.hotwatermat-ble"' --last 1d
package audit

import (
	"crypto/rand"
	"encoding/json"
	"fmt"
	"os"
	"unicode/utf8"
)

// Subsystem과 Category는 통합 로그에서 감사 기록을 찾는 이름입니다.
// Subsystem은 macOS 서명 identifier(Info.plist의 CFBundleIdentifier)와 같습니다.
const (
	Subsystem = "io.github.minjun0219.hotwatermat-ble"
	Category  = "audit"
)

// 기록 단계: 매트에 닿기 전(attempt)과 끝난 뒤(result)에 한 줄씩 남깁니다.
// 명령 도중 프로세스가 죽어도 attempt는 남습니다.
const (
	PhaseAttempt = "attempt"
	PhaseResult  = "result"
)

// OutcomeOK는 성공한 조작의 outcome입니다. 실패하면 CLI의 --json 오류 code가 들어갑니다.
const OutcomeOK = "ok"

// maxErrorLen은 error 필드의 최대 글자 수입니다. 통합 로그는 긴 메시지를 잘라 버리므로 줄 전체를 짧게 유지합니다.
const maxErrorLen = 200

// Caller는 CLI를 부른 쪽에 대해 CLI가 직접 알아낸 정보입니다.
type Caller struct {
	UID    int    `json:"uid"`
	PPID   int    `json:"ppid"`
	Parent string `json:"parent,omitempty"` // 부모 프로세스의 실행 파일 경로 (알 수 없으면 비움)
	TTY    bool   `json:"tty"`              // 표준 입력이 터미널인지 (사람이 직접 쳤을 가능성)
}

// Record는 감사 기록 한 줄입니다.
type Record struct {
	V       int            `json:"v"`                 // 형식 버전
	Req     string         `json:"req"`               // attempt와 result를 잇는 요청 ID
	Phase   string         `json:"phase"`             // PhaseAttempt 또는 PhaseResult
	Op      string         `json:"op"`                // CLI 명령 이름 (scan, status, temp, on, off, setup)
	Args    map[string]any `json:"args,omitempty"`    // attempt: 지정한 플래그
	Version string         `json:"version,omitempty"` // attempt: CLI 버전
	Caller  *Caller        `json:"caller,omitempty"`  // attempt: 호출자
	Sent    map[string]any `json:"sent,omitempty"`    // result: 매트에 실제로 보낸 명령 값
	Outcome string         `json:"outcome,omitempty"` // result: OutcomeOK 또는 오류 code
	Error   string         `json:"error,omitempty"`   // result: 오류 메시지 (잘림)
	After   any            `json:"after,omitempty"`   // result: 명령 뒤 읽은 상태 (읽었을 때만)
}

// Version은 현재 기록 형식 버전입니다.
const Version = 1

// Emit은 기록 한 줄을 내보냅니다. 내보내기는 실패를 알려 주지 않으므로 반환값이 없습니다.
func Emit(r Record) {
	line, err := Format(r)
	if err != nil {
		return
	}
	write(line)
}

// Format은 기록을 JSON 한 줄로 만듭니다. V가 비어 있으면 Version을 넣고 error를 자릅니다.
func Format(r Record) (string, error) {
	if r.V == 0 {
		r.V = Version
	}
	r.Error = truncate(r.Error, maxErrorLen)
	b, err := json.Marshal(r)
	if err != nil {
		return "", err
	}
	return string(b), nil
}

// NewRequestID는 무작위 UUID(v4) 문자열을 만듭니다.
func NewRequestID() string {
	var b [16]byte
	if _, err := rand.Read(b[:]); err != nil {
		return "unknown"
	}
	b[6] = b[6]&0x0f | 0x40
	b[8] = b[8]&0x3f | 0x80
	return fmt.Sprintf("%x-%x-%x-%x-%x", b[0:4], b[4:6], b[6:8], b[8:10], b[10:16])
}

// CurrentCaller는 지금 프로세스를 부른 쪽의 정보를 모읍니다.
func CurrentCaller() Caller {
	ppid := os.Getppid()
	return Caller{
		UID:    os.Getuid(),
		PPID:   ppid,
		Parent: processPath(ppid),
		TTY:    stdinIsTerminal(),
	}
}

// truncate는 s를 최대 n글자로 자릅니다.
func truncate(s string, n int) string {
	if utf8.RuneCountInString(s) <= n {
		return s
	}
	r := []rune(s)
	return string(r[:n]) + "…"
}
