package service

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/thedataflows/dotdrift/internal/mise"
	"github.com/thedataflows/dotdrift/internal/profile"
	"github.com/thedataflows/dotdrift/internal/resolve"
)

// White-box unit tests for the systemFilesStep internals, moved here from
// cmd/apply_scope_test.go by issue 0070 (the step types now live in this
// package — 0064-D6 absorbed the steps into the session, 0070 deleted the
// cmd copy these tests targeted).

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
