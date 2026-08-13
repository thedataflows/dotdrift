package cmd

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/thedataflows/dotdrift/internal/facts"
	"github.com/thedataflows/dotdrift/internal/mise"
	"github.com/thedataflows/dotdrift/internal/resolve"
)

func scopeFixture(t *testing.T) string {
	t.Helper()
	return filepath.Join("..", "testdata", "profiles", "scope")
}

// A profile with system-scope modules gains a dotfiles-system step that runs
// after dotfiles, applies only the system entries via mise dotfiles apply from
// its own config dir, and is recorded in resume state.
func TestApply_dotfilesSystemStep(t *testing.T) {
	dir := t.TempDir()
	statePath := filepath.Join(dir, "state.json")
	f := &facts.Facts{Hostname: "myhost", Username: "cri", OS: "linux", Backend: "paru"}
	events, _ := stubApplyDeps(t, f)

	cmd := &ApplyCmd{Profile: scopeFixture(t), State: statePath, Yes: true}
	require.NoError(t, cmd.Run())

	// User and system dotfiles both use mise dotfiles apply, each from their
	// own config dir; the system step runs after the user step.
	userApply := "dotfiles apply --cd " + filepath.Join(dir, "mise", "dotfiles")
	systemApply := "dotfiles apply --cd " + filepath.Join(dir, "mise", "system")
	userIdx := -1
	for i, e := range *events {
		if strings.Contains(e, userApply) {
			userIdx = i
		}
	}
	systemIdx := -1
	for i, e := range *events {
		if strings.Contains(e, systemApply) {
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
	require.Contains(t, string(sysCfg), "[dotfiles]")
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

// System-scope edit entries (line/block/template) apply via the same unified
// dotfiles apply path as whole-file entries — the system step writes a single
// [dotfiles] config and invokes mise dotfiles apply. When the OS denies access
// (permission denied), the step retries elevated via DotfilesApplySudo.
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

	// The system edit runs as `mise dotfiles apply` from the system/ config dir.
	var foundApply bool
	for _, e := range *events {
		if strings.Contains(e, "dotfiles apply") && strings.Contains(e, filepath.Join("mise", "system")) {
			foundApply = true
		}
	}
	require.True(t, foundApply, "system edit entries must reach dotfiles apply, events: %v", *events)

	// The system config carries a [dotfiles] section (not [bootstrap.files]).
	sysCfg, err := os.ReadFile(filepath.Join(dir, "mise", "system", "mise.toml"))
	require.NoError(t, err)
	require.Contains(t, string(sysCfg), "[dotfiles]")
	require.Contains(t, string(sysCfg), `line = "127.0.0.1 dev.local"`)
	require.NotContains(t, string(sysCfg), "[bootstrap.files]")
}

// System-scope whole-file and edit entries coexist in one system step:
// both apply via a single [dotfiles] config and one mise dotfiles apply call.
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

	// Both whole-file and edit entries reach the same dotfiles apply call.
	var foundApply bool
	for _, e := range *events {
		if strings.Contains(e, "dotfiles apply") && strings.Contains(e, filepath.Join("mise", "system")) {
			foundApply = true
		}
	}
	require.True(t, foundApply, "system entries must reach dotfiles apply, events: %v", *events)

	// The unified config has both entries in [dotfiles].
	sysCfg, err := os.ReadFile(filepath.Join(dir, "mise", "system", "mise.toml"))
	require.NoError(t, err)
	require.Contains(t, string(sysCfg), "/etc/demo.conf")
	require.Contains(t, string(sysCfg), `line = "127.0.0.1 dev.local"`)
}

// When mise dotfiles apply fails with permission denied (system paths like
// /etc), the system step retries elevated via DotfilesApplySudo (sudo -E).
func TestSystemFilesStep_retriesElevatedOnPermissionDenied(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("elevation retry test requires a non-root user")
	}
	dir := t.TempDir()
	configPath := filepath.Join(dir, "system", "mise.toml")

	callCount := 0
	m := &mise.Mise{
		LookPath: func(string) (string, error) { return "/fake/mise", nil },
		RunContext: func(_ context.Context, name string, args ...string) (string, error) {
			if len(args) > 0 && args[0] == "--version" {
				return mise.MinMiseVersion + "\n", nil
			}
			callCount++
			// Non-sudo attempt fails with permission denied.
			if name != "sudo" {
				return "", fmt.Errorf("exit status 1\nPermission denied (os error 13)")
			}
			// Sudo retry succeeds.
			return "", nil
		},
	}
	em := mise.NewExecMise(m)

	step := &systemFilesStep{
		exec: em,
		entries: []resolve.DotfileEntry{
			{Target: "/etc/test.conf", Source: "test.conf", Mode: "copy"},
		},
		sourceRoot: "/fake/profile",
		homeDir:    "/home/test",
		configPath: configPath,
		yes:        true,
	}

	require.NoError(t, step.Run(context.Background()))
	require.Equal(t, 2, callCount,
		"should try dotfiles apply (fail) then retry elevated (succeed)")
}

// When mise dotfiles apply fails with a non-permission error, the system step
// does NOT retry elevated — the original error propagates.
func TestSystemFilesStep_noRetryOnNonPermissionError(t *testing.T) {
	dir := t.TempDir()
	configPath := filepath.Join(dir, "system", "mise.toml")

	callCount := 0
	m := &mise.Mise{
		LookPath: func(string) (string, error) { return "/fake/mise", nil },
		RunContext: func(_ context.Context, name string, args ...string) (string, error) {
			if len(args) > 0 && args[0] == "--version" {
				return mise.MinMiseVersion + "\n", nil
			}
			callCount++
			return "", fmt.Errorf("exit status 1\nmise ERROR config parse error")
		},
	}
	em := mise.NewExecMise(m)

	step := &systemFilesStep{
		exec: em,
		entries: []resolve.DotfileEntry{
			{Target: "/etc/test.conf", Source: "test.conf", Mode: "copy"},
		},
		sourceRoot: "/fake/profile",
		homeDir:    "/home/test",
		configPath: configPath,
		yes:        true,
	}

	err := step.Run(context.Background())
	require.Error(t, err)
	require.Equal(t, 1, callCount, "should not retry on non-permission errors")
	require.Contains(t, err.Error(), "config parse error")
}

// System-scope entries declared as symlink are translated to copy mode in the
// generated [dotfiles] config — a symlink from /etc into the user's profile is
// fragile. symlink-each entries are expanded to individual copy entries.
func TestSystemFilesStep_symlinkTranslatedToCopy(t *testing.T) {
	dir := t.TempDir()
	configPath := filepath.Join(dir, "system", "mise.toml")

	m := &mise.Mise{
		LookPath: func(string) (string, error) { return "/fake/mise", nil },
		RunContext: func(_ context.Context, name string, args ...string) (string, error) {
			if len(args) > 0 && args[0] == "--version" {
				return mise.MinMiseVersion + "\n", nil
			}
			return "", nil
		},
	}
	em := mise.NewExecMise(m)

	step := &systemFilesStep{
		exec: em,
		entries: []resolve.DotfileEntry{
			{Target: "/etc/symlinked.conf", Source: "files/symlinked.conf", Mode: "symlink"},
		},
		sourceRoot: "/fake/profile",
		homeDir:    "/home/test",
		configPath: configPath,
		yes:        true,
	}

	require.NoError(t, step.Run(context.Background()))
	cfg, err := os.ReadFile(configPath)
	require.NoError(t, err)
	require.Contains(t, string(cfg), `mode = "copy"`,
		"system symlink entries must be translated to copy mode")
	require.NotContains(t, string(cfg), `mode = "symlink"`,
		"system symlink entries must NOT keep symlink mode")
}
