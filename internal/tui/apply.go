package tui

import (
	"context"
	"errors"
	"fmt"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"charm.land/bubbletea/v2"
	"github.com/thedataflows/dotdrift/internal/service"
)

// The coalescing beat and ring cap (research 0059 §2): ~100 ms keeps
// chatty children from starving the render loop; 500 lines bound the
// tail's memory.
const (
	outputTick    = 100 * time.Millisecond
	outputRingMax = 500
)

// Apply mode (T-tui-apply, 0062-D4): a mode replacing the main pane —
// the plan gate on the session's up-front TTY classification (0064-D4),
// the session's events streamed into a progress view-model, output
// coalescing (research 0059 §2), real-terminal handover through
// tea.ExecProcess (0064-D9), and cancel as the explicit gated exit.

// ApplyLauncher is the consumer-side narrow interface over the service
// apply area (ADR-0008's doorway): classify up front, start on confirm.
// Satisfied by *service.ApplyArea.
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

type applyPhase int

const (
	applyGate applyPhase = iota
	applyRunning
	applyEnded
)

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
type applyModel struct {
	launcher    ApplyLauncher
	profilePath string
	statePath   string
	send        func(tea.Msg)
	th          theme

	phase    applyPhase
	previews []service.StepPreview
	loadErr  error
	closeReq bool

	run       ApplyRun
	startErr  error
	ended     applyEndedState
	cancelAsk bool // the gated cancel confirm showing

	ring      *outputRing
	tail      []string
	rows      []*applyRow
	progress  string // "2/3" while a step runs
	backupMsg []string
}

func newApplyModel(l ApplyLauncher, profilePath, statePath string, send func(tea.Msg), th theme) *applyModel {
	return &applyModel{
		launcher:    l,
		profilePath: profilePath,
		statePath:   statePath,
		send:        send,
		th:          th,
	}
}

// loadPreview classifies the would-be session async (a resolve + step
// build — cheap but not free), like every other read in the shell.
func (m *applyModel) loadPreview() tea.Cmd {
	return func() tea.Msg {
		previews, err := m.launcher.Preview(m.opts())
		return applyPreviewMsg{previews: previews, err: err}
	}
}

// opts is the TUI's consumption policy, decided once: event mode (nil
// Output — the stream feeds the progress view), --yes for pane-run steps
// (a null stdin makes mise silently no-op, issue 0028), and handover
// always available — a UI that owns the terminal can always give it up
// (0064-D4).
func (m *applyModel) opts() service.ApplyOpts {
	yes, avail := true, true
	return service.ApplyOpts{
		ProfilePath:       m.profilePath,
		StatePath:         m.statePath,
		Yes:               yes,
		HandoverAvailable: &avail,
		Handover:          m.handover,
	}
}

// handover is the session's Handover seam: the child goes to the
// program as applyHandoverMsg (Update answers tea.ExecProcess), and the
// step's goroutine blocks here until the child ran — the terminal is
// the child's for exactly that window.
func (m *applyModel) handover(cmd *exec.Cmd) error {
	done := make(chan error, 1)
	m.send(applyHandoverMsg{cmd: cmd, done: done})
	return <-done
}

// handleKey runs the mode's vocabulary. The gate: y runs, n/esc leave.
// Running: x (or q/ctrl+c, routed by the shell) opens the gated cancel
// confirm — esc does NOT pop a running apply, cancel is the explicit
// exit. Ended: esc leaves.
func (m *applyModel) handleKey(s string) tea.Cmd {
	switch m.phase {
	case applyGate:
		switch s {
		case "y":
			return m.startRun()
		case "n", "esc":
			m.closeReq = true
		}
	case applyRunning:
		if m.cancelAsk {
			switch s {
			case "y":
				m.cancelAsk = false
				if m.run != nil {
					m.run.Cancel()
				}
			case "n", "esc", "x":
				m.cancelAsk = false
			}
			return nil
		}
		switch s {
		case "x":
			m.cancelAsk = true
		}
	case applyEnded:
		if s == "esc" {
			m.closeReq = true
		}
	}
	return nil
}

// update absorbs the mode's async messages. It returns the tick cmd to
// re-arm (the shell threads it into its own Update return).
func (m *applyModel) update(msg tea.Msg) tea.Cmd {
	switch msg := msg.(type) {
	case applyPreviewMsg:
		m.previews, m.loadErr = msg.previews, msg.err

	case applyStartedMsg:
		if msg.err != nil {
			m.startErr = msg.err
			m.phase = applyEnded
			return nil
		}
		m.run = msg.run
		m.phase = applyRunning
		m.ring = newOutputRing(outputRingMax)
		m.rows = make([]*applyRow, 0, len(msg.run.Preview()))
		for _, p := range msg.run.Preview() {
			m.rows = append(m.rows, &applyRow{name: p.Name, needsTTY: p.NeedsTTY})
		}
		// The run handle's mandate — events MUST be drained — is owned
		// here, once, for the one window the TUI opens.
		go m.drain(msg.run)
		return m.tick()

	case applyEventMsg:
		m.absorb(msg.ev)

	case applyHandoverMsg:
		// The one answer that leaves the model: the child gets the real
		// terminal (bubbletea suspends and restores the screen), its stdio
		// stays unwired here so ExecProcess fills it from the program
		// (0064-D9 — never pre-wired pipes), and the callback releases the
		// blocked session step with the child's outcome.
		cmd, done := msg.cmd, msg.done
		return tea.ExecProcess(cmd, func(err error) tea.Msg {
			done <- err
			return applyHandoverDoneMsg{}
		})

	case applyTickMsg:
		m.tail = m.ring.snapshot()
		if m.phase == applyRunning {
			return m.tick()
		}

	case applyWaitedMsg:
		m.end(msg)
	}
	return nil
}

// absorb folds one session event into the view-model. Semantics come
// from step boundaries and hook indexes — never from output parsing.
func (m *applyModel) absorb(ev service.Event) {
	absorbEvent(m.rows, m.ring, &m.backupMsg, &m.progress, ev)
}

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
func (m *applyModel) end(msg applyWaitedMsg) {
	m.ended = applyEndState(msg.res, msg.err)
	m.phase = applyEnded
}

// drain pumps the session's event stream into the program: output lines
// ring (never one message per line), everything else forwards, and the
// stream's close is Wait's signal to send the terminal result.
func (m *applyModel) drain(run ApplyRun) {
	for ev := range run.Events() {
		if out, ok := ev.(service.StepOutput); ok {
			m.ring.append(string(out.Chunk))
			continue
		}
		m.send(applyEventMsg{ev: ev})
	}
	res, err := run.Wait()
	m.send(applyWaitedMsg{res: res, err: err})
}

// tick re-renders the output tail on a fixed beat while the run lives.
func (m *applyModel) tick() tea.Cmd {
	return tea.Tick(outputTick, func(time.Time) tea.Msg { return applyTickMsg{} })
}

func (m *applyModel) row(name string) *applyRow {
	for _, r := range m.rows {
		if r.name == name {
			return r
		}
	}
	return nil
}

// closeRequested reports that the mode wants to leave (gate declined,
// ended acknowledged) — the shell drops it and the underlying view
// returns.
func (m *applyModel) closeRequested() bool { return m.closeReq }

// startRun starts the session in a cmd (Start resolves and locks —
// never in Update).
func (m *applyModel) startRun() tea.Cmd {
	opts := m.opts()
	return func() tea.Msg {
		run, err := m.launcher.Start(context.Background(), opts)
		return applyStartedMsg{run: run, err: err}
	}
}

func (m *applyModel) view() string {
	switch m.phase {
	case applyGate:
		return m.gateView()
	case applyRunning:
		return m.progressView()
	default:
		return m.endedView()
	}
}

// gateView renders the classification: the announcement line, then one
// row per step with its TTY reason where it has one.
func (m *applyModel) gateView() string {
	var b strings.Builder
	b.WriteString(m.th.viewTitle.Render("APPLY"))
	b.WriteString("\nrun the resolved plan against this machine\n\n")
	if m.loadErr != nil {
		b.WriteString(m.th.errorMark.Render("error: "+m.loadErr.Error()) + "\n")
		b.WriteString(m.th.meta.Render("esc back"))
		return b.String()
	}
	tty := 0
	for _, p := range m.previews {
		if p.NeedsTTY {
			tty++
		}
	}
	if tty > 0 {
		fmt.Fprintf(&b, "%d steps will take the terminal.\n\n", tty)
	}
	for _, p := range m.previews {
		b.WriteString("  " + p.Name)
		if p.NeedsTTY {
			b.WriteString(" — " + p.Reason)
		}
		b.WriteString("\n")
	}
	b.WriteString("\n" + m.th.reasonMark.Render("run apply?  y/n") + m.th.meta.Render("  · esc declines"))
	return b.String()
}

func (m *applyModel) progressView() string {
	var b strings.Builder
	b.WriteString(m.th.viewTitle.Render("APPLY"))
	b.WriteString("\napply in progress")
	if m.progress != "" {
		b.WriteString(" (" + m.progress + ")")
	}
	b.WriteString("\n\n")
	for _, note := range m.backupMsg {
		b.WriteString(m.th.meta.Render(note) + "\n")
	}
	for _, r := range m.rows {
		b.WriteString(m.renderRow(r))
	}
	if len(m.tail) > 0 {
		b.WriteString("\n" + m.th.sectionLabel.Render("output") + "\n")
		for _, line := range m.tail {
			b.WriteString("  " + line + "\n")
		}
	}
	if m.cancelAsk {
		b.WriteString("\n" + m.th.confirmLine.Render(" Really cancel? The process group is killed. (y/n)"))
	} else {
		b.WriteString("\n" + m.th.meta.Render("x cancels · a running apply keeps the pane until it ends"))
	}
	return b.String()
}

// renderRow is one step's progress line: state marker, name, tty marker,
// and any hook sub-command or error text under it.
func (m *applyModel) renderRow(r *applyRow) string { return renderApplyRow(m.th, r) }

// renderApplyRow renders one step row (M14 mode and the M15 detail
// modal).
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

func (m *applyModel) endedView() string {
	var b strings.Builder
	b.WriteString(m.th.viewTitle.Render("APPLY") + "\n")
	if m.startErr != nil {
		b.WriteString(m.th.errorMark.Render("error: "+m.startErr.Error()) + "\n")
		b.WriteString(m.th.meta.Render("esc back"))
		return b.String()
	}
	switch m.ended.outcome {
	case service.OutcomeCompleted:
		b.WriteString("apply completed\n")
	case service.OutcomeCancelled:
		b.WriteString(m.th.errorMark.Render("apply cancelled") + "\n")
		if m.ended.stepName != "" {
			b.WriteString("interrupted during " + m.ended.stepName + "\n")
		}
		b.WriteString(m.resumeLine() + "\n")
	case service.OutcomeFailed:
		b.WriteString(m.th.errorMark.Render("apply failed") + "\n")
		if m.ended.stepName != "" {
			b.WriteString("step " + m.ended.stepName + " failed: " + m.ended.errText + "\n")
		}
		b.WriteString(m.resumeLine() + "\n")
	}
	if len(m.backupMsg) > 0 || len(m.rows) > 0 {
		b.WriteString("\n")
		for _, note := range m.backupMsg {
			b.WriteString(m.th.meta.Render(note) + "\n")
		}
		for _, r := range m.rows {
			b.WriteString(m.renderRow(r))
		}
	}
	if len(m.tail) > 0 {
		b.WriteString("\n" + m.th.sectionLabel.Render("output") + "\n")
		for _, line := range m.tail {
			b.WriteString("  " + line + "\n")
		}
	}
	b.WriteString("\n" + m.th.meta.Render("esc back"))
	return b.String()
}

// resumeLine is contract 2 as a sentence: the cursor names the last
// completed step, and the next run resumes from the step after it.
func (m *applyModel) resumeLine() string {
	if m.ended.resumeCursor == "" {
		return "nothing completed — the next run starts from the beginning"
	}
	return "progress is kept — the next run resumes after " + m.ended.resumeCursor
}
