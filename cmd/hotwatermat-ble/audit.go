package main

import (
	"fmt"
	"os"
	"sync"

	"github.com/minjun0219/hotwatermat-ble/pkg/audit"
	"github.com/minjun0219/hotwatermat-ble/pkg/protocol"
	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
)

// auditCommands는 매트에 닿는 명령입니다. 이 명령들은 실행할 때마다 감사 기록을 남기고, 끄는 옵션은 없습니다.
// 온열기기라 누가 언제 무엇을 했는지 남겨야 하고, 기록은 같은 사용자 계정의 에이전트가 지울 수 없는 곳(통합 로그)에 둡니다.
func auditCommands() []*cobra.Command {
	return []*cobra.Command{scanCmd, statusCmd, tempCmd, standbyCmd, onCmd, offCmd, setupCmd}
}

// auditState는 실행 중인 명령의 감사 기록 상태입니다. 시그널 핸들러도 result를 남기므로 잠금으로 보호합니다.
var auditState struct {
	sync.Mutex
	req   string
	op    string
	sent  map[string]any
	after *protocol.Status
	done  bool // result를 이미 남겼으면 true
}

// withAudit은 명령의 RunE를 감싸 실행 전에 attempt, 끝난 뒤 result를 남깁니다.
func withAudit(c *cobra.Command) {
	run := c.RunE
	c.RunE = func(cmd *cobra.Command, args []string) error {
		req := audit.NewRequestID()
		caller := audit.CurrentCaller()
		auditState.Lock()
		auditState.req, auditState.op = req, cmd.Name()
		auditState.sent, auditState.after, auditState.done = nil, nil, false
		auditState.Unlock()

		audit.Emit(audit.Record{
			Req:     req,
			Phase:   audit.PhaseAttempt,
			Op:      cmd.Name(),
			Args:    auditArgs(cmd.Flags()),
			Version: version,
			Caller:  &caller,
		})

		err := run(cmd, args)
		outcome, msg := audit.OutcomeOK, ""
		if err != nil {
			outcome, msg = errorCode(err), err.Error()
		}
		select {
		case <-interrupted:
			// 시그널로 연결을 끊어 실패한 것이므로 중단으로 남김
			outcome = "interrupted"
		default:
		}
		auditFinish(outcome, msg)
		return err
	}
}

// auditArgs는 사용자가 지정한 플래그를 기록용으로 모읍니다. DeviceGid는 가려서 남깁니다.
func auditArgs(flags *pflag.FlagSet) map[string]any {
	args := map[string]any{}
	flags.Visit(func(f *pflag.Flag) {
		v := f.Value.String()
		if f.Name == "device-gid" {
			v = maskDeviceGid(v)
		}
		args[f.Name] = v
	})
	if len(args) == 0 {
		return nil
	}
	return args
}

// auditSent는 매트에 보내기 직전의 명령 값을 기록해 둡니다. result에 담깁니다.
func auditSent(sent map[string]any) {
	auditState.Lock()
	auditState.sent = sent
	auditState.Unlock()
}

// auditAfter는 명령 뒤 읽은 매트 상태를 기록해 둡니다. result에 담깁니다.
func auditAfter(st *protocol.Status) {
	auditState.Lock()
	auditState.after = st
	auditState.Unlock()
}

// auditInterrupted는 시그널로 종료할 때 실행 중인 명령의 result를 남깁니다.
func auditInterrupted(sig os.Signal) {
	auditFinish("interrupted", fmt.Sprintf("received %s", sig))
}

// auditFinish는 실행 중인 명령의 result를 한 번만 남깁니다.
func auditFinish(outcome, msg string) {
	auditState.Lock()
	defer auditState.Unlock()
	if auditState.req == "" || auditState.done {
		return
	}
	auditState.done = true
	r := audit.Record{
		Req:     auditState.req,
		Phase:   audit.PhaseResult,
		Op:      auditState.op,
		Sent:    auditState.sent,
		Outcome: outcome,
		Error:   msg,
	}
	if auditState.after != nil {
		r.After = newStatusJSON(auditState.after)
	}
	audit.Emit(r)
}
