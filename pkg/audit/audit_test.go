package audit

import (
	"encoding/json"
	"regexp"
	"strings"
	"testing"
)

func TestFormatAttempt(t *testing.T) {
	line, err := Format(Record{
		Req:     "req-1",
		Phase:   PhaseAttempt,
		Op:      "temp",
		Args:    map[string]any{"left": "33", "verify": "true"},
		Version: "v0.1.0",
		Caller:  &Caller{UID: 501, PPID: 42, Parent: "/bin/zsh", TTY: true},
	})
	if err != nil {
		t.Fatal(err)
	}
	want := `{"v":1,"req":"req-1","phase":"attempt","op":"temp","args":{"left":"33","verify":"true"},"version":"v0.1.0","caller":{"uid":501,"ppid":42,"parent":"/bin/zsh","tty":true}}`
	if line != want {
		t.Errorf("Format() =\n%s\nwant\n%s", line, want)
	}
}

func TestFormatResult(t *testing.T) {
	line, err := Format(Record{
		Req:     "req-1",
		Phase:   PhaseResult,
		Op:      "on",
		Sent:    map[string]any{"power": "on"},
		Outcome: OutcomeOK,
	})
	if err != nil {
		t.Fatal(err)
	}
	want := `{"v":1,"req":"req-1","phase":"result","op":"on","sent":{"power":"on"},"outcome":"ok"}`
	if line != want {
		t.Errorf("Format() =\n%s\nwant\n%s", line, want)
	}
}

func TestFormatTruncatesError(t *testing.T) {
	line, err := Format(Record{Req: "r", Phase: PhaseResult, Op: "status", Outcome: "error", Error: strings.Repeat("가", 500)})
	if err != nil {
		t.Fatal(err)
	}
	var got Record
	if err := json.Unmarshal([]byte(line), &got); err != nil {
		t.Fatal(err)
	}
	if n := len([]rune(got.Error)); n != maxErrorLen+1 {
		t.Errorf("error length = %d runes, want %d", n, maxErrorLen+1)
	}
}

func TestNewRequestID(t *testing.T) {
	uuid := regexp.MustCompile(`^[0-9a-f]{8}-[0-9a-f]{4}-4[0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}$`)
	a, b := NewRequestID(), NewRequestID()
	if !uuid.MatchString(a) {
		t.Errorf("NewRequestID() = %q, not a v4 UUID", a)
	}
	if a == b {
		t.Errorf("NewRequestID() returned the same ID twice: %q", a)
	}
}
