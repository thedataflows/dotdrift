package cmd

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/thedataflows/dotdrift/internal/facts"
)

func scopeFixture(t *testing.T) string {
	t.Helper()
	return filepath.Join("..", "testdata", "profiles", "scope")
}

// A profile with system-scope modules gains a dotfiles-system step that runs
// after dotfiles, applies only the system entries from its own config dir,
// and is recorded in resume state.
func TestApply_dotfilesSystemStep(t *testing.T) {
	dir := t.TempDir()
	statePath := filepath.Join(dir, "state.json")
	f := &facts.Facts{Hostname: "myhost", Username: "cri", OS: "linux", Backend: "paru"}
	events, _ := stubApplyDeps(t, f)

	cmd := &ApplyCmd{Profile: scopeFixture(t), State: statePath, Yes: true}
	require.NoError(t, cmd.Run())

	// User dotfiles still use mise dotfiles apply; system dotfiles now use
	// mise bootstrap --only files from the system/ config dir.
	userApply := "dotfiles apply --cd " + filepath.Join(dir, "mise", "dotfiles")
	userIdx := -1
	for i, e := range *events {
		if strings.Contains(e, userApply) {
			userIdx = i
		}
	}
	systemIdx := -1
	for i, e := range *events {
		if strings.Contains(e, "bootstrap") && strings.Contains(e, "--only files") {
			systemIdx = i
		}
	}
	require.GreaterOrEqual(t, userIdx, 0, "user dotfiles apply missing in %v", *events)
	require.Greater(t, systemIdx, userIdx, "system files step must run after dotfiles in %v", *events)

	// The per-step configs are partitioned by scope.
	userCfg, err := os.ReadFile(filepath.Join(dir, "mise", "dotfiles", "mise.toml"))
	require.NoError(t, err)
	require.Contains(t, string(userCfg), "~/.bashrc")
	require.NotContains(t, string(userCfg), "/etc/demo.conf")

	sysCfg, err := os.ReadFile(filepath.Join(dir, "mise", "system", "mise.toml"))
	require.NoError(t, err)
	require.Contains(t, string(sysCfg), "[bootstrap.files]")
	require.Contains(t, string(sysCfg), "/etc/demo.conf")

	// The pre-pipeline full config (D8a crash snapshot) still contains everything.
	full, err := os.ReadFile(filepath.Join(dir, "mise", "mise.toml"))
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
	events, _ := stubApplyDeps(t, f)

	cmd := &ApplyCmd{Profile: resolveFixture(t), State: statePath, Yes: true}
	require.NoError(t, cmd.Run())

	for _, e := range *events {
		require.NotContains(t, e, "dotfiles-system", "no system step must run for a user-only plan")
	}

	_, statErr := os.Stat(statePath)
	require.True(t, os.IsNotExist(statErr), "state file must be removed after a successful apply")

	_, err := os.Stat(filepath.Join(dir, "mise", "dotfiles-system"))
	require.True(t, os.IsNotExist(err), "no dotfiles-system config dir must be created")
}

// System-scope edit entries (line/block/template) run through the elevated
// mise dotfiles apply path — [bootstrap.files] can only place files, not edit
// them in place. The system step writes a dedicated [dotfiles] config and
// invokes DotfilesApplySudo for the edit entries.
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
	events, _ := stubApplyDeps(t, f)

	cmd := &ApplyCmd{Profile: profileDir, State: statePath, Yes: true}
	require.NoError(t, cmd.Run())

	// The system edit runs as an elevated `mise dotfiles apply` from the
	// system/edits config dir.
	var foundEditApply bool
	for _, e := range *events {
		if strings.Contains(e, "dotfiles apply") && strings.Contains(e, filepath.Join("system", "edits")) {
			foundEditApply = true
		}
	}
	require.True(t, foundEditApply, "system edit entries must reach elevated dotfiles apply, events: %v", *events)

	// The edit config carries a [dotfiles] section (not [bootstrap.files]).
	editCfg, err := os.ReadFile(filepath.Join(dir, "mise", "system", "edits", "mise.toml"))
	require.NoError(t, err)
	require.Contains(t, string(editCfg), "[dotfiles]")
	require.Contains(t, string(editCfg), `line = "127.0.0.1 dev.local"`)
	require.NotContains(t, string(editCfg), "[bootstrap.files]")
}

// System-scope whole-file and edit entries coexist in one system step:
// whole-file → [bootstrap.files], edit → elevated dotfiles apply.
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
	events, _ := stubApplyDeps(t, f)

	cmd := &ApplyCmd{Profile: profileDir, State: statePath, Yes: true}
	require.NoError(t, cmd.Run())

	var foundBootstrap, foundEditApply bool
	for _, e := range *events {
		if strings.Contains(e, "bootstrap") && strings.Contains(e, "--only files") {
			foundBootstrap = true
		}
		if strings.Contains(e, "dotfiles apply") && strings.Contains(e, filepath.Join("system", "edits")) {
			foundEditApply = true
		}
	}
	require.True(t, foundBootstrap, "whole-file system entries must reach bootstrap --only files, events: %v", *events)
	require.True(t, foundEditApply, "system edit entries must reach elevated dotfiles apply, events: %v", *events)
}
