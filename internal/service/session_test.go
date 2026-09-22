package service

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/thedataflows/dotdrift/internal/facts"
	"github.com/thedataflows/dotdrift/internal/mise"
	"github.com/thedataflows/dotdrift/internal/packages"
	"github.com/thedataflows/dotdrift/internal/profile"
	"github.com/thedataflows/dotdrift/internal/resolve"
	"github.com/thedataflows/dotdrift/internal/smb"
)

// recordingBackend is a packages.Backend fake that records calls (same
// shape as the cmd-level fake: the session drives the same backends).
type recordingBackend struct {
	events     *[]string
	presentErr error
}

var _ packages.Backend = (*recordingBackend)(nil)

func (b *recordingBackend) Present(_ context.Context, pkgs []string) error {
	*b.events = append(*b.events, "packages:present "+strings.Join(pkgs, ","))
	return b.presentErr
}

func (b *recordingBackend) Absent(_ context.Context, pkgs []string) error {
	*b.events = append(*b.events, "packages:absent "+strings.Join(pkgs, ","))
	return nil
}

func (b *recordingBackend) IsInstalled(context.Context, string) (bool, error) { return false, nil }
func (b *recordingBackend) Installed(context.Context) ([]string, error)       { return nil, nil }
func (b *recordingBackend) DirectDeps(context.Context, string) ([]string, error) {
	return nil, nil
}

// fakeMise is a mise bootstrapper that never touches the OS.
func fakeMise(events *[]string) *mise.Mise {
	return &mise.Mise{
		LookPath: func(string) (string, error) {
			*events = append(*events, "mise:ensure")
			return "/fake/mise", nil
		},
		Run: func(_ string, args ...string) (string, error) {
			*events = append(*events, "mise:run "+strings.Join(args, " "))
			for _, a := range args {
				if a == "--version" {
					return mise.MinMiseVersion + "\n", nil
				}
			}
			return "", nil
		},
		Install:  func() (string, error) { return "", errors.New("test: unexpected mise install") },
		Classify: func(string) mise.InstallKind { return mise.InstallKindUserManaged },
	}
}

// recordingSmbRunner records smb commands without touching the OS.
type recordingSmbRunner struct{ events *[]string }

func (r *recordingSmbRunner) Run(_ context.Context, name string, args ...string) (string, error) {
	*r.events = append(*r.events, "smb:run "+name+" "+strings.Join(args, " "))
	return "", nil
}

func (r *recordingSmbRunner) RunInteractive(_ context.Context, name string, args ...string) error {
	*r.events = append(*r.events, "smb:run-interactive "+name+" "+strings.Join(args, " "))
	return nil
}

var _ smb.Runner = (*recordingSmbRunner)(nil)

// stubSessionDeps returns real load/resolve wiring (same end-to-end stance
// as the cmd apply tests) with every OS-writing seam faked, contained to
// temp dirs.
func stubSessionDeps(t *testing.T, f *facts.Facts) (ApplyDeps, *[]string) {
	t.Helper()
	t.Setenv("XDG_DATA_HOME", t.TempDir())
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	events := &[]string{}
	backend := &recordingBackend{events: events}
	return ApplyDeps{
		Detect:      func() (*facts.Facts, error) { return f, nil },
		LoadProfile: profile.Load,
		Resolve:     resolve.Resolve,
		NewMise:     func() *mise.Mise { return fakeMise(events) },
		PackagesFor: func(string) packages.Backend { return backend },
		NewSmbRunner: func() smb.Runner {
			return &recordingSmbRunner{events: events}
		},
	}, events
}

func resolveFixture(t *testing.T) string {
	t.Helper()
	return filepath.Join("..", "..", "testdata", "profiles", "resolve")
}

// stepNames returns the step-level start order (Sub nil — hook sub-step
// starts carry a Sub and are asserted by the hook-specific tests).
func stepNames(evs []Event) []string {
	var names []string
	for _, ev := range evs {
		if st, ok := ev.(StepStarted); ok && st.Sub == nil {
			names = append(names, st.Name)
		}
	}
	return names
}

// drain collects every event until the session closes its stream.
func drain(t *testing.T, s *ApplySession) []Event {
	t.Helper()
	var evs []Event
	for ev := range s.Events() {
		evs = append(evs, ev)
	}
	return evs
}

// A started session runs the whole absorbed pipeline: PlanResolved (with
// cursor state) opens the stream, steps fire Started/Finished in pipeline
// order, SessionEnded{Completed} closes it, the state file is removed
// (contract 2), and Wait reports Completed with no final cursor.
func TestSession_lifecycleCompleted(t *testing.T) {
	dir := t.TempDir()
	statePath := filepath.Join(dir, "state.json")
	f := &facts.Facts{Hostname: "myhost", Username: "cri", OS: "linux", Backend: "paru"}
	deps, events := stubSessionDeps(t, f)

	sess, err := NewApplyArea(deps).Start(context.Background(), ApplyOpts{
		ProfilePath: resolveFixture(t),
		StatePath:   statePath,
		Yes:         true,
		Handover:    func(*exec.Cmd) error { return nil },
	})
	require.NoError(t, err)

	evs := drain(t, sess)
	res, err := sess.Wait()
	require.NoError(t, err)
	require.Equal(t, OutcomeCompleted, res.Outcome)
	require.Empty(t, res.FinalCursor)
	require.Nil(t, res.StepError)

	// Stream shape: PlanResolved first, SessionEnded last, nothing after.
	require.NotEmpty(t, evs)
	pr, ok := evs[0].(PlanResolved)
	require.True(t, ok, "first event must be PlanResolved, got %T", evs[0])
	require.NotNil(t, pr.Plan)
	require.NotNil(t, pr.Profile)
	require.NotNil(t, pr.Facts)
	require.Empty(t, pr.Cursor)
	require.False(t, pr.CursorEffective)
	end, ok := evs[len(evs)-1].(SessionEnded)
	require.True(t, ok, "last event must be SessionEnded, got %T", evs[len(evs)-1])
	require.Equal(t, OutcomeCompleted, end.Outcome)
	require.Empty(t, end.ResumeCursor)

	// Pipeline order for the resolve fixture: hooks-pre, packages, tools,
	// dotfiles, hooks-post (the same steps the cmd happy path exercises).
	started := stepNames(evs)
	var sawOutput bool
	for _, ev := range evs {
		switch e := ev.(type) {
		case StepOutput:
			sawOutput = true
		case BackupTaken:
			t.Fatalf("unexpected BackupTaken without the backup opt: %+v", e)
		}
	}
	require.Equal(t, []string{"hooks-pre", "packages", "tools", "dotfiles", "hooks-post"}, started)
	require.False(t, sawOutput, "event mode emits StepOutput only from real child output")

	// The pipeline actually ran through the fake seams.
	found := false
	for _, e := range *events {
		if strings.HasPrefix(e, "mise:run dotfiles apply") {
			found = true
		}
	}
	require.True(t, found, "dotfiles step must run through the mise runner")

	_, statErr := os.Stat(statePath)
	require.True(t, os.IsNotExist(statErr), "state file must be removed after a successful apply")
}
