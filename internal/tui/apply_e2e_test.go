package tui

import (
	"context"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"testing"
	"time"

	"charm.land/bubbletea/v2"
	"github.com/stretchr/testify/require"
	"github.com/thedataflows/dotdrift/internal/facts"
	"github.com/thedataflows/dotdrift/internal/mise"
	"github.com/thedataflows/dotdrift/internal/service"
)

// E2E (T-tui-apply): a REAL service session — real load/resolve, real
// pipeline, real sidecar lock — driving the apply model inside a real
// pipe-backed tea.Program. Two integration truths are pinned here: a
// NeedsTTY step's child actually runs through tea.ExecProcess with the
// program's stdio wired, and the gated cancel reaches a running session
// mid-step and stops it. (While a handover child owns the terminal the
// event loop is tea-suspended — cancel is a pane-run affordance; the
// child group-kill itself is the session's, covered service-side.)

// scriptBin writes an executable sh script and returns its path.
func scriptBin(t *testing.T, body string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "mise")
	require.NoError(t, os.WriteFile(path, []byte("#!/bin/sh\n"+body), 0o755))
	return path
}

// blockingBackend's Present blocks until its context dies — a pane-run
// step in flight for the cancel test.
type blockingBackend struct {
	started chan struct{}
}

func (b *blockingBackend) Present(ctx context.Context, _ []string) error {
	close(b.started)
	<-ctx.Done()
	return ctx.Err()
}

func (b *blockingBackend) Absent(context.Context, []string) error { return nil }
func (b *blockingBackend) IsInstalled(context.Context, string) (bool, error) {
	return false, nil
}
func (b *blockingBackend) Installed(context.Context) ([]string, error) { return nil, nil }
func (b *blockingBackend) DirectDeps(context.Context, string) ([]string, error) {
	return nil, nil
}

func boolPtr(v bool) *bool { return &v }

// e2eLauncher wraps the real apply area, records the started run, and
// scopes the session to the given sections. paneRun forces the session
// into pane-run mode (HandoverAvailable=false): hook children run
// through the piped mise runner instead of the terminal handover.
type e2eLauncher struct {
	inner    *service.ApplyArea
	run      chan ApplyRun
	sections map[string]bool
	paneRun  bool
}

func (l *e2eLauncher) Preview(opts service.ApplyOpts) ([]service.StepPreview, error) {
	return l.inner.Preview(opts)
}

func (l *e2eLauncher) Start(ctx context.Context, opts service.ApplyOpts) (ApplyRun, error) {
	opts.Sections = l.sections
	if l.paneRun {
		opts.HandoverAvailable = boolPtr(false)
	}
	run, err := l.inner.Start(ctx, opts)
	if err == nil {
		select {
		case l.run <- run:
		default:
		}
	}
	return run, err
}

// e2eModel wires a real area into an apply model hosted in a live
// program. The send seam starts as a lazy forward: it is bound to the
// program's Send before anything can fire a handover (the gate has not
// been confirmed yet).
func e2eModel(t *testing.T, script string, sections map[string]bool, paneRun bool) (*applyModel, func(tea.Msg), *safeBuf, chan ApplyRun, func()) {
	t.Helper()
	deps := service.ApplyDeps{}.WithDefaults()
	f := &facts.Facts{Hostname: "myhost", Username: "cri", OS: "linux", Backend: "apt"}
	deps.Detect = func() (*facts.Facts, error) { return f, nil }
	deps.NewMise = func() *mise.Mise {
		return &mise.Mise{
			LookPath: func(string) (string, error) { return script, nil },
			Run: func(_ string, args ...string) (string, error) {
				for _, a := range args {
					if a == "--version" {
						return mise.MinMiseVersion + "\n", nil
					}
				}
				return "", nil
			},
			Classify: func(string) mise.InstallKind { return mise.InstallKindUserManaged },
		}
	}
	deps.StdinIsTerminal = func() bool { return false }

	profileRoot := filepath.Join("..", "..", "testdata", "profiles", "resolve")
	l := &e2eLauncher{
		inner:    service.NewApplyArea(deps),
		run:      make(chan ApplyRun, 1),
		sections: sections,
		paneRun:  paneRun,
	}
	m := newApplyModel(l, profileRoot, filepath.Join(t.TempDir(), "state", "state.json"),
		nil, newTheme(true))
	var send func(tea.Msg)
	m.send = func(msg tea.Msg) { send(msg) }

	send, out, quit := startApplyProgram(t, m)
	return m, send, out, l.run, quit
}

// driveToRunning loads the preview and confirms the gate, all through
// the program's own loop.
func driveToRunning(t *testing.T, send func(tea.Msg), m *applyModel) {
	t.Helper()
	preview := m.loadPreview()
	send(preview()) // the classification is a pure read
	send(keyPress("y"))
}

// waitFor polls until the predicate holds or the deadline passes.
func waitFor(t *testing.T, what string, d time.Duration, ok func() bool) {
	t.Helper()
	deadline := time.Now().Add(d)
	for time.Now().Before(deadline) {
		if ok() {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("timed out waiting for %s", what)
}

// A NeedsTTY step runs its real child through the bridge: the child is
// an actual process (a script standing in for mise), its stdout lands
// in the program's output — stdio wired by tea.ExecProcess — and the
// session completes once it exits.
func TestApplyE2E_handoverRunsRealChild(t *testing.T) {
	marker := filepath.Join(t.TempDir(), "hook-ran")
	script := scriptBin(t, "echo hook-output\ntouch "+marker+"\n")

	m, send, out, runs, quit := e2eModel(t, script, map[string]bool{"hooks": true}, false)
	defer quit()

	driveToRunning(t, send, m)

	var run ApplyRun
	select {
	case run = <-runs:
	case <-time.After(10 * time.Second):
		t.Fatal("session never started")
	}
	res, err := run.Wait()
	require.NoError(t, err)
	require.Equal(t, service.OutcomeCompleted, res.Outcome)

	waitFor(t, "the hook child's marker", 5*time.Second, func() bool {
		_, err := os.Stat(marker)
		return err == nil
	})
	require.Contains(t, out.String(), "hook-output",
		"the handover child ran with the program's output wired")
}

// Cancel mid-handover: the session handle's cancel (exactly what the
// gated x/y calls) group-kills the handover child even though the
// bridge runs it — the exec returns, the step fails as cancelled, and
// the program's loop comes back. The keys themselves cannot fire while
// a child owns the terminal (tea suspends the loop during exec — the
// TUI is not on screen), which is why the handle is driven directly.
func TestApplyE2E_cancelKillsHandoverChild(t *testing.T) {
	pidFile := filepath.Join(t.TempDir(), "hook.pid")
	script := scriptBin(t, "echo $$ > "+pidFile+"\nsleep 30\n")

	m, send, _, runs, quit := e2eModel(t, script, map[string]bool{"hooks": true}, false)
	defer quit()

	driveToRunning(t, send, m)

	var run ApplyRun
	select {
	case run = <-runs:
	case <-time.After(10 * time.Second):
		t.Fatal("session never started")
	}
	waitFor(t, "the handover child to start", 10*time.Second, func() bool {
		_, err := os.Stat(pidFile)
		return err == nil
	})

	t0 := time.Now()
	run.Cancel()

	res, err := run.Wait()
	t.Logf("outcome=%v stepErr=%v finalCursor=%q waitErr=%v", res.Outcome, res.StepError, res.FinalCursor, err)
	require.Error(t, err, "a cancelled session errors from Wait")
	var sce *service.SessionCancelledError
	require.ErrorAs(t, err, &sce)
	require.Equal(t, "hooks-pre", sce.StepName, "the interrupted step is named")
	require.Equal(t, service.OutcomeCancelled, res.Outcome)
	require.Less(t, time.Since(t0), 10*time.Second, "cancel lands promptly, not at the child's runtime")

	raw, err := os.ReadFile(pidFile)
	require.NoError(t, err)
	pid, convErr := strconv.Atoi(strings.TrimSpace(string(raw)))
	require.NoError(t, convErr)
	waitFor(t, "the handover child to die", 5*time.Second, func() bool {
		return syscall.Kill(pid, syscall.Signal(0)) != nil
	})
}
