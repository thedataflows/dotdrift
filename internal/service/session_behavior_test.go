package service

// Acceptance scenarios for the apply session (issue 0069, spec 0064):
// lock, resume/stale cursor, section filters, failure, cancel, output
// policies, backup events, preview, and the handover contract.

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"github.com/thedataflows/dotdrift/internal/facts"
	"github.com/thedataflows/dotdrift/internal/mise"
	"github.com/thedataflows/dotdrift/internal/state"
)

func baseOpts(root, statePath string) ApplyOpts {
	return ApplyOpts{
		ProfilePath: root,
		StatePath:   statePath,
		Yes:         true,
		Handover:    func(*exec.Cmd) error { return nil },
	}
}

func testFacts() *facts.Facts {
	return &facts.Facts{Hostname: "myhost", Username: "cri", OS: "linux", Backend: "paru"}
}

// Another apply holding the sidecar lock makes Start fail with
// AlreadyRunningError — never a queue, never a second run (contract 11).
func TestSession_lockHeld(t *testing.T) {
	dir := t.TempDir()
	statePath := filepath.Join(dir, "state.json")
	deps, _ := stubSessionDeps(t, testFacts())

	store := state.NewFileStore(statePath)
	ok, err := store.TryLock()
	require.NoError(t, err)
	require.True(t, ok, "test must hold the lock")
	t.Cleanup(func() { _ = store.Unlock() })

	_, err = NewApplyArea(deps).Start(context.Background(), baseOpts(resolveFixture(t), statePath))
	require.Error(t, err)
	var are *AlreadyRunningError
	require.ErrorAs(t, err, &are)
}

// A cursor naming a completed step skips through it: tools cursor runs
// only dotfiles + hooks-post, reports CursorEffective, and completes.
func TestSession_resumeSkipsThroughCursor(t *testing.T) {
	dir := t.TempDir()
	statePath := filepath.Join(dir, "state.json")
	deps, events := stubSessionDeps(t, testFacts())
	require.NoError(t, state.NewFileStore(statePath).Save(&state.State{LastCompleted: "tools"}))

	sess, err := NewApplyArea(deps).Start(context.Background(), baseOpts(resolveFixture(t), statePath))
	require.NoError(t, err)
	evs := drain(t, sess)
	res, err := sess.Wait()
	require.NoError(t, err)

	pr := evs[0].(PlanResolved)
	require.Equal(t, "tools", pr.Cursor)
	require.True(t, pr.CursorEffective)

	started := stepNames(evs)
	require.Equal(t, []string{"dotfiles", "hooks-post"}, started)
	for _, e := range *events {
		require.NotContains(t, e, "mise:run bootstrap", "packages must be skipped after a tools cursor")
		require.NotContains(t, e, "mise:run install", "tools must be skipped after a tools cursor")
	}
	require.Equal(t, OutcomeCompleted, res.Outcome)
	_, statErr := os.Stat(statePath)
	require.True(t, os.IsNotExist(statErr), "state file must be removed after a successful apply")
}

// A stale cursor (naming a step absent from this run) is ignored: every
// step runs and CursorEffective says so (contract 2).
func TestSession_staleCursorIgnored(t *testing.T) {
	dir := t.TempDir()
	statePath := filepath.Join(dir, "state.json")
	deps, _ := stubSessionDeps(t, testFacts())
	require.NoError(t, state.NewFileStore(statePath).Save(&state.State{LastCompleted: "vanished"}))

	sess, err := NewApplyArea(deps).Start(context.Background(), baseOpts(resolveFixture(t), statePath))
	require.NoError(t, err)
	evs := drain(t, sess)
	_, err = sess.Wait()
	require.NoError(t, err)

	pr := evs[0].(PlanResolved)
	require.Equal(t, "vanished", pr.Cursor)
	require.False(t, pr.CursorEffective)
	started := stepNames(evs)
	require.Equal(t, []string{"hooks-pre", "packages", "tools", "dotfiles", "hooks-post"}, started)
}

// Section selection filters the pipeline: only the selected sections run.
func TestSession_sectionsFilter(t *testing.T) {
	dir := t.TempDir()
	statePath := filepath.Join(dir, "state.json")
	deps, _ := stubSessionDeps(t, testFacts())

	opts := baseOpts(resolveFixture(t), statePath)
	opts.Sections = map[string]bool{"packages": true}
	sess, err := NewApplyArea(deps).Start(context.Background(), opts)
	require.NoError(t, err)
	evs := drain(t, sess)
	_, err = sess.Wait()
	require.NoError(t, err)

	started := stepNames(evs)
	require.Equal(t, []string{"packages"}, started)
}

// A failing step ends the session Failed with the typed StepError, leaves
// the cursor on the last completed step, and never runs later steps —
// Wait does not error (a failed step is a result, 0064-D7).
func TestSession_stepFailure(t *testing.T) {
	dir := t.TempDir()
	statePath := filepath.Join(dir, "state.json")
	f := testFacts()
	deps, _ := stubSessionDeps(t, f)
	// Swap in a mise fake whose dotfiles step explodes.
	deps.NewMise = func() *mise.Mise {
		m := fakeMise(&[]string{})
		m.Run = func(_ string, args ...string) (string, error) {
			for _, a := range args {
				if a == "--version" {
					return mise.MinMiseVersion + "\n", nil
				}
			}
			if len(args) > 0 && args[0] == "dotfiles" {
				return "", errors.New("mise exploded")
			}
			return "", nil
		}
		return m
	}

	sess, err := NewApplyArea(deps).Start(context.Background(), baseOpts(resolveFixture(t), statePath))
	require.NoError(t, err)
	evs := drain(t, sess)
	res, err := sess.Wait()
	require.NoError(t, err)
	require.NotNil(t, res)
	require.Equal(t, OutcomeFailed, res.Outcome)
	require.NotNil(t, res.StepError)
	require.Equal(t, "dotfiles", res.StepError.Step)
	require.ErrorContains(t, res.StepError, "mise exploded")
	require.Equal(t, "tools", res.FinalCursor)

	var failed *StepFailed
	for _, ev := range evs {
		if sf, ok := ev.(StepFailed); ok {
			failed = &sf
		}
	}
	require.NotNil(t, failed)
	require.Equal(t, "dotfiles", failed.Name)

	end := evs[len(evs)-1].(SessionEnded)
	require.Equal(t, OutcomeFailed, end.Outcome)
	require.Equal(t, "tools", end.ResumeCursor)

	s, loadErr := state.NewFileStore(statePath).Load()
	require.NoError(t, loadErr)
	require.Equal(t, "tools", s.LastCompleted, "cursor must name the last completed step (contract 2)")
}

// Cancel mid-step kills immediately: the session reports Cancelled with
// the interrupted step, the cursor still names the last completed step,
// and a rerun resumes (contract 2, 0064-D5).
func TestSession_cancelMidStep(t *testing.T) {
	dir := t.TempDir()
	statePath := filepath.Join(dir, "state.json")
	f := testFacts()
	deps, _ := stubSessionDeps(t, f)
	deps.NewMise = func() *mise.Mise {
		return &mise.Mise{
			LookPath: func(string) (string, error) { return "/fake/mise", nil },
			RunContext: func(ctx context.Context, _ string, args ...string) (string, error) {
				for _, a := range args {
					if a == "--version" {
						return mise.MinMiseVersion + "\n", nil
					}
				}
				if len(args) > 0 && args[0] == "dotfiles" {
					<-ctx.Done()
					return "", ctx.Err()
				}
				return "", nil
			},
			Install:  func() (string, error) { return "", errors.New("test: unexpected mise install") },
			Classify: func(string) mise.InstallKind { return mise.InstallKindUserManaged },
		}
	}

	sess, err := NewApplyArea(deps).Start(context.Background(), baseOpts(resolveFixture(t), statePath))
	require.NoError(t, err)

	// Cancel once the dotfiles step is announced.
	started := make(chan struct{})
	go func() {
		for ev := range sess.Events() {
			if st, ok := ev.(StepStarted); ok && st.Name == "dotfiles" {
				close(started)
			}
		}
	}()
	<-started
	sess.Cancel()

	res, err := sess.Wait()
	require.Error(t, err)
	var sce *SessionCancelledError
	require.ErrorAs(t, err, &sce)
	require.Equal(t, "dotfiles", sce.StepName)
	require.Equal(t, OutcomeCancelled, res.Outcome)
	require.Equal(t, "tools", res.FinalCursor)

	s, loadErr := state.NewFileStore(statePath).Load()
	require.NoError(t, loadErr)
	require.Equal(t, "tools", s.LastCompleted)
}

// Cancel after the session finished is a no-op.
func TestSession_cancelAfterCompletionNoop(t *testing.T) {
	dir := t.TempDir()
	deps, _ := stubSessionDeps(t, testFacts())
	sess, err := NewApplyArea(deps).Start(context.Background(), baseOpts(resolveFixture(t), filepath.Join(dir, "s.json")))
	require.NoError(t, err)
	_ = drain(t, sess)
	res, err := sess.Wait()
	require.NoError(t, err)
	require.Equal(t, OutcomeCompleted, res.Outcome)
	require.NotPanics(t, func() { sess.Cancel() })
}

// Passthrough mode (Output attached): children stream to the writer, no
// StepOutput events exist, and streaming is not forced (terminals keep
// the child's color, piped writers capture — exactly StreamLive).
func TestSession_outputPolicyPassthrough(t *testing.T) {
	dir := t.TempDir()
	statePath := filepath.Join(dir, "state.json")
	f := testFacts()
	deps, _ := stubSessionDeps(t, f)
	var captured *mise.Mise
	deps.NewMise = func() *mise.Mise {
		m := fakeMise(&[]string{})
		captured = m
		return m
	}
	buf := &bytes.Buffer{}
	opts := baseOpts(resolveFixture(t), statePath)
	opts.Output = buf

	sess, err := NewApplyArea(deps).Start(context.Background(), opts)
	require.NoError(t, err)
	evs := drain(t, sess)
	_, err = sess.Wait()
	require.NoError(t, err)

	for _, ev := range evs {
		_, isOutput := ev.(StepOutput)
		require.False(t, isOutput, "passthrough mode must not emit StepOutput")
	}
	require.NotNil(t, captured)
	require.False(t, captured.ForceStream)
	require.Same(t, buf, captured.Out)
	require.Nil(t, captured.Err,
		"child stderr must keep the process's stderr (parity with the pre-session CLI), not merge into Output")
}

// Event mode (Output absent): streaming is forced to the collector
// without the verbose echo, so children land in the event stream.
func TestSession_outputPolicyEventMode(t *testing.T) {
	dir := t.TempDir()
	statePath := filepath.Join(dir, "state.json")
	deps, _ := stubSessionDeps(t, testFacts())
	var captured *mise.Mise
	deps.NewMise = func() *mise.Mise {
		m := fakeMise(&[]string{})
		captured = m
		return m
	}

	sess, err := NewApplyArea(deps).Start(context.Background(), baseOpts(resolveFixture(t), statePath))
	require.NoError(t, err)
	_ = drain(t, sess)
	_, err = sess.Wait()
	require.NoError(t, err)

	require.NotNil(t, captured)
	require.True(t, captured.ForceStream, "event mode must force streaming to the collector")
	require.False(t, captured.Verbose, "event mode must not carry the verbose echo")
	require.NotNil(t, captured.Out)
	require.Same(t, captured.Out, captured.Err)
}

// The collector line-buffers child output into StepOutput events: one
// event per line, partial lines flush at step end, and the step name is
// the currently running step.
func TestLineCollector_buffersLinesPerStep(t *testing.T) {
	run := &sessionRunner{ch: make(chan Event, 8), opts: ApplyOpts{}}
	run.mu.Lock()
	run.current = "packages"
	run.mu.Unlock()
	c := &lineCollector{run: run}

	n, err := c.Write([]byte("alpha\nbe"))
	require.NoError(t, err)
	require.Equal(t, len("alpha\nbe"), n)
	n, err = c.Write([]byte("ta\n"))
	require.NoError(t, err)
	require.Equal(t, len("ta\n"), n)

	ev1 := (<-run.ch).(StepOutput)
	require.Equal(t, "packages", ev1.Name)
	require.Equal(t, "alpha", string(ev1.Chunk))
	ev2 := (<-run.ch).(StepOutput)
	require.Equal(t, "beta", string(ev2.Chunk))

	// A trailing partial line flushes when the step finishes.
	_, err = c.Write([]byte("partial"))
	require.NoError(t, err)
	c.flush("packages")
	ev3 := (<-run.ch).(StepOutput)
	require.Equal(t, "partial", string(ev3.Chunk))
}

// Backup mode snapshots copy-mode destinations and emits one BackupTaken
// per receiving module directory (base + host winner), before any step
// runs; Wait reports the generation dirs.
func TestSession_backupEmitsEvents(t *testing.T) {
	// Fixture: one base module with a copy entry for a drifted live target,
	// plus a host overlay module with its own copy entry.
	root := t.TempDir()
	write := func(rel, content string) {
		path := filepath.Join(root, rel)
		require.NoError(t, os.MkdirAll(filepath.Dir(path), 0o755))
		require.NoError(t, os.WriteFile(path, []byte(content), 0o644))
	}
	// gone.conf's destination does not exist on this host (fresh-host
	// semantics): backup.Run skips it, so the module's written count is 1
	// while the request lists 2 targets.
	write("modules/demo/module.toml", "[dotfiles]\n\"~/.config/demo/app.conf\" = { source = \"home/app.conf\", mode = \"copy\" }\n\"~/.config/demo/gone.conf\" = { source = \"home/gone.conf\", mode = \"copy\" }\n")
	write("modules/demo/home/app.conf", "profile copy content")
	write("modules/demo/home/gone.conf", "profile gone content")
	write("hosts/myhost/modules/demo/module.toml", "[dotfiles]\n\"~/.config/demo/host.conf\" = { source = \"home/host.conf\", mode = \"copy\" }\n")
	write("hosts/myhost/modules/demo/home/host.conf", "host copy content")

	home := t.TempDir()
	t.Setenv("HOME", home)
	live := filepath.Join(home, ".config", "demo")
	require.NoError(t, os.MkdirAll(live, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(live, "app.conf"), []byte("local edits"), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(live, "host.conf"), []byte("host local edits"), 0o644))

	statePath := filepath.Join(t.TempDir(), "state.json")
	deps, _ := stubSessionDeps(t, testFacts())
	opts := baseOpts(root, statePath)
	opts.Backup = true

	sess, err := NewApplyArea(deps).Start(context.Background(), opts)
	require.NoError(t, err)
	evs := drain(t, sess)
	res, err := sess.Wait()
	require.NoError(t, err)
	require.Equal(t, OutcomeCompleted, res.Outcome)
	require.Len(t, res.Backups, 2)

	var backups []BackupTaken
	for _, ev := range evs {
		if bt, ok := ev.(BackupTaken); ok {
			backups = append(backups, bt)
		}
	}
	require.Len(t, backups, 2)
	byDir := map[string]*BackupTaken{}
	for i := range backups {
		byDir[filepath.Dir(filepath.Dir(backups[i].Dir))] = &backups[i]
	}
	require.Contains(t, byDir, filepath.Join(root, "modules", "demo"))
	// Fresh-host count semantics: the demo module requested two copy
	// destinations but only app.conf existed — Count reports the written
	// truth (the CLI summary line), Files the full request.
	demo := byDir[filepath.Join(root, "modules", "demo")]
	require.Equal(t, 2, len(demo.Files))
	require.Equal(t, 1, demo.Count)
	host := byDir[filepath.Join(root, "hosts", "myhost", "modules", "demo")]
	require.Equal(t, 1, len(host.Files))
	require.Equal(t, 1, host.Count)
	require.Contains(t, byDir, filepath.Join(root, "hosts", "myhost", "modules", "demo"))
	require.FileExists(t, filepath.Join(backups[0].Dir,
		strings.TrimPrefix(filepath.Join(live, "host.conf"), "/"))) // hosts/ sorts first
	require.FileExists(t, filepath.Join(backups[1].Dir,
		strings.TrimPrefix(filepath.Join(live, "app.conf"), "/"))) // absolute target path mirrored under the generation
	require.Equal(t, []string{filepath.Join(live, "app.conf"), filepath.Join(live, "gone.conf")}, demo.Files)
	require.Equal(t, []string{filepath.Join(live, "host.conf")}, host.Files)

	// Symlink and edit entries are never backed up; no dotfiles section, no
	// backups — covered by cmd/apply_backup_test.go's selection matrix.
}

// Preview freezes the per-step TTY classification at Start: stable across
// calls, one entry per step. Real steps declare no terminal need yet —
// the honest classification arrives with real handover surfacing (0071).
func TestSession_previewReportsTTYClassification(t *testing.T) {
	dir := t.TempDir()
	deps, _ := stubSessionDeps(t, testFacts())
	sess, err := NewApplyArea(deps).Start(context.Background(), baseOpts(resolveFixture(t), filepath.Join(dir, "s.json")))
	require.NoError(t, err)

	pv := sess.Preview()
	require.Len(t, pv, 5)
	require.Equal(t, "hooks-pre", pv[0].Name)
	require.Equal(t, "hooks-post", pv[4].Name)
	for _, p := range pv {
		require.False(t, p.NeedsTTY, "no real step surfaces a terminal need until 0071")
		require.Empty(t, p.Reason)
	}
	require.Equal(t, pv, sess.Preview(), "preview must be stable")
}

// The session's handover wrapper enforces the D9 contract: Setpgid on,
// stdio cleared (the consumer wires them), then the consumer's callback
// runs and its result (including a refusal) is returned unchanged.
func TestSession_handoverWrapperEnforcesContract(t *testing.T) {
	var got *exec.Cmd
	refused := errors.New("refused")
	r := &sessionRunner{
		opts: ApplyOpts{Handover: func(cmd *exec.Cmd) error {
			got = cmd
			return refused
		}},
		runCtx: context.Background(),
	}

	cmd := exec.Command("sudo", "-E", "ls", "/tmp")

	err := r.handover(cmd)
	require.Same(t, refused, err)
	require.NotNil(t, got, "the consumer must receive the session-built cmd")
	// The consumer gets a session-ctx twin of the step's command spec:
	// same argv, own process group, group-kill armed on session cancel,
	// stdio unwired (D9: wiring stdio is the consumer's job).
	require.Equal(t, cmd.Path, got.Path)
	require.Equal(t, cmd.Args, got.Args)
	require.Equal(t, cmd.Env, got.Env)
	require.Equal(t, cmd.Dir, got.Dir)
	require.NotNil(t, got.SysProcAttr)
	require.True(t, got.SysProcAttr.Setpgid, "handover cmds run in their own process group (D5)")
	require.NotNil(t, got.Cancel, "session cancel must reach the handover child (D5)")
	require.Nil(t, got.Stdin)
	require.Nil(t, got.Stdout)
	require.Nil(t, got.Stderr)
}

// Cancel reaches a child mid-handover (D5): the consumer runs the cmd
// outside the session's context, so the wrapper's watchdog kills the
// process group when the session dies and the exec returns promptly.
func TestSession_cancelKillsMidHandover(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	r := &sessionRunner{
		opts:   ApplyOpts{Handover: func(cmd *exec.Cmd) error { return cmd.Run() }},
		runCtx: ctx,
	}

	cmd := exec.Command("sleep", "30")
	done := make(chan error, 1)
	start := time.Now()
	go func() { done <- r.handover(cmd) }()
	time.Sleep(150 * time.Millisecond) // let the child start
	cancel()

	select {
	case err := <-done:
		require.Error(t, err, "killed child must surface as a handover error")
	case <-time.After(10 * time.Second):
		t.Fatal("cancel did not reach the handover child — process group survived")
	}
	require.Less(t, time.Since(start), 10*time.Second)
}

// A step that fails on its own merits stays Failed even when the
// consumer's context dies around the same moment: the StepFailed event
// already told the stream the truth, and the outcome must not be
// rewritten to Cancelled.
func TestSession_failureOutracesCancel(t *testing.T) {
	dir := t.TempDir()
	statePath := filepath.Join(dir, "state.json")
	deps, _ := stubSessionDeps(t, testFacts())
	failedStep := make(chan struct{})
	deps.NewMise = func() *mise.Mise {
		m := fakeMise(&[]string{})
		m.Run = func(_ string, args ...string) (string, error) {
			for _, a := range args {
				if a == "--version" {
					return mise.MinMiseVersion + "\n", nil
				}
			}
			if len(args) > 0 && args[0] == "dotfiles" {
				close(failedStep)
				time.Sleep(300 * time.Millisecond) // the cancel races in here
				return "", errors.New("mise exploded")
			}
			return "", nil
		}
		return m
	}

	sess, err := NewApplyArea(deps).Start(context.Background(), baseOpts(resolveFixture(t), statePath))
	require.NoError(t, err)

	go func() {
		for ev := range sess.Events() {
			if _, ok := ev.(StepFailed); ok {
				sess.Cancel() // cancels while the step is still failing
				return
			}
		}
	}()

	res, err := sess.Wait()
	require.NoError(t, err, "a failed step is a result, not a Go error")
	require.Equal(t, OutcomeFailed, res.Outcome, "a genuine failure must not be rewritten to Cancelled")
	require.NotNil(t, res.StepError)
	require.Equal(t, "dotfiles", res.StepError.Step)
	require.Equal(t, "tools", res.FinalCursor)
}

// Hook steps surface per-command sub-step boundaries (0071): each hook
// command's start is observable as a StepStarted carrying Sub{Index, Total,
// Command} mirroring the resolved plan, alongside the step-level start.
func TestSession_hookSubStepEvents(t *testing.T) {
	dir := t.TempDir()
	deps, _ := stubSessionDeps(t, testFacts())
	sess, err := NewApplyArea(deps).Start(context.Background(), baseOpts(resolveFixture(t), filepath.Join(dir, "s.json")))
	require.NoError(t, err)
	evs := drain(t, sess)
	_, err = sess.Wait()
	require.NoError(t, err)

	pr := evs[0].(PlanResolved)
	wantPre := pr.Plan.Hooks.Pre
	wantPost := pr.Plan.Hooks.Post
	require.NotEmpty(t, wantPre, "fixture must declare pre hooks")

	type subStart struct {
		step string
		sub  *SubStep
	}
	var subs []subStart
	for _, ev := range evs {
		if st, ok := ev.(StepStarted); ok && st.Sub != nil {
			subs = append(subs, subStart{st.Name, st.Sub})
		}
	}

	// Step-level starts stay one per step (Sub nil); sub starts ride the
	// hooks steps only.
	require.Equal(t, []string{"hooks-pre", "packages", "tools", "dotfiles", "hooks-post"}, stepNames(evs))
	require.Len(t, subs, len(wantPre)+len(wantPost))

	var gotPre, gotPost []SubStep
	for _, s := range subs {
		require.Equal(t, len(wantPre), s.sub.Total, "hooks-pre total")
		switch s.step {
		case "hooks-pre":
			gotPre = append(gotPre, *s.sub)
		case "hooks-post":
			gotPost = append(gotPost, *s.sub)
		default:
			t.Fatalf("sub-step start on non-hook step %s", s.step)
		}
	}
	require.Len(t, gotPre, len(wantPre))
	for i, w := range wantPre {
		require.Equal(t, i, gotPre[i].Index, "hook index follows plan order")
		require.Equal(t, w.Command, gotPre[i].Command)
	}
	require.Len(t, gotPost, len(wantPost))
	for i, w := range wantPost {
		require.Equal(t, i, gotPost[i].Index)
		require.Equal(t, w.Command, gotPost[i].Command)
	}
}

// A failing required hook command is observable before the step-level
// failure: a Sub-carrying StepFailed names the command, then the step
// fails resumably (cursor untouched, session outcome Failed).
func TestSession_hookSubStepFailure(t *testing.T) {
	dir := t.TempDir()
	statePath := filepath.Join(dir, "state.json")
	deps, _ := stubSessionDeps(t, testFacts())
	events := &[]string{}
	deps.NewMise = func() *mise.Mise {
		m := fakeMise(events)
		inner := m.Run
		m.Run = func(name string, args ...string) (string, error) {
			for _, a := range args {
				if a == "hooks-pre-0" {
					return "", errors.New("hook boom")
				}
			}
			return inner(name, args...)
		}
		return m
	}

	sess, err := NewApplyArea(deps).Start(context.Background(), baseOpts(resolveFixture(t), statePath))
	require.NoError(t, err)
	evs := drain(t, sess)
	res, err := sess.Wait()
	require.NoError(t, err)

	pr := evs[0].(PlanResolved)
	require.Equal(t, OutcomeFailed, res.Outcome)
	require.Equal(t, "", res.FinalCursor, "nothing completed before hooks-pre")

	var subFails, stepFails []StepFailed
	for _, ev := range evs {
		if f, ok := ev.(StepFailed); ok {
			if f.Sub != nil {
				subFails = append(subFails, f)
			} else {
				stepFails = append(stepFails, f)
			}
		}
	}
	require.Len(t, subFails, 1, "the failing hook command is announced")
	require.Equal(t, "hooks-pre", subFails[0].Name)
	require.Equal(t, 0, subFails[0].Sub.Index)
	require.Equal(t, pr.Plan.Hooks.Pre[0].Command, subFails[0].Sub.Command)
	require.Contains(t, subFails[0].Err.Error(), pr.Plan.Hooks.Pre[0].Command)
	require.Len(t, stepFails, 1, "the step-level failure follows, Sub nil")
	require.Equal(t, "hooks-pre", stepFails[0].Name)
}

// With handover available, hook steps classify as needing the terminal
// (0064-D4: the interactive-hook opt-in) and their commands run through
// the Handover seam instead of the piped mise runner (0071).
func TestSession_interactiveHooksHandover(t *testing.T) {
	dir := t.TempDir()
	deps, events := stubSessionDeps(t, testFacts())

	var handed []*exec.Cmd
	opts := baseOpts(resolveFixture(t), filepath.Join(dir, "s.json"))
	opts.HandoverAvailable = ptr(true)
	opts.Handover = func(cmd *exec.Cmd) error {
		handed = append(handed, cmd)
		return nil
	}

	sess, err := NewApplyArea(deps).Start(context.Background(), opts)
	require.NoError(t, err)
	pv := sess.Preview()
	evs := drain(t, sess)
	_, err = sess.Wait()
	require.NoError(t, err)

	// Classification names the real cause, only for the hook steps.
	byName := map[string]StepPreview{}
	for _, p := range pv {
		byName[p.Name] = p
	}
	require.True(t, byName["hooks-pre"].NeedsTTY)
	require.Contains(t, byName["hooks-pre"].Reason, "interactive hook")
	require.True(t, byName["hooks-post"].NeedsTTY)
	require.False(t, byName["packages"].NeedsTTY)
	require.False(t, byName["tools"].NeedsTTY)
	require.False(t, byName["dotfiles"].NeedsTTY)

	// Every hook command went through handover as `mise run <task>`, in
	// plan order (pre then post), never through the piped runner.
	pr := evs[0].(PlanResolved)
	require.Len(t, handed, len(pr.Plan.Hooks.Pre)+len(pr.Plan.Hooks.Post))
	taskAt := func(i int) string {
		if i < len(pr.Plan.Hooks.Pre) {
			return fmt.Sprintf("hooks-pre-%d", i)
		}
		return fmt.Sprintf("hooks-post-%d", i-len(pr.Plan.Hooks.Pre))
	}
	for i, cmd := range handed {
		require.Equal(t, "/fake/mise", cmd.Path)
		require.Equal(t, []string{"/fake/mise", "run", "--cd", cmd.Args[3], taskAt(i)}, cmd.Args)
		require.True(t, hasEnv(cmd.Env, "MISE_TRUSTED_CONFIG_PATHS"), "trust plumbing rides the handover child")
	}
	for _, e := range *events {
		require.NotContains(t, e, "hooks-pre-0", "hook tasks must not run through the piped runner")
	}
	// Sub-step boundaries still announced.
	subs := 0
	for _, ev := range evs {
		if st, ok := ev.(StepStarted); ok && st.Sub != nil {
			subs++
		}
	}
	require.Equal(t, len(pr.Plan.Hooks.Pre)+len(pr.Plan.Hooks.Post), subs)
	require.Equal(t, OutcomeCompleted, func() SessionOutcome { r, _ := sess.Wait(); return r.Outcome }())
}

func ptr[T any](v T) *T { return &v }

// hasEnv reports whether the environment carries the key.
func hasEnv(env []string, key string) bool {
	for _, e := range env {
		if strings.HasPrefix(e, key+"=") {
			return true
		}
	}
	return false
}

// System-scope edit entries whose targets the invoking user cannot write
// converge through one elevated `sudo -E mise dotfiles apply` child —
// surfaced through the handover seam (contract 13, issue 0071): the step
// classifies NeedsTTY with the sudo reason and hands the real child over.
func TestSession_sudoEditsHandover(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("running as root: every target is writable, no elevation decision exists")
	}
	dir := t.TempDir()
	statePath := filepath.Join(dir, "state.json")

	// Profile: one system-scope module with an edit entry under /etc —
	// the walk-up writability probe hits /etc (not user-writable).
	root := filepath.Join(dir, "profile")
	require.NoError(t, os.MkdirAll(filepath.Join(root, "modules", "sysedit"), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(root, "dotdrift.toml"), []byte("[modules]\ndisable = []\n"), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(root, "modules", "sysedit", "module.toml"), []byte(
		"id = \"sysedit\"\napp = \"sysedit\"\nscope = \"system\"\n\n[dotfiles]\n\"/etc/dotdrift-0071-test/x.conf/anchor\" = { line = \"x\" }\n",
	), 0o644))

	deps, events := stubSessionDeps(t, testFacts())

	var handed []*exec.Cmd
	opts := baseOpts(root, statePath)
	opts.Sections = map[string]bool{"dotfiles": true}
	opts.Handover = func(cmd *exec.Cmd) error {
		handed = append(handed, cmd)
		return nil
	}

	sess, err := NewApplyArea(deps).Start(context.Background(), opts)
	require.NoError(t, err)
	pv := sess.Preview()
	drain(t, sess)
	res, err := sess.Wait()
	require.NoError(t, err)

	byName := map[string]StepPreview{}
	for _, p := range pv {
		byName[p.Name] = p
	}
	require.True(t, byName["dotfiles-system"].NeedsTTY, "sudo edits need the terminal")
	require.Contains(t, byName["dotfiles-system"].Reason, "sudo")
	require.False(t, byName["dotfiles"].NeedsTTY, "the user dotfiles step has no terminal need")

	// The elevated apply went through handover as one sudo -E child.
	require.Len(t, handed, 1)
	cmd := handed[0]
	require.True(t, strings.HasSuffix(cmd.Path, "/sudo"), "sudo resolves on PATH, got %q", cmd.Path)
	require.Equal(t, []string{"sudo", "-E", "/fake/mise", "dotfiles", "apply",
		"--cd", filepath.Join(dir, "mise", "system-edits"), "--yes"}, cmd.Args)
	require.True(t, hasEnv(cmd.Env, "MISE_TRUSTED_CONFIG_PATHS"), "trust plumbing survives the elevation")
	for _, e := range *events {
		require.NotContains(t, e, "dotfiles apply", "the elevated apply must not run through the piped runner")
	}
	require.Equal(t, OutcomeCompleted, res.Outcome)
}
