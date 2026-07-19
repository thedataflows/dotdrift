package dotdrift

import (
	"bytes"
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/rs/zerolog/log"
	"github.com/stretchr/testify/require"
	"github.com/thedataflows/dotdrift/internal/facts"
	"github.com/thedataflows/dotdrift/internal/mise"
	"github.com/thedataflows/dotdrift/internal/packages"
	"github.com/thedataflows/dotdrift/internal/profile"
	"github.com/thedataflows/dotdrift/internal/resolve"
	"github.com/thedataflows/dotdrift/internal/state"
)

// recordingBackend is a packages.Backend fake that records calls and can be
// made to fail the install.
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

// fakeMise returns a mise bootstrapper that never touches the OS: the binary
// is always found and reports the minimum required version.
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

// stubApplyDeps swaps the package-level seams in apply.go for fakes and
// returns the shared event log plus the recording packages backend.
// profile.Load and resolve.Resolve run for real (wrapped to record order) so
// the wiring is exercised end-to-end against a fixture profile.
func stubApplyDeps(t *testing.T, f *facts.Facts) (*[]string, *recordingBackend) {
	t.Helper()
	events := &[]string{}
	backend := &recordingBackend{events: events}

	origDetect, origLoad, origResolve, origMise, origFor := detectFacts, profileLoad, resolvePlan, defaultMise, packagesFor
	t.Cleanup(func() {
		detectFacts, profileLoad, resolvePlan, defaultMise, packagesFor = origDetect, origLoad, origResolve, origMise, origFor
	})

	detectFacts = func() (*facts.Facts, error) { return f, nil }
	profileLoad = func(root string, ff *facts.Facts) (*profile.Profile, error) {
		*events = append(*events, "load")
		return origLoad(root, ff)
	}
	resolvePlan = func(p *profile.Profile, ff *facts.Facts) (*resolve.Plan, error) {
		*events = append(*events, "resolve")
		return origResolve(p, ff)
	}
	defaultMise = func() *mise.Mise { return fakeMise(events) }
	packagesFor = func(string) packages.Backend { return backend }
	return events, backend
}

// requireOrder asserts that the given event prefixes appear in order.
func requireOrder(t *testing.T, events []string, want ...string) {
	t.Helper()
	at := 0
	for _, w := range want {
		found := -1
		for i := at; i < len(events); i++ {
			if strings.HasPrefix(events[i], w) {
				found = i
				break
			}
		}
		require.GreaterOrEqual(t, found, 0, "event %q missing after position %d in %v", w, at, events)
		at = found + 1
	}
}

func loadStateFile(t *testing.T, path string) *state.State {
	t.Helper()
	s, err := state.NewFileStore(path).Load()
	require.NoError(t, err)
	return s
}

func fingerprintFor(t *testing.T, profileDir string, f *facts.Facts) string {
	t.Helper()
	p, err := profile.Load(profileDir, f)
	require.NoError(t, err)
	return resolve.Fingerprint(p, f)
}

func resolveFixture(t *testing.T) string {
	t.Helper()
	return filepath.Join("..", "testdata", "profiles", "resolve")
}

// Happy path: detect → load → resolve → ensure → packages/tools/dotfiles
// in order, ending in a complete state with the current selection fingerprint.
func TestApply_happyPath(t *testing.T) {
	dir := t.TempDir()
	statePath := filepath.Join(dir, "state.json")
	f := &facts.Facts{Hostname: "myhost", Username: "cri", OS: "linux", Backend: "paru"}
	events, _ := stubApplyDeps(t, f)

	cmd := &ApplyCmd{Profile: resolveFixture(t), State: statePath, Yes: true}
	require.NoError(t, cmd.Run())

	requireOrder(t, *events,
		"load",
		"resolve",
		"mise:ensure",
		"packages:absent",
		"packages:present",
		"mise:run install",
		"mise:run dotfiles apply",
	)
	require.Contains(t, *events, "packages:present eza,fd,neovim,ripgrep")
	require.Contains(t, *events, "packages:absent emacs,nano")
	require.Contains(t, *events, "mise:run dotfiles apply --cd "+filepath.Join(dir, "mise")+" --yes")

	s := loadStateFile(t, statePath)
	require.Equal(t, state.StatusComplete, s.Status)
	require.Empty(t, s.Current)
	require.Empty(t, s.Error)
	require.Equal(t, fingerprintFor(t, resolveFixture(t), f), s.Selection)
	for _, step := range []string{"packages", "tools", "dotfiles"} {
		require.True(t, s.IsCompleted(step), "step %s not completed", step)
	}
}

// When the persisted selection fingerprint differs from the current one, apply
// warns and resets the resume state, so previously completed steps re-run.
func TestApply_selectionChangeResetsStateAndWarns(t *testing.T) {
	dir := t.TempDir()
	statePath := filepath.Join(dir, "state.json")
	f := &facts.Facts{Hostname: "myhost", Username: "cri", OS: "linux", Backend: "paru"}
	events, _ := stubApplyDeps(t, f)

	stale := state.New()
	stale.Selection = "stale-fingerprint"
	stale.Completed["packages"] = true
	stale.Current = "packages"
	stale.Status = state.StatusFailed
	stale.Error = "boom"
	require.NoError(t, state.NewFileStore(statePath).Save(stale))

	var logBuf bytes.Buffer
	origLogger := log.Logger
	log.Logger = log.Output(&logBuf)
	t.Cleanup(func() { log.Logger = origLogger })

	cmd := &ApplyCmd{Profile: resolveFixture(t), State: statePath}
	require.NoError(t, cmd.Run())

	require.Contains(t, logBuf.String(), "selection changed")
	require.Contains(t, logBuf.String(), "reset")
	require.Contains(t, *events, "packages:present eza,fd,neovim,ripgrep",
		"packages step must re-run after the selection reset cleared completed steps")

	s := loadStateFile(t, statePath)
	require.Equal(t, state.StatusComplete, s.Status)
	require.Equal(t, fingerprintFor(t, resolveFixture(t), f), s.Selection)
	for _, step := range []string{"packages", "tools", "dotfiles"} {
		require.True(t, s.IsCompleted(step), "step %s not completed", step)
	}
}

// Apply prints the effective resolved plan (same rendering as `dotdrift plan`)
// to its Out writer before the pipeline runs.
func TestApply_printsPlan(t *testing.T) {
	dir := t.TempDir()
	statePath := filepath.Join(dir, "state.json")
	f := &facts.Facts{Hostname: "myhost", Username: "cri", OS: "linux", Backend: "paru"}
	stubApplyDeps(t, f)

	var buf bytes.Buffer
	cmd := &ApplyCmd{Profile: resolveFixture(t), State: statePath, Yes: true, Out: &buf}
	require.NoError(t, cmd.Run())

	out := buf.String()
	t.Log(out)
	require.Contains(t, out, "fingerprint:")
	require.Contains(t, out, "packages:")
	require.Contains(t, out, "  - ripgrep")
	require.Contains(t, out, "remove:")
	require.Contains(t, out, "  - emacs")
	require.Contains(t, out, "tools:")
	require.Contains(t, out, "  node: 22")
	require.Contains(t, out, "dotfiles:")
	require.Contains(t, out, "~/.bashrc")

	s := loadStateFile(t, statePath)
	require.Equal(t, state.StatusComplete, s.Status)
}

// Decision D8a (keep + test): apply writes the FULL mise config (tools +
// dotfiles) before the pipeline starts. When apply fails at the first
// pipeline step, the crash snapshot on disk still contains that full config
// and the state file records the failed step.
func TestApply_crashSnapshotKeepsFullMiseConfig(t *testing.T) {
	dir := t.TempDir()
	statePath := filepath.Join(dir, "state.json")
	f := &facts.Facts{Hostname: "myhost", Username: "cri", OS: "linux", Backend: "paru"}
	events, backend := stubApplyDeps(t, f)
	backend.presentErr = errors.New("paru boom")

	cmd := &ApplyCmd{Profile: resolveFixture(t), State: statePath}
	err := cmd.Run()
	require.Error(t, err)
	require.Contains(t, err.Error(), "packages")

	s := loadStateFile(t, statePath)
	require.Equal(t, "packages", s.Current)
	require.Equal(t, state.StatusFailed, s.Status)
	require.Contains(t, s.Error, "boom")
	require.False(t, s.IsCompleted("packages"))
	require.Equal(t, fingerprintFor(t, resolveFixture(t), f), s.Selection)

	configPath := filepath.Join(dir, "mise", "mise.toml")
	data, err := os.ReadFile(configPath)
	require.NoError(t, err, "pre-pipeline full mise config must exist after a crash")
	cfg := string(data)
	require.Contains(t, cfg, "[tools]")
	require.Contains(t, cfg, `node = "22"`)
	require.Contains(t, cfg, "[dotfiles]")
	require.Contains(t, cfg, "~/.bashrc")

	for _, e := range *events {
		require.NotContains(t, e, "mise:run install", "tools step must not run after the packages failure")
	}
}
