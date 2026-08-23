package cmd

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/thedataflows/dotdrift/internal/facts"
)

// backupFixtureProfile writes a profile with one base module declaring a
// copy-mode and a symlink-mode dotfile for the same target dir, plus a host
// overlay module with its own copy entry. Returns the profile root.
func backupFixtureProfile(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	write := func(rel, content string) {
		path := filepath.Join(root, rel)
		require.NoError(t, os.MkdirAll(filepath.Dir(path), 0o755))
		require.NoError(t, os.WriteFile(path, []byte(content), 0o644))
	}
	write("modules/demo/module.toml", `[dotfiles]
"~/.config/demo/app.conf" = { source = "home/app.conf", mode = "copy" }
"~/.config/demo/linked.conf" = { source = "home/linked.conf", mode = "symlink" }
`)
	write("modules/demo/home/app.conf", "profile copy content")
	write("modules/demo/home/linked.conf", "profile link content")
	write("hosts/myhost/modules/demo/module.toml", `[dotfiles]
"~/.config/demo/host.conf" = { source = "home/host.conf", mode = "copy" }
`)
	write("hosts/myhost/modules/demo/home/host.conf", "host copy content")
	return root
}

// backupFixtureHome seeds the live targets the fixture declares: the two
// copy targets exist with drifted content; the symlink target is absent
// (nothing to back up either way). Returns the home dir.
func backupFixtureHome(t *testing.T) string {
	t.Helper()
	home := t.TempDir()
	live := filepath.Join(home, ".config", "demo")
	require.NoError(t, os.MkdirAll(live, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(live, "app.conf"), []byte("local edits"), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(live, "host.conf"), []byte("host local edits"), 0o644))
	return home
}

// applyBackupDeps runs apply with the standard fake mise plumbing and the
// given backup flag value, returning the output buffer.
func applyBackupDeps(t *testing.T, backup bool, sections []string) (*strings.Builder, string) {
	t.Helper()
	home := backupFixtureHome(t)
	t.Setenv("HOME", home)
	root := backupFixtureProfile(t)
	statePath := filepath.Join(t.TempDir(), "state.json")

	f := &facts.Facts{Hostname: "myhost", Username: "cri", OS: "linux", Backend: "paru"}
	stubApplyDeps(t, f)

	var out strings.Builder
	cmd := &ApplyCmd{Profile: root, State: statePath, Yes: true, Backup: backup, Out: &out}
	if sections != nil {
		cmd.onlySections = sections
	}
	require.NoError(t, cmd.Run())
	return &out, root
}

// findGeneration returns the single backups generation directory under the
// given module dir (one apply = one generation).
func findGeneration(t *testing.T, moduleDir string) string {
	t.Helper()
	gens, err := filepath.Glob(filepath.Join(moduleDir, "backups", "*"))
	require.NoError(t, err)
	require.Len(t, gens, 1, "exactly one generation under %s", moduleDir)
	return gens[0]
}

// --backup snapshots every existing copy-mode destination into the
// declaring module's backups/ tree before the pipeline runs: base entries
// land in modules/<m>/backups, host-layer entries in hosts/<h>/modules/<m>/
// backups, and symlink-mode targets are left alone (their destination is a
// link, not content mise overwrites).
func TestApply_backupSnapshotsCopyTargets(t *testing.T) {
	out, root := applyBackupDeps(t, true, nil)

	home, err := os.UserHomeDir()
	require.NoError(t, err)

	baseGen := findGeneration(t, filepath.Join(root, "modules", "demo"))
	hostGen := findGeneration(t, filepath.Join(root, "hosts", "myhost", "modules", "demo"))

	got, err := os.ReadFile(filepath.Join(baseGen, home, ".config", "demo", "app.conf"))
	require.NoError(t, err, "base copy target must be backed up")
	require.Equal(t, "local edits", string(got))

	got, err = os.ReadFile(filepath.Join(hostGen, home, ".config", "demo", "host.conf"))
	require.NoError(t, err, "host-layer copy target must back up into the host module dir")
	require.Equal(t, "host local edits", string(got))

	_, err = os.Stat(filepath.Join(baseGen, home, ".config", "demo", "linked.conf"))
	require.True(t, os.IsNotExist(err), "symlink-mode target is not backed up")

	require.Contains(t, out.String(), "backup:", "a summary line reports the backups")
}

// Without --backup nothing is written: no backups/ directory appears.
func TestApply_noBackupByDefault(t *testing.T) {
	_, root := applyBackupDeps(t, false, nil)

	_, err := os.Stat(filepath.Join(root, "modules", "demo", "backups"))
	require.True(t, os.IsNotExist(err), "backups/ must not exist without --backup")
}

// --backup with the dotfiles section deselected writes nothing: no copy
// step runs, so there is nothing to safeguard.
func TestApply_backupSkippedWithoutDotfiles(t *testing.T) {
	_, root := applyBackupDeps(t, true, []string{"packages"})

	_, err := os.Stat(filepath.Join(root, "modules", "demo", "backups"))
	require.True(t, os.IsNotExist(err), "no backups without the dotfiles section")
}
