package cmd

import (
	"bytes"
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/thedataflows/dotdrift/internal/facts"
	"github.com/thedataflows/dotdrift/internal/mise"
	"github.com/thedataflows/dotdrift/internal/packages"
	"github.com/thedataflows/dotdrift/internal/state"
)

// allInstalledBackend reports every package installed; used for clean-drift
// status tests where only the packages section is exercised.
type allInstalledBackend struct{}

var _ packages.Backend = allInstalledBackend{}

func (allInstalledBackend) Present(context.Context, []string) error              { return nil }
func (allInstalledBackend) Absent(context.Context, []string) error               { return nil }
func (allInstalledBackend) IsInstalled(context.Context, string) (bool, error)    { return true, nil }
func (allInstalledBackend) DirectDeps(context.Context, string) ([]string, error) { return nil, nil }

// statusMinimalProfile writes a temp profile with one module that installs a
// single package and declares no tools/dotfiles/mounts/smb, so drift can be
// driven entirely through the packages backend.
func statusMinimalProfile(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	modDir := filepath.Join(dir, "modules", "demo")
	require.NoError(t, os.MkdirAll(modDir, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(modDir, "module.toml"), []byte(`id = "demo"
app = "demo"

[packages]
present = ["demo-pkg"]
`), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "dotdrift.toml"), []byte("[modules]\ndisable = []\n"), 0o644))
	return dir
}

// stubStatusDeps swaps the detection/mise/package seams and returns the facts
// to use. profile.Load and resolve.Resolve run for real against the profile.
func stubStatusDeps(t *testing.T, f *facts.Facts, backend packages.Backend, miseFactory func() *mise.Mise) {
	t.Helper()
	origDetect, origMise, origFor := detectFacts, defaultMise, packagesFor
	t.Cleanup(func() {
		detectFacts, defaultMise, packagesFor = origDetect, origMise, origFor
	})
	detectFacts = func() (*facts.Facts, error) { return f, nil }
	defaultMise = miseFactory
	packagesFor = func(string) packages.Backend { return backend }
}

func fakeMiseNoOp() *mise.Mise {
	return &mise.Mise{
		LookPath: func(string) (string, error) { return "/fake/mise", nil },
		Run:      func(string, ...string) (string, error) { return "", nil },
	}
}

func TestStatus_cleanNoDrift(t *testing.T) {
	f := &facts.Facts{Hostname: "myhost", Username: "cri", OS: "linux", Backend: "paru"}
	stubStatusDeps(t, f, allInstalledBackend{}, fakeMiseNoOp)
	profile := statusMinimalProfile(t)

	var buf bytes.Buffer
	require.NoError(t, (&StatusCmd{Profile: profile, State: filepath.Join(t.TempDir(), "state.json"), out: &buf}).Run())

	out := buf.String()
	t.Log(out)
	require.Contains(t, out, "resume: clean — next apply starts from the beginning")
	require.Contains(t, out, "ok: all 1 checks passed")
	require.True(t, strings.HasSuffix(strings.TrimSpace(out), "no drift"), "final line must be 'no drift'")
}

func TestStatus_showsCursor(t *testing.T) {
	f := &facts.Facts{Hostname: "myhost", Username: "cri", OS: "linux", Backend: "paru"}
	stubStatusDeps(t, f, allInstalledBackend{}, fakeMiseNoOp)
	profile := statusMinimalProfile(t)

	statePath := filepath.Join(t.TempDir(), "state.json")
	require.NoError(t, state.NewFileStore(statePath).Save(&state.State{LastCompleted: "packages"}))

	var buf bytes.Buffer
	require.NoError(t, (&StatusCmd{Profile: profile, State: statePath, out: &buf}).Run())
	require.Contains(t, buf.String(), `resume: last completed "packages" — next apply resumes after it`)
}

func TestStatus_reportsDrift(t *testing.T) {
	f := &facts.Facts{Hostname: "myhost", Username: "cri", OS: "linux", Backend: "paru"}
	// recordingBackend.IsInstalled reports everything absent → drift.
	stubStatusDeps(t, f, &recordingBackend{events: &[]string{}}, fakeMiseNoOp)
	profile := statusMinimalProfile(t)

	var buf bytes.Buffer
	err := (&StatusCmd{Profile: profile, State: filepath.Join(t.TempDir(), "state.json"), out: &buf}).Run()
	require.NoError(t, err, "status must exit 0 even when drift is found")
	out := buf.String()
	t.Log(out)
	require.Contains(t, out, "demo: demo-pkg — missing")
	require.Contains(t, out, "drift: 1 item")
}

func TestStatus_miseMissingToolsUnknown(t *testing.T) {
	f := &facts.Facts{Hostname: "myhost", Username: "cri", OS: "linux", Backend: "paru"}
	// A mise whose LookPath fails → every tool reports unknown.
	noMise := func() *mise.Mise {
		return &mise.Mise{LookPath: func(string) (string, error) { return "", errors.New("no mise on PATH") }}
	}
	stubStatusDeps(t, f, allInstalledBackend{}, noMise)

	// Profile with a tool declaration so the tools section is exercised.
	dir := t.TempDir()
	modDir := filepath.Join(dir, "modules", "demo")
	require.NoError(t, os.MkdirAll(modDir, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(modDir, "module.toml"), []byte(`id = "demo"
app = "demo"

[tools]
node = "20"
`), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "dotdrift.toml"), []byte("[modules]\ndisable = []\n"), 0o644))

	var buf bytes.Buffer
	require.NoError(t, (&StatusCmd{Profile: dir, State: filepath.Join(t.TempDir(), "state.json"), out: &buf}).Run())
	out := buf.String()
	t.Log(out)
	require.Contains(t, out, "demo: node")
	require.Contains(t, out, "unknown")
}

func TestStatus_defaultsToProfileStatePath(t *testing.T) {
	f := &facts.Facts{Hostname: "myhost", Username: "cri", OS: "linux", Backend: "paru"}
	stubStatusDeps(t, f, allInstalledBackend{}, fakeMiseNoOp)
	profile := statusMinimalProfile(t)

	var buf bytes.Buffer
	require.NoError(t, (&StatusCmd{Profile: profile, out: &buf}).Run())
	require.Contains(t, buf.String(), "state: "+state.ProfileStatePath(profile))
}

func TestStatus_verboseShowsProgress(t *testing.T) {
	f := &facts.Facts{Hostname: "myhost", Username: "cri", OS: "linux", Backend: "paru"}
	stubStatusDeps(t, f, allInstalledBackend{}, fakeMiseNoOp)
	profile := statusMinimalProfile(t)

	var buf, errBuf bytes.Buffer
	require.NoError(t, (&StatusCmd{
		Profile: profile,
		State:   filepath.Join(t.TempDir(), "state.json"),
		Verbose: true,
		out:     &buf,
		err:     &errBuf,
	}).Run())

	// Progress lines reach stderr; stdout report stays clean.
	require.Contains(t, errBuf.String(), "checking packages: demo-pkg")
	require.NotContains(t, buf.String(), "checking")
}

func TestStatus_jobsFlagAccepted(t *testing.T) {
	f := &facts.Facts{Hostname: "myhost", Username: "cri", OS: "linux", Backend: "paru"}
	stubStatusDeps(t, f, allInstalledBackend{}, fakeMiseNoOp)
	profile := statusMinimalProfile(t)

	statePath := filepath.Join(t.TempDir(), "state.json")
	var j1, j0 bytes.Buffer
	require.NoError(t, (&StatusCmd{Profile: profile, State: statePath, Jobs: 1, out: &j1}).Run())
	require.NoError(t, (&StatusCmd{Profile: profile, State: statePath, Jobs: 0, out: &j0}).Run())
	require.Equal(t, j0.String(), j1.String())
}

func TestStatus_moduleFilterLimitsScope(t *testing.T) {
	f := &facts.Facts{Hostname: "myhost", Username: "cri", OS: "linux", Backend: "paru"}
	stubStatusDeps(t, f, &recordingBackend{events: &[]string{}}, fakeMiseNoOp)

	// Two-module profile: "alpha" installs alpha-pkg, "beta" installs beta-pkg.
	dir := t.TempDir()
	for _, mod := range []struct{ dir, pkg string }{
		{"alpha", "alpha-pkg"},
		{"beta", "beta-pkg"},
	} {
		modDir := filepath.Join(dir, "modules", mod.dir)
		require.NoError(t, os.MkdirAll(modDir, 0o755))
		require.NoError(t, os.WriteFile(filepath.Join(modDir, "module.toml"), []byte(`id = "`+mod.dir+`"
app = "`+mod.dir+`"

[packages]
present = ["`+mod.pkg+`"]
`), 0o644))
	}
	require.NoError(t, os.WriteFile(filepath.Join(dir, "dotdrift.toml"), []byte("[modules]\ndisable = []\n"), 0o644))

	statePath := filepath.Join(t.TempDir(), "state.json")

	// No filter: both packages drift.
	var all bytes.Buffer
	require.NoError(t, (&StatusCmd{Profile: dir, State: statePath, out: &all}).Run())
	require.Contains(t, all.String(), "alpha: alpha-pkg — missing")
	require.Contains(t, all.String(), "beta: beta-pkg — missing")

	// Filter to "alpha" only: alpha-pkg drifts, beta-pkg absent from output.
	var filtered bytes.Buffer
	require.NoError(t, (&StatusCmd{Profile: dir, State: statePath, Modules: []string{"alpha"}, out: &filtered}).Run())
	require.Contains(t, filtered.String(), "alpha: alpha-pkg — missing")
	require.NotContains(t, filtered.String(), "beta")
}

func TestStatus_diffFlagShowsDiff(t *testing.T) {
	f := &facts.Facts{Hostname: "myhost", Username: "cri", OS: "linux", Backend: "paru"}
	stubStatusDeps(t, f, allInstalledBackend{}, fakeMiseNoOp)

	// Profile with a copy-mode dotfile whose target has different content
	dir := t.TempDir()
	target := filepath.Join(dir, "live-target")
	require.NoError(t, os.WriteFile(target, []byte("theme = \"light\"\n"), 0o644))
	modDir := filepath.Join(dir, "modules", "demo")
	require.NoError(t, os.MkdirAll(filepath.Join(modDir, "files"), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(modDir, "files", "config"), []byte("theme = \"dark\"\n"), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(modDir, "module.toml"), []byte(`id = "demo"
app = "demo"

[dotfiles]
"`+target+`" = { source = "files/config", mode = "copy" }
`), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "dotdrift.toml"), []byte("[modules]\ndisable = []\n"), 0o644))

	var buf bytes.Buffer
	require.NoError(t, (&StatusCmd{Profile: dir, State: filepath.Join(t.TempDir(), "state.json"), Diff: "internal", out: &buf}).Run())

	out := buf.String()
	t.Log(out)
	// Drift report present
	require.Contains(t, out, "content differs")
	// Diff output present after the report
	require.Contains(t, out, "[demo]")
	require.Contains(t, out, "---")
	require.Contains(t, out, "+++")
	require.Contains(t, out, "-theme = \"light\"")
	require.Contains(t, out, "+theme = \"dark\"")
}

func TestStatus_diffFlagToolNotFoundErrors(t *testing.T) {
	f := &facts.Facts{Hostname: "myhost", Username: "cri", OS: "linux", Backend: "paru"}
	stubStatusDeps(t, f, allInstalledBackend{}, fakeMiseNoOp)

	dir := t.TempDir()
	target := filepath.Join(dir, "live-target")
	require.NoError(t, os.WriteFile(target, []byte("old\n"), 0o644))
	modDir := filepath.Join(dir, "modules", "demo")
	require.NoError(t, os.MkdirAll(filepath.Join(modDir, "files"), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(modDir, "files", "config"), []byte("new\n"), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(modDir, "module.toml"), []byte(`id = "demo"
app = "demo"

[dotfiles]
"`+target+`" = { source = "files/config", mode = "copy" }
`), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "dotdrift.toml"), []byte("[modules]\ndisable = []\n"), 0o644))

	var buf bytes.Buffer
	err := (&StatusCmd{Profile: dir, State: filepath.Join(t.TempDir(), "state.json"), Diff: "nonexistent-diff-tool-xyz", out: &buf}).Run()
	require.Error(t, err)
	require.Contains(t, err.Error(), "nonexistent-diff-tool-xyz")
}
