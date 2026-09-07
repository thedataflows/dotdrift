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
	"github.com/thedataflows/dotdrift/internal/profile"
	"github.com/thedataflows/dotdrift/internal/resolve"
)

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
	events, _ := stubApplyDeps(t, f)

	cmd := &ApplyCmd{Profile: scopeFixture(t), State: statePath, Yes: true}
	require.NoError(t, cmd.Run())

	// User dotfiles use mise dotfiles apply; system whole-file entries use
	// mise bootstrap --only files; the system step runs after the user step.
	userApply := "dotfiles apply --cd " + filepath.Join(dir, "mise", "dotfiles")
	systemApply := "bootstrap --cd " + filepath.Join(dir, "mise", "system")
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

	// Whole-file system entries are [bootstrap.files], not [dotfiles].
	sysCfg, err := os.ReadFile(filepath.Join(dir, "mise", "system", "mise.toml"))
	require.NoError(t, err)
	require.Contains(t, string(sysCfg), "[bootstrap.files]")
	require.Contains(t, string(sysCfg), "/etc/demo.conf")
	require.NotContains(t, string(sysCfg), "[dotfiles]")

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
	events, _ := stubApplyDeps(t, f)

	cmd := &ApplyCmd{Profile: profileDir, State: statePath, Yes: true}
	require.NoError(t, cmd.Run())

	// The system edit runs as `mise dotfiles apply` from the system-edits/
	// config dir; no bootstrap files phase runs for an edits-only plan.
	var foundApply bool
	for _, e := range *events {
		if strings.Contains(e, "dotfiles apply") && strings.Contains(e, filepath.Join("mise", "system-edits")) {
			foundApply = true
		}
		require.NotContains(t, e, filepath.Join("mise", "system "), "no bootstrap system config for edits-only: %v", e)
	}
	require.True(t, foundApply, "system edit entries must reach dotfiles apply, events: %v", *events)

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
	events, _ := stubApplyDeps(t, f)

	cmd := &ApplyCmd{Profile: profileDir, State: statePath, Yes: true}
	require.NoError(t, cmd.Run())

	// Whole-file entries reach bootstrap --only files from system/.
	var foundBootstrap bool
	for _, e := range *events {
		if strings.Contains(e, "bootstrap") && strings.Contains(e, filepath.Join("mise", "system")) && strings.Contains(e, "--only files") {
			foundBootstrap = true
		}
	}
	require.True(t, foundBootstrap, "whole-file system entries must reach bootstrap --only files, events: %v", *events)

	// Edits reach dotfiles apply from system-edits/.
	var foundApply bool
	for _, e := range *events {
		if strings.Contains(e, "dotfiles apply") && strings.Contains(e, filepath.Join("mise", "system-edits")) {
			foundApply = true
		}
	}
	require.True(t, foundApply, "system edit entries must reach dotfiles apply, events: %v", *events)

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

// Whole-file system entries converge via mise bootstrap --only files and
// dotdrift never elevates them itself (issue 0042): mise tries as the current
// user and retries the remaining changes in one privileged batch — the old
// dotdrift-side writability pre-flight + sudo pass is gone for whole files.
func TestSystemFilesStep_wholeFilesViaBootstrapNeverSudo(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("elevation test requires a non-root user")
	}
	dir := t.TempDir()
	configPath := filepath.Join(dir, "system", "mise.toml")

	var names []string
	m := &mise.Mise{
		LookPath: func(string) (string, error) { return "/fake/mise", nil },
		RunContext: func(_ context.Context, name string, args ...string) (string, error) {
			if len(args) > 0 && args[0] == "--version" {
				return mise.MinMiseVersion + "\n", nil
			}
			names = append(names, name)
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
		editsPath:  filepath.Join(dir, "system-edits", "mise.toml"),
		yes:        true,
	}

	require.NoError(t, step.Run(context.Background()))
	require.Equal(t, []string{"/fake/mise"}, names,
		"whole-file entries converge through mise bootstrap; dotdrift never invokes sudo for them")

	cfg, err := os.ReadFile(configPath)
	require.NoError(t, err)
	require.Contains(t, string(cfg), "[bootstrap.files]")
	require.Contains(t, string(cfg), `"/etc/test.conf" = { source = "/fake/profile/test.conf" }`)
}

// A bootstrap failure propagates verbatim — no dotdrift-side retry or
// elevation second-guessing (mise already self-elevated or failed loud).
func TestSystemFilesStep_bootstrapFailurePropagates(t *testing.T) {
	dir := t.TempDir()
	configPath := filepath.Join(dir, "system", "mise.toml")

	var names []string
	m := &mise.Mise{
		LookPath: func(string) (string, error) { return "/fake/mise", nil },
		RunContext: func(_ context.Context, name string, args ...string) (string, error) {
			if len(args) > 0 && args[0] == "--version" {
				return mise.MinMiseVersion + "\n", nil
			}
			names = append(names, name)
			return "", fmt.Errorf("exit status 1\nmise ERROR config parse error")
		},
	}
	em := mise.NewExecMise(m)

	step := &systemFilesStep{
		exec: em,
		entries: []resolve.DotfileEntry{
			{Target: filepath.Join(dir, "mine.conf"), Source: "mine.conf", Mode: "copy"},
		},
		sourceRoot: "/fake/profile",
		homeDir:    "/home/test",
		configPath: configPath,
		editsPath:  filepath.Join(dir, "system-edits", "mise.toml"),
		yes:        true,
	}

	err := step.Run(context.Background())
	require.Error(t, err)
	require.NotContains(t, names, "sudo")
	require.Contains(t, err.Error(), "config parse error")
}

// System-scope EDIT entries keep the elevated [dotfiles] path: a
// non-writable edit target still converges via DotfilesApplySudo (contract
// #18 — bootstrap.files has no edit concept).
func TestSystemFilesStep_editEntriesElevatedWhenNotWritable(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("elevation test requires a non-root user")
	}
	dir := t.TempDir()

	var names []string
	m := &mise.Mise{
		LookPath: func(string) (string, error) { return "/fake/mise", nil },
		RunContext: func(_ context.Context, name string, args ...string) (string, error) {
			if len(args) > 0 && args[0] == "--version" {
				return mise.MinMiseVersion + "\n", nil
			}
			names = append(names, name)
			return "", nil
		},
	}
	em := mise.NewExecMise(m)

	editsPath := filepath.Join(dir, "system-edits", "mise.toml")
	step := &systemFilesStep{
		exec: em,
		entries: []resolve.DotfileEntry{
			{Target: "/etc/hosts/dev", Line: "127.0.0.1 dev.local"},
		},
		sourceRoot: "/fake/profile",
		homeDir:    "/home/test",
		configPath: filepath.Join(dir, "system", "mise.toml"),
		editsPath:  editsPath,
		yes:        true,
	}

	require.NoError(t, step.Run(context.Background()))
	require.Equal(t, []string{"sudo"}, names,
		"a non-writable edit target converges elevated in one sudo pass")

	cfg, err := os.ReadFile(editsPath)
	require.NoError(t, err)
	require.Contains(t, string(cfg), "[dotfiles]")
	require.Contains(t, string(cfg), `line = "127.0.0.1 dev.local"`)

	// No whole-file entries and no dirs → no bootstrap config is written.
	_, statErr := os.Stat(filepath.Join(dir, "system", "mise.toml"))
	require.True(t, os.IsNotExist(statErr), "edits-only step writes no bootstrap config")
}

// Mount destination directories are emitted as [bootstrap.directories] in the
// same system config and converge in the same bootstrap --only files call —
// no dotdrift-side mkdir/ensureDir (issue 0042).
func TestSystemFilesStep_mountDirsViaBootstrapDirectories(t *testing.T) {
	dir := t.TempDir()
	configPath := filepath.Join(dir, "system", "mise.toml")

	var names []string
	m := &mise.Mise{
		LookPath: func(string) (string, error) { return "/fake/mise", nil },
		RunContext: func(_ context.Context, name string, args ...string) (string, error) {
			if len(args) > 0 && args[0] == "--version" {
				return mise.MinMiseVersion + "\n", nil
			}
			names = append(names, name)
			return "", nil
		},
	}
	em := mise.NewExecMise(m)

	step := &systemFilesStep{
		exec:       em,
		sourceRoot: "/fake/profile",
		homeDir:    "/home/test",
		dirs:       []string{"/mnt/data", "/mnt/backup"},
		configPath: configPath,
		editsPath:  filepath.Join(dir, "system-edits", "mise.toml"),
		yes:        true,
	}

	require.NoError(t, step.Run(context.Background()))
	require.Equal(t, []string{"/fake/mise"}, names)

	cfg, err := os.ReadFile(configPath)
	require.NoError(t, err)
	require.Contains(t, string(cfg), "[bootstrap.directories]")
	require.Contains(t, string(cfg), "/mnt/data")
	require.Contains(t, string(cfg), "/mnt/backup")
}

// pathUserWritable walks up to the nearest existing ancestor and checks the
// write-access bit: a new file under a user-owned dir is writable; under a
// read-only dir (or root-owned /etc) it is not. Only the edit-entry elevation
// pre-flight uses this (issue 0042).
func TestPathUserWritable(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("writability test requires a non-root user")
	}
	userDir := t.TempDir()
	require.True(t, pathUserWritable(filepath.Join(userDir, "new-file")),
		"a path under a user-owned dir is writable")

	locked := t.TempDir()
	require.NoError(t, os.Chmod(locked, 0o555))
	t.Cleanup(func() { _ = os.Chmod(locked, 0o755) })
	require.False(t, pathUserWritable(filepath.Join(locked, "new-file")),
		"a path under a read-only dir is not writable")

	require.False(t, pathUserWritable("/etc/dotdrift-probe.conf"),
		"a path under root-owned /etc is not user-writable")
}

// System-scope entries declared as symlink carry no mode into
// [bootstrap.files]: mise's system files manage content (an inherent
// symlink→copy), so no symlink mode can leak into the emitted config.
// symlink-each entries are expanded to individual file entries.
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
		editsPath:  filepath.Join(dir, "system-edits", "mise.toml"),
		yes:        true,
	}

	require.NoError(t, step.Run(context.Background()))
	cfg, err := os.ReadFile(configPath)
	require.NoError(t, err)
	require.Contains(t, string(cfg), "[bootstrap.files]")
	require.Contains(t, string(cfg), `"/etc/symlinked.conf" = { source = "/fake/profile/files/symlinked.conf" }`)
	require.NotContains(t, string(cfg), "mode = ",
		"bootstrap.files manages content — no mode vocabulary may leak")
}

// Declared secrets are emitted as [bootstrap.secrets] into the system files
// config alongside [bootstrap.files] (issue 0044) — the only template path
// where mise resolves secret() ([dotfiles] templates have no secret
// function, verified against mise 2026.9.1).
func TestSystemFilesStep_includesSecrets(t *testing.T) {
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
			{Target: "/etc/svc.env", Source: "svc.env", Mode: "template"},
		},
		sourceRoot: "/fake/profile",
		homeDir:    "/home/test",
		configPath: configPath,
		editsPath:  filepath.Join(dir, "system-edits", "mise.toml"),
		yes:        true,
		secrets: map[string]profile.Secret{
			"cache_token": {Env: "MISE_CACHE_TOKEN"},
		},
	}

	require.NoError(t, step.Run(context.Background()))
	cfg, err := os.ReadFile(configPath)
	require.NoError(t, err)
	require.Contains(t, string(cfg), "[bootstrap.secrets]")
	require.Contains(t, string(cfg), `cache_token = "MISE_CACHE_TOKEN"`)
	require.Contains(t, string(cfg), "template = true")
}

// No declared secrets → no [bootstrap.secrets] section.
func TestSystemFilesStep_noSecretsNoSection(t *testing.T) {
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
			{Target: "/etc/test.conf", Source: "test.conf", Mode: "copy"},
		},
		sourceRoot: "/fake/profile",
		homeDir:    "/home/test",
		configPath: configPath,
		editsPath:  filepath.Join(dir, "system-edits", "mise.toml"),
		yes:        true,
	}

	require.NoError(t, step.Run(context.Background()))
	cfg, err := os.ReadFile(configPath)
	require.NoError(t, err)
	require.NotContains(t, string(cfg), "[bootstrap.secrets]")
}
