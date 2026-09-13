package tui

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"testing"
	"time"

	"charm.land/bubbletea/v2"
	"github.com/stretchr/testify/require"
	"github.com/thedataflows/dotdrift/internal/drift"
	"github.com/thedataflows/dotdrift/internal/facts"
	"github.com/thedataflows/dotdrift/internal/service"
)

var errBoom = errors.New("boom")

// The apply mode (T-tui-apply): the plan gate on Preview, the streamed
// progress view-model over the session's events, tea.ExecProcess
// handover, gated cancel. Pure state-machine tests through the narrow
// ApplyLauncher/ApplyRun interfaces; the handover and E2E tests run a
// real pipe-backed tea.Program (apply_handover_test.go).

// fakeLauncher is the consumer-side doorway, faked: previews are canned,
// Start counts calls and hands back the canned run or error.
type fakeLauncher struct {
	previews    []service.StepPreview
	previewErr  error
	startErr    error
	run         ApplyRun
	started     int
	startedOpts []service.ApplyOpts
}

func (f *fakeLauncher) Preview(service.ApplyOpts) ([]service.StepPreview, error) {
	return f.previews, f.previewErr
}

func (f *fakeLauncher) Start(_ context.Context, opts service.ApplyOpts) (ApplyRun, error) {
	f.started++
	f.startedOpts = append(f.startedOpts, opts)
	if f.startErr != nil {
		return nil, f.startErr
	}
	return f.run, nil
}

// testRun is an ApplyRun fake: a controllable event channel and result.
type testRun struct {
	previews  []service.StepPreview
	events    chan service.Event
	result    *service.SessionResult
	err       error
	cancelled int
}

func newTestRun() *testRun {
	return &testRun{events: make(chan service.Event)}
}

func (r *testRun) Events() <-chan service.Event          { return r.events }
func (r *testRun) Preview() []service.StepPreview        { return r.previews }
func (r *testRun) Cancel()                               { r.cancelled++ }
func (r *testRun) Wait() (*service.SessionResult, error) { return r.result, r.err }

// recordSender collects what the model hands to the program's Send.
// The drain goroutine calls it, so reads take the lock.
type recordSender struct {
	mu   sync.Mutex
	msgs []tea.Msg
}

func (s *recordSender) send(m tea.Msg) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.msgs = append(s.msgs, m)
}

func (s *recordSender) all() []tea.Msg {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]tea.Msg(nil), s.msgs...)
}

func (s *recordSender) waitFor(t *testing.T, want func(tea.Msg) bool) tea.Msg {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		for _, m := range s.all() {
			if want(m) {
				return m
			}
		}
		time.Sleep(2 * time.Millisecond)
	}
	t.Fatal("expected message never arrived")
	return nil
}

func gateLauncher() *fakeLauncher {
	return &fakeLauncher{previews: []service.StepPreview{
		{Name: "hooks-pre", NeedsTTY: true, Reason: "interactive hook commands run on your terminal"},
		{Name: "packages", NeedsTTY: false},
		{Name: "dotfiles-system", NeedsTTY: true, Reason: "elevated system edits: edit targets are not user-writable (sudo)"},
	}}
}

func loadGate(t *testing.T, l *fakeLauncher, sender *recordSender) *applyModel {
	t.Helper()
	m := newApplyModel(l, "/profile", "/state", sender.send, newTheme(true))
	cmd := m.loadPreview()
	require.NotNil(t, cmd, "the gate loads its preview async")
	m.update(cmd())
	return m
}

// The gate announces what Preview classified, before anything runs:
// "N steps will take the terminal" with the per-step reasons.
func TestApplyGate_previewAnnouncesTTY(t *testing.T) {
	l := gateLauncher()
	sender := &recordSender{}
	m := loadGate(t, l, sender)
	view := m.view()

	require.Contains(t, view, "2 steps will take the terminal")
	require.Contains(t, view, "hooks-pre")
	require.Contains(t, view, "interactive hook commands run on your terminal")
	require.Contains(t, view, "dotfiles-system")
	require.Contains(t, view, "sudo")
	require.Contains(t, view, "packages", "every classified step is listed")
	require.Equal(t, 0, l.started, "the gate never starts a session")
}

// Confirming the gate starts exactly one session, with the TUI's
// consumption policy: event mode (nil Output), --yes for pane-run steps
// (the null-stdin trap, issue 0028), handover available, and the TUI's
// handover bridge wired.
func TestApplyGate_confirmStartsSession(t *testing.T) {
	run := newTestRun()
	l := &fakeLauncher{previews: []service.StepPreview{{Name: "packages"}}, run: run}
	sender := &recordSender{}
	m := loadGate(t, l, sender)

	start := m.handleKey("y")
	require.NotNil(t, start, "confirm returns the start cmd")
	require.Equal(t, 0, l.started, "Start runs in the cmd, not in Update")

	msg := start()
	started, ok := msg.(applyStartedMsg)
	require.True(t, ok, "the cmd yields applyStartedMsg, got %T", msg)
	require.NoError(t, started.err)
	require.Equal(t, 1, l.started, "exactly one session")

	opts := l.startedOpts[0]
	require.Equal(t, "/profile", opts.ProfilePath)
	require.Equal(t, "/state", opts.StatePath)
	require.Nil(t, opts.Output, "event mode: no passthrough writer")
	require.NotNil(t, opts.Handover, "the TUI wires its handover bridge")
	require.NotNil(t, opts.HandoverAvailable)
	require.True(t, *opts.HandoverAvailable, "the TUI can always hand the terminal over")
	require.True(t, opts.Yes, "pane-run steps keep --yes (issue 0028)")

	m.update(started)
	require.Equal(t, applyRunning, m.phase)
}

// Declining the gate closes the mode; no session, no lock, nothing.
func TestApplyGate_declineWritesNothing(t *testing.T) {
	l := &fakeLauncher{previews: []service.StepPreview{{Name: "packages"}}}
	sender := &recordSender{}

	for _, key := range []string{"n", "esc"} {
		m := newApplyModel(l, "/profile", "/state", sender.send, newTheme(true))
		m.update(m.loadPreview()())
		m.handleKey(key)
		require.True(t, m.closeRequested(), "%s declines and requests close", key)
	}
	require.Equal(t, 0, l.started, "declining never starts a session")
}

// runningModel drives the gate through confirm and Start, leaving the
// model in the running phase with the named steps pending. The events
// channel stays open for the test to feed.
func runningModel(t *testing.T, steps ...string) (*applyModel, *testRun, *recordSender) {
	t.Helper()
	pls := make([]service.StepPreview, len(steps))
	for i, s := range steps {
		pls[i] = service.StepPreview{Name: s}
	}
	run := newTestRun()
	run.previews = pls
	l := &fakeLauncher{previews: pls, run: run}
	sender := &recordSender{}
	m := loadGate(t, l, sender)
	start := m.handleKey("y")
	m.update(start())
	return m, run, sender
}

func feed(m *applyModel, evs ...service.Event) {
	for _, ev := range evs {
		m.update(applyEventMsg{ev: ev})
	}
}

// The full event sequence renders the progress list correctly: rows from
// the frozen classification, states advancing, backups noted, completion
// announced.
func TestApplyModel_eventSequence(t *testing.T) {
	m, run, _ := runningModel(t, "hooks-pre", "packages", "dotfiles")

	feed(m,
		service.PlanResolved{Cursor: "", CursorEffective: false},
		service.BackupTaken{Dir: "/state/backups/20260912T000000", Count: 3},
		service.StepStarted{Name: "hooks-pre", Index: 0, Total: 3},
		service.StepFinished{Name: "hooks-pre"},
		service.StepStarted{Name: "packages", Index: 1, Total: 3, NeedsTTY: false},
		service.StepFinished{Name: "packages"},
		service.StepStarted{Name: "dotfiles", Index: 2, Total: 3},
		service.StepFinished{Name: "dotfiles"},
		service.SessionEnded{Outcome: service.OutcomeCompleted},
	)
	run.result = &service.SessionResult{Outcome: service.OutcomeCompleted}
	m.update(applyWaitedMsg{res: run.result})

	view := m.view()
	require.Contains(t, view, "apply completed")
	require.Contains(t, view, "backup: 3 path(s) -> 20260912T000000")
	require.Equal(t, applyEnded, m.phase)
	for _, name := range []string{"hooks-pre", "packages", "dotfiles"} {
		require.Contains(t, view, "done  "+name, "%s renders done", name)
	}
}

// Step states render: pending rows, running with the progress counter,
// done, failed with the step error, NeedsTTY markers, and hook
// sub-commands under their step.
func TestApplyModel_stepStates(t *testing.T) {
	m, _, _ := runningModel(t, "hooks-pre", "packages", "dotfiles-system")

	// Pending rows before anything starts.
	view := m.view()
	for _, name := range []string{"hooks-pre", "packages", "dotfiles-system"} {
		require.Contains(t, view, "wait  "+name)
	}

	feed(m,
		service.StepStarted{Name: "hooks-pre", Index: 0, Total: 3, NeedsTTY: true},
		service.StepStarted{Name: "hooks-pre", Index: 0, Total: 3,
			Sub: &service.SubStep{Index: 0, Total: 2, Command: "echo one"}},
	)
	view = m.view()
	require.Contains(t, view, "run   hooks-pre")
	require.Contains(t, view, "1/2", "the hook sub-command's position shows")
	require.Contains(t, view, "echo one")
	require.Contains(t, view, "tty", "NeedsTTY steps carry the marker")
	require.Contains(t, view, "1/3", "the progress counter names the step position")

	feed(m,
		service.StepFailed{Name: "hooks-pre",
			Err: &service.StepError{Step: "hooks-pre", Err: context.DeadlineExceeded},
			Sub: &service.SubStep{Index: 1, Total: 2, Command: "echo two"}},
	)
	view = m.view()
	require.Contains(t, view, "echo two")
	require.Contains(t, view, "fail", "the failing sub-command marks")

	feed(m,
		service.StepFinished{Name: "hooks-pre"},
		service.StepStarted{Name: "packages", Index: 1, Total: 3},
		service.StepFailed{Name: "packages", Err: &service.StepError{Step: "packages", Err: errBoom}},
	)
	view = m.view()
	require.Contains(t, view, "done  hooks-pre")
	require.Contains(t, view, "fail  packages")
	require.Contains(t, view, "boom", "the step's error text shows")
}

// Cancel is the explicit gated exit: x asks, y cancels the session, and
// the cancelled screen names the interrupted step plus the resume hint
// (contract 2: the cursor names the last completed step).
func TestApplyModel_cancelNamesStep(t *testing.T) {
	m, run, _ := runningModel(t, "packages", "dotfiles-system")
	feed(m, service.StepStarted{Name: "dotfiles-system", Index: 1, Total: 2, NeedsTTY: true})

	cmd := m.handleKey("x")
	require.Nil(t, cmd, "x opens the confirm, nothing runs yet")
	require.Equal(t, 0, run.cancelled, "the confirm gates the cancel")
	require.Contains(t, m.view(), "cancel", "the confirm is visible")

	m.handleKey("n")
	require.Equal(t, 0, run.cancelled, "n keeps the run alive")
	require.Equal(t, applyRunning, m.phase)

	m.handleKey("x")
	cancelCmd := m.handleKey("y")
	require.Nil(t, cancelCmd)
	require.Equal(t, 1, run.cancelled, "y cancels the session")

	m.update(applyWaitedMsg{
		res: &service.SessionResult{Outcome: service.OutcomeCancelled, FinalCursor: "packages"},
		err: &service.SessionCancelledError{StepName: "dotfiles-system"},
	})
	require.Equal(t, applyEnded, m.phase)
	view := m.view()
	require.Contains(t, view, "apply cancelled")
	require.Contains(t, view, "interrupted during dotfiles-system")
	require.Contains(t, view, "resumes after packages")
}

// The already-running refusal renders the typed error and leaves no
// second session behind.
func TestApplyModel_alreadyRunningRefusal(t *testing.T) {
	run := newTestRun()
	l := &fakeLauncher{
		previews: []service.StepPreview{{Name: "packages"}},
		run:      run,
		startErr: &service.AlreadyRunningError{StatePath: "/state.lock"},
	}
	sender := &recordSender{}
	m := loadGate(t, l, sender)

	start := m.handleKey("y")
	m.update(start())
	require.Equal(t, applyEnded, m.phase)
	require.Equal(t, 1, l.started, "exactly one Start attempt")
	require.Contains(t, m.view(), "another apply is already running (state file /state.lock)")
	require.Contains(t, m.view(), "esc back", "the refusal's exit is esc")
}

// The failed screen names the failing step, its error, and the resume
// cursor (contract 2).
func TestApplyModel_failedScreen(t *testing.T) {
	m, run, _ := runningModel(t, "packages", "dotfiles")
	feed(m,
		service.StepStarted{Name: "packages", Index: 0, Total: 2},
		service.StepFinished{Name: "packages"},
		service.StepStarted{Name: "dotfiles", Index: 1, Total: 2},
		service.StepFailed{Name: "dotfiles", Err: &service.StepError{Step: "dotfiles", Err: errBoom}},
	)
	run.result = &service.SessionResult{
		Outcome:     service.OutcomeFailed,
		StepError:   &service.StepError{Step: "dotfiles", Err: errBoom},
		FinalCursor: "packages",
	}
	m.update(applyWaitedMsg{res: run.result})

	view := m.view()
	require.Contains(t, view, "apply failed")
	require.Contains(t, view, "step dotfiles failed: boom")
	require.Contains(t, view, "resumes after packages")
}

// The completed screen closes the loop: cursor empty, nothing to resume.
func TestApplyModel_completedScreen(t *testing.T) {
	m, run, _ := runningModel(t, "packages")
	feed(m,
		service.StepStarted{Name: "packages", Index: 0, Total: 1},
		service.StepFinished{Name: "packages"},
		service.SessionEnded{Outcome: service.OutcomeCompleted},
	)
	run.result = &service.SessionResult{Outcome: service.OutcomeCompleted, FinalCursor: ""}
	m.update(applyWaitedMsg{res: run.result})

	view := m.view()
	require.Contains(t, view, "apply completed")
	require.NotContains(t, view, "resumes", "a completed run has no resume hint")
	require.True(t, m.closeRequested() == false, "ended waits for esc")
	m.handleKey("esc")
	require.True(t, m.closeRequested(), "esc leaves the ended screen")
}

// A burst of StepOutput never renders per line: the drain goroutine
// rings it (bounded), nothing reaches the program per line, and a tick
// drains the ring into the view once.
func TestApplyModel_outputCoalescing(t *testing.T) {
	m, run, sender := runningModel(t, "packages")

	const burst = 2000
	go func() {
		for i := range burst {
			run.events <- service.StepOutput{Name: "packages", Chunk: []byte(fmt.Sprintf("line %d", i))}
		}
		close(run.events)
	}()

	waited := sender.waitFor(t, func(msg tea.Msg) bool {
		_, ok := msg.(applyWaitedMsg)
		return ok
	})
	m.update(waited)

	// No per-line messages crossed the bridge.
	for _, msg := range sender.all() {
		if ev, ok := msg.(applyEventMsg); ok {
			_, isOutput := ev.ev.(service.StepOutput)
			require.False(t, isOutput, "StepOutput must never become a program message")
		}
	}
	// No tick yet: the burst is invisible until one lands.
	require.NotContains(t, m.view(), "line 1999")

	m.update(applyTickMsg{})
	require.Contains(t, m.view(), "line 1999", "the tick drains the ring into the view")
	require.LessOrEqual(t, len(m.tail), outputRingMax, "the rendered tail is bounded")
	require.LessOrEqual(t, len(m.ring.snapshot()), outputRingMax, "the ring is bounded")
}

// Shell integration: `a` on the plan view opens the apply mode — it owns
// the main pane while active and the stack stays untouched underneath.

func openPlanView(t *testing.T) *Shell {
	t.Helper()
	area := &fakeReads{read: treeFixture(t)}
	m := New(area, Options{
		ProfilePath: "/home/cri/profiles/main",
		ProbesFor:   func(*facts.Facts) drift.Probes { return drift.Probes{} },
	})
	init := m.Init()
	require.NotNil(t, init, "Init schedules the startup modules load")
	m, _ = step(m, init())
	m, _ = step(m, tea.WindowSizeMsg{Width: 100, Height: 30})
	m.walkTo(treeItem{kind: kindView, view: viewPlan, label: "plan"})
	m.syncSelection() // the plan read loads async; the mode test does not need it
	return m
}

func TestShell_applyOpensFromPlanView(t *testing.T) {
	l := gateLauncher()
	m := openPlanView(t)
	m.opts.ApplyFor = func(*facts.Facts) ApplyLauncher { return l }
	require.Nil(t, m.apply, "no apply mode initially")

	next, _ := m.handleKey(keyPress("a"))
	m = next.(*Shell)
	require.NotNil(t, m.apply, "a opens the apply mode from the plan view")
	require.Equal(t, focusMain, m.focus)
	require.Equal(t, 0, l.started)

	// The gate renders in the main pane; esc declines and the plan view
	// is exactly where it was.
	m.apply.handleKey("y") // startRun cmd — the shell threads it; not run here
	m.apply = newApplyModel(l, m.opts.ProfilePath, m.opts.StatePath, func(tea.Msg) {}, m.th)
	m.apply.update(m.apply.loadPreview()())

	require.NotEqual(t, "", m.apply.view())
	m.apply.handleKey("esc")
	require.True(t, m.apply.closeRequested())
}

// While a session runs, quit routes into the cancel gate: q and ctrl+c
// never kill the program outright.
func TestShell_runningApplyRoutesKeys(t *testing.T) {
	run := newTestRun()
	l := &fakeLauncher{previews: []service.StepPreview{{Name: "packages"}}, run: run}
	m := openPlanView(t)
	m.opts.ApplyFor = func(*facts.Facts) ApplyLauncher { return l }

	next, _ := m.handleKey(keyPress("a"))
	m = next.(*Shell)
	start := m.apply.handleKey("y")
	m.apply.update(start())
	require.Equal(t, applyRunning, m.apply.phase)

	// q routes to the cancel confirm — no tea.Quit comes back.
	next, cmd := m.handleKey(keyPress("q"))
	m = next.(*Shell)
	require.Nil(t, cmd, "q during a running apply never quits")
	require.True(t, m.apply.cancelAsk, "q opens the cancel confirm")

	m.apply.handleKey("n")
	require.False(t, m.apply.cancelAsk)

	// x still opens the confirm; esc during running pops nothing.
	m.apply.handleKey("x")
	require.True(t, m.apply.cancelAsk)
	m.apply.handleKey("esc")
	require.False(t, m.apply.cancelAsk)
	require.NotNil(t, m.apply, "esc does not leave a running apply")
}
