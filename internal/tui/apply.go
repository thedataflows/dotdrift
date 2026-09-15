package tui

import (
	"context"
	"errors"
	"fmt"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"

	"github.com/thedataflows/dotdrift/internal/service"
)

// The coalescing beat and ring cap (research 0059 §2): ~100 ms keeps
// chatty children from starving the render loop; 500 lines bound the
// tail's memory.
const outputRingMax = 500

// The apply pieces the compositor's driver reuses (T-tui-modals): the
// launcher/run seams, the session messages, the bounded output ring, the
// step rows, and the shared event/verdict/row translations. The M14
// apply mode (its gate, drain goroutine, and views) is deleted — the
// compositor's pump and detail inspector replaced it.

type ApplyLauncher interface {
	Preview(opts service.ApplyOpts) ([]service.StepPreview, error)
	Start(ctx context.Context, opts service.ApplyOpts) (ApplyRun, error)
}

// ApplyRun is one started apply session (satisfied by
// *service.ApplySession): the closed event stream, the frozen
// classification, cancel, and the terminal result.
type ApplyRun interface {
	Events() <-chan service.Event
	Preview() []service.StepPreview
	Cancel()
	Wait() (*service.SessionResult, error)
}

// applyHandoverMsg hands one session-built child to the program: Update
// returns tea.ExecProcess for it (the real terminal, alt-screen
// suspend/restore is bubbletea's job), and the callback releases done —
// the blocked handover caller is the session's step goroutine.
type applyHandoverMsg struct {
	cmd  *exec.Cmd
	done chan error
}

// applyStartedMsg reports Start's outcome; applyPreviewMsg the gate
// classification load's. StepOutput never becomes a message — the drain
// goroutine rings it and a tick drains the ring (research 0059 §2:
// per-line messages starve rendering). applyEventMsg carries every
// other session event; applyWaitedMsg closes the mode's run with the
// terminal result (the drain goroutine owns the MUST-drain mandate).
type (
	applyPreviewMsg struct {
		previews []service.StepPreview
		err      error
	}
	applyStartedMsg struct {
		run ApplyRun
		err error
	}
	applyEventMsg struct{ ev service.Event }
	applyTickMsg  struct{}
	// applyHandoverDoneMsg is the no-op the exec callback returns; it
	// keeps bubbletea's Send away from a nil message.
	applyHandoverDoneMsg struct{}
	applyWaitedMsg       struct {
		res *service.SessionResult
		err error
	}
)

// outputRing is the bounded line buffer between the drain goroutine and
// the render loop (research 0059 §2): appends never block the session,
// and the oldest lines fall off the cap.
type outputRing struct {
	mu    sync.Mutex
	lines []string
	max   int
}

func newOutputRing(max int) *outputRing { return &outputRing{max: max} }

func (r *outputRing) append(line string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.lines = append(r.lines, line)
	if len(r.lines) > r.max {
		r.lines = r.lines[len(r.lines)-r.max:]
	}
}

// snapshot copies the ring's current tail for rendering.
func (r *outputRing) snapshot() []string {
	r.mu.Lock()
	defer r.mu.Unlock()
	out := make([]string, len(r.lines))
	copy(out, r.lines)
	return out
}

type stepState int

const (
	stepWait stepState = iota
	stepRun
	stepDone
	stepFail
)

func (s stepState) mark() string {
	switch s {
	case stepRun:
		return "run"
	case stepDone:
		return "done"
	case stepFail:
		return "fail"
	default:
		return "wait"
	}
}

// applyRow is one step's line in the progress list, plus the hook
// sub-command currently running under it (0071's sub-steps).
type applyRow struct {
	name     string
	needsTTY bool
	state    stepState
	sub      string
	subFail  bool
	errText  string
}

// applyEndedState is the terminal screen's data: how the run finished,
// which step owns a failure or an interruption, and the resume cursor
// the next run picks up from (contract 2).
type applyEndedState struct {
	outcome      service.SessionOutcome
	stepName     string
	errText      string
	resumeCursor string
}

// applyModel is the apply mode's state machine: gate → running → ended.
// absorbEvent is the shared event → rows/ring translation (M14 mode and
// the M15 compositor's apply driver).
func absorbEvent(rows []*applyRow, _ *outputRing, backups *[]string, progress *string, ev service.Event) {
	switch e := ev.(type) {
	case service.BackupTaken:
		*backups = append(*backups,
			fmt.Sprintf("backup: %d path(s) -> %s", e.Count, filepath.Base(e.Dir)))
	case service.StepStarted:
		row := applyRowByName(rows, e.Name)
		if row == nil {
			return
		}
		if e.Sub != nil {
			row.sub = fmt.Sprintf("%d/%d  %s", e.Sub.Index+1, e.Sub.Total, e.Sub.Command)
			return
		}
		row.state = stepRun
		row.needsTTY = row.needsTTY || e.NeedsTTY
		*progress = fmt.Sprintf("%d/%d", e.Index+1, e.Total)
	case service.StepFinished:
		if row := applyRowByName(rows, e.Name); row != nil {
			row.state = stepDone
			row.sub = ""
		}
	case service.StepFailed:
		row := applyRowByName(rows, e.Name)
		if row == nil {
			return
		}
		if e.Sub != nil {
			row.sub = fmt.Sprintf("%d/%d  %s", e.Sub.Index+1, e.Sub.Total, e.Sub.Command)
			row.subFail = true
			return
		}
		row.state = stepFail
		row.errText = e.Err.Error()
	}
}

// applyRowByName finds a row by step name.
func applyRowByName(rows []*applyRow, name string) *applyRow {
	for _, r := range rows {
		if r.name == name {
			return r
		}
	}
	return nil
}

// applyEndState is the shared terminal-verdict translation (Wait's
// result or the cancel classification).
func applyEndState(res *service.SessionResult, err error) applyEndedState {
	e := applyEndedState{resumeCursor: ""}
	if res != nil {
		e.outcome = res.Outcome
		e.resumeCursor = res.FinalCursor
		if res.StepError != nil {
			e.stepName = res.StepError.Step
			e.errText = res.StepError.Err.Error()
		}
	}
	if err != nil {
		var sce *service.SessionCancelledError
		if errors.As(err, &sce) {
			e.outcome = service.OutcomeCancelled
			e.stepName = sce.StepName
		} else if e.errText == "" {
			e.errText = err.Error()
		}
	}
	return e
}

// end records the terminal state from Wait's verdict: the outcome, the
// failing or interrupted step, and the resume cursor (contract 2).
func renderApplyRow(th theme, r *applyRow) string {
	var b strings.Builder
	fmt.Fprintf(&b, "  %-4s  %s", r.state.mark(), r.name)
	if r.needsTTY {
		b.WriteString("  [tty]")
	}
	b.WriteString("\n")
	if r.sub != "" {
		if r.subFail {
			b.WriteString("  " + th.Error("fail") + "  " + r.sub + "\n")
		} else {
			b.WriteString("      · " + r.sub + "\n")
		}
	}
	if r.errText != "" {
		b.WriteString(th.Error("      "+r.errText) + "\n")
	}
	return b.String()
}
