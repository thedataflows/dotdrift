package cmd

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/thedataflows/dotdrift/internal/facts"
)

// systemdFixture builds a profile with one module declaring a user service
// and a timer (issue 0048).
func systemdFixture(t *testing.T) string {
	t.Helper()
	profileDir := t.TempDir()
	modDir := filepath.Join(profileDir, "modules", "syncer")
	require.NoError(t, os.MkdirAll(modDir, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(modDir, "module.toml"), []byte(`
[systemd.units.my-sync]
description = "sync files"
exec_start = "~/.local/bin/my-sync --watch"
after = ["network-online.target"]

[systemd.units.my-sync-timer]
on_boot_sec = "2min"
unit = "my-sync"
`), 0o644))
	return profileDir
}

// A profile with systemd units gains a systemd step after dotfiles that
// converges via mise bootstrap --only linux-systemd-units from its own
// config dir.
func TestApply_systemdStep(t *testing.T) {
	dir := t.TempDir()
	statePath := filepath.Join(dir, "state.json")
	f := &facts.Facts{Hostname: "myhost", Username: "cri", OS: "linux", Backend: "paru"}
	events, _ := stubApplyDeps(t, f)

	cmd := &ApplyCmd{Profile: systemdFixture(t), State: statePath, Yes: true}
	require.NoError(t, cmd.Run())

	unitIdx := -1
	for i, e := range *events {
		if strings.Contains(e, "bootstrap") && strings.Contains(e, filepath.Join("mise", "systemd")) && strings.Contains(e, "--only linux-systemd-units") {
			unitIdx = i
		}
	}
	require.GreaterOrEqual(t, unitIdx, 0, "systemd units bootstrap missing in %v", *events)

	cfg, err := os.ReadFile(filepath.Join(dir, "mise", "systemd", "mise.toml"))
	require.NoError(t, err)
	require.Contains(t, string(cfg), "[bootstrap.linux.systemd.units.my-sync]")
	require.Contains(t, string(cfg), `exec_start = "~/.local/bin/my-sync --watch"`)
	require.Contains(t, string(cfg), "[bootstrap.linux.systemd.units.my-sync-timer]")
	require.Contains(t, string(cfg), `on_boot_sec = "2min"`)
}

// --no-systemd drops the units step; --systemd alone runs only it (plus the
// always-on machinery).
func TestApply_sectionFlagsSystemd(t *testing.T) {
	dir := t.TempDir()
	statePath := filepath.Join(dir, "state.json")
	f := &facts.Facts{Hostname: "myhost", Username: "cri", OS: "linux", Backend: "paru"}
	events, _ := stubApplyDeps(t, f)

	cmd := &ApplyCmd{Profile: systemdFixture(t), State: statePath, Yes: true,
		onlySections: []string{"packages", "tools", "dotfiles", "mounts", "smb", "hooks"}}
	require.NoError(t, cmd.Run())

	for _, e := range *events {
		require.NotContains(t, e, "linux-systemd-units", "systemd step must not run when deselected")
	}
	_, err := os.Stat(filepath.Join(dir, "mise", "systemd"))
	require.True(t, os.IsNotExist(err), "no systemd config dir when deselected")
}

// No units declared → no systemd step, no config dir.
func TestApply_noSystemdNoStep(t *testing.T) {
	dir := t.TempDir()
	statePath := filepath.Join(dir, "state.json")
	f := &facts.Facts{Hostname: "myhost", Username: "cri", OS: "linux", Backend: "paru"}
	events, _ := stubApplyDeps(t, f)

	cmd := &ApplyCmd{Profile: resolveFixture(t), State: statePath, Yes: true}
	require.NoError(t, cmd.Run())

	for _, e := range *events {
		require.NotContains(t, e, "linux-systemd-units")
	}
}
