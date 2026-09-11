package cmd

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/thedataflows/dotdrift/internal/facts"
)

// recordHandover redirects the CLI's terminal handover into the events
// slice: the fake mise path must never really exec, and the recorded argv
// is the observable for "which child would run on the terminal" (0071).
func recordHandover(t *testing.T, events *[]string) {
	t.Helper()
	orig := handoverToTerminal
	handoverToTerminal = func(cmd *exec.Cmd) error {
		*events = append(*events, "handover:"+cmd.Path+" "+strings.Join(cmd.Args, " "))
		return nil
	}
	t.Cleanup(func() { handoverToTerminal = orig })
}

func scopeFixture(t *testing.T) string {
	t.Helper()
	return filepath.Join("..", "testdata", "profiles", "scope")
}

// A profile with system-scope modules gains a dotfiles-system step that runs
// after dotfiles, converges whole-file system entries via mise bootstrap
// --only files from its own config dir (issue 0042), and is recorded in
// resume state.
func TestApply_dotfilesSystemStep(t *testing.T) {
	dir := t.TempDir()
	statePath := filepath.Join(dir, "state.json")
	f := &facts.Facts{Hostname: "myhost", Username: "cri", OS: "linux", Backend: "paru"}
	fk := stubApplyDeps(t, f)

	cmd := &ApplyCmd{deps: &fk.deps, Profile: scopeFixture(t), State: statePath, Yes: true}
	require.NoError(t, cmd.Run())

	// User dotfiles use mise dotfiles apply; system whole-file entries use
	// mise bootstrap --only files; the system step runs after the user step.
	userApply := "dotfiles apply --cd " + filepath.Join(dir, "mise", "dotfiles")
	systemApply := "bootstrap --cd " + filepath.Join(dir, "mise", "system")
	userIdx := -1
	for i, e := range *fk.events {
		if strings.Contains(e, userApply) {
			userIdx = i
		}
	}
	systemIdx := -1
	for i, e := range *fk.events {
		if strings.Contains(e, systemApply) {
			systemIdx = i
		}
	}
	require.GreaterOrEqual(t, userIdx, 0, "user dotfiles apply missing in %v", *fk.events)
	require.Greater(t, systemIdx, userIdx, "system files step must run after dotfiles in %v", *fk.events)

	// The per-step configs are partitioned by scope.
	userCfg, err := os.ReadFile(filepath.Join(dir, "mise", "dotfiles", "mise.toml"))
	require.NoError(t, err)
	require.Contains(t, string(userCfg), "~/.bashrc")
	require.NotContains(t, string(userCfg), "/etc/demo.conf")

	// Whole-file system entries are [bootstrap.files], not [dotfiles].
	sysCfg, err := os.ReadFile(filepath.Join(dir, "mise", "system", "mise.toml"))
	require.NoError(t, err)
	require.Contains(t, string(sysCfg), "[bootstrap.files]")
	require.Contains(t, string(sysCfg), "/etc/demo.conf")
	require.NotContains(t, string(sysCfg), "[dotfiles]")

	// The pre-pipeline full config (D8a crash snapshot) still contains everything.
	full, err := os.ReadFile(filepath.Join(dir, "mise", "shared", "mise.toml"))
	require.NoError(t, err)
	require.Contains(t, string(full), "/etc/demo.conf")
	require.Contains(t, string(full), "~/.bashrc")

	_, statErr := os.Stat(statePath)
	require.True(t, os.IsNotExist(statErr), "state file must be removed after a successful apply")
}

// Without system-scope entries there is no dotfiles-system step: no
// invocation, no config dir, no completed state entry.
func TestApply_noSystemEntriesSkipsDotfilesSystem(t *testing.T) {
	dir := t.TempDir()
	statePath := filepath.Join(dir, "state.json")
	f := &facts.Facts{Hostname: "myhost", Username: "cri", OS: "linux", Backend: "paru"}
	fk := stubApplyDeps(t, f)

	cmd := &ApplyCmd{deps: &fk.deps, Profile: resolveFixture(t), State: statePath, Yes: true}
	require.NoError(t, cmd.Run())

	for _, e := range *fk.events {
		require.NotContains(t, e, "dotfiles-system", "no system step must run for a user-only plan")
	}

	_, statErr := os.Stat(statePath)
	require.True(t, os.IsNotExist(statErr), "state file must be removed after a successful apply")

	_, err := os.Stat(filepath.Join(dir, "mise", "dotfiles-system"))
	require.True(t, os.IsNotExist(err), "no dotfiles-system config dir must be created")
}

// System-scope edit entries (line/block/template) have no bootstrap.files
// equivalent (contract #18): they keep the elevated [dotfiles] path, from
// their own system-edits config dir (issue 0042).
func TestApply_systemEditEntriesUseDotfilesApply(t *testing.T) {
	dir := t.TempDir()
	statePath := filepath.Join(dir, "state.json")
	// Build a profile with one system-scope module carrying an edit entry.
	profileDir := t.TempDir()
	modDir := filepath.Join(profileDir, "modules", "sysmod")
	require.NoError(t, os.MkdirAll(modDir, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(modDir, "module.toml"), []byte(`
scope = "system"
[dotfiles]
"/etc/hosts/dev" = { line = "127.0.0.1 dev.local" }
`), 0o644))

	f := &facts.Facts{Hostname: "h", Username: "u", OS: "linux", Backend: "paru"}
	fk := stubApplyDeps(t, f)
	recordHandover(t, fk.events)

	cmd := &ApplyCmd{deps: &fk.deps, Profile: profileDir, State: statePath, Yes: true}
	require.NoError(t, cmd.Run())

	// The system edit runs as `sudo -E mise dotfiles apply` from the
	// system-edits/ config dir (0071: through the handover seam); no
	// bootstrap files phase runs for an edits-only plan.
	var foundApply bool
	for _, e := range *fk.events {
		if strings.Contains(e, "dotfiles apply") && strings.Contains(e, filepath.Join("mise", "system-edits")) {
			foundApply = true
		}
		require.NotContains(t, e, filepath.Join("mise", "system "), "no bootstrap system config for edits-only: %v", e)
	}
	require.True(t, foundApply, "system edit entries must reach dotfiles apply, events: %v", *fk.events)

	// The edits config carries a [dotfiles] section (not [bootstrap.files]).
	editCfg, err := os.ReadFile(filepath.Join(dir, "mise", "system-edits", "mise.toml"))
	require.NoError(t, err)
	require.Contains(t, string(editCfg), "[dotfiles]")
	require.Contains(t, string(editCfg), `line = "127.0.0.1 dev.local"`)
	require.NotContains(t, string(editCfg), "[bootstrap.files]")
}

// System-scope whole-file and edit entries split across two configs in one
// system step: whole-file entries converge via [bootstrap.files] +
// `bootstrap --only files`, edits via [dotfiles] + `dotfiles apply`.
func TestApply_systemWholeFileAndEditBothApply(t *testing.T) {
	dir := t.TempDir()
	statePath := filepath.Join(dir, "state.json")
	profileDir := t.TempDir()
	modDir := filepath.Join(profileDir, "modules", "sysmod")
	require.NoError(t, os.MkdirAll(modDir, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(modDir, "demo.conf"), []byte("config"), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(modDir, "module.toml"), []byte(`
scope = "system"
[dotfiles]
"/etc/demo.conf" = { source = "demo.conf", mode = "copy" }
"/etc/hosts/dev" = { line = "127.0.0.1 dev.local" }
`), 0o644))

	f := &facts.Facts{Hostname: "h", Username: "u", OS: "linux", Backend: "paru"}
	fk := stubApplyDeps(t, f)
	recordHandover(t, fk.events)

	cmd := &ApplyCmd{deps: &fk.deps, Profile: profileDir, State: statePath, Yes: true}
	require.NoError(t, cmd.Run())

	// Whole-file entries reach bootstrap --only files from system/.
	var foundBootstrap bool
	for _, e := range *fk.events {
		if strings.Contains(e, "bootstrap") && strings.Contains(e, filepath.Join("mise", "system")) && strings.Contains(e, "--only files") {
			foundBootstrap = true
		}
	}
	require.True(t, foundBootstrap, "whole-file system entries must reach bootstrap --only files, events: %v", *fk.events)

	// Edits reach dotfiles apply from system-edits/.
	var foundApply bool
	for _, e := range *fk.events {
		if strings.Contains(e, "dotfiles apply") && strings.Contains(e, filepath.Join("mise", "system-edits")) {
			foundApply = true
		}
	}
	require.True(t, foundApply, "system edit entries must reach dotfiles apply, events: %v", *fk.events)

	// The configs are partitioned: bootstrap.files holds the whole-file entry,
	// dotfiles holds the edit.
	sysCfg, err := os.ReadFile(filepath.Join(dir, "mise", "system", "mise.toml"))
	require.NoError(t, err)
	require.Contains(t, string(sysCfg), "[bootstrap.files]")
	require.Contains(t, string(sysCfg), "/etc/demo.conf")
	require.NotContains(t, string(sysCfg), "127.0.0.1 dev.local")

	editCfg, err := os.ReadFile(filepath.Join(dir, "mise", "system-edits", "mise.toml"))
	require.NoError(t, err)
	require.Contains(t, string(editCfg), `line = "127.0.0.1 dev.local"`)
	require.NotContains(t, string(editCfg), "/etc/demo.conf")
}
