package backup_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/thedataflows/dotdrift/internal/backup"
)

// backupPath is the expected backup location: the absolute target path
// mirrored under the module's backups generation directory.
func backupPath(moduleDir, gen, target string) string {
	return filepath.Join(moduleDir, "backups", gen, strings.TrimPrefix(target, "/"))
}

// Run copies an existing file target into the module's backups generation
// directory, mirroring the absolute target path, and skips missing targets.
func TestRun_copiesExistingFileMirroringAbsPath(t *testing.T) {
	tmp := t.TempDir()
	live := filepath.Join(tmp, "home", "cri", ".bashrc")
	require.NoError(t, os.MkdirAll(filepath.Dir(live), 0o755))
	require.NoError(t, os.WriteFile(live, []byte("current"), 0o644))

	moduleDir := filepath.Join(tmp, "profile", "modules", "shell")
	missing := filepath.Join(tmp, "home", "cri", ".missing")

	n, err := backup.Run([]backup.File{
		{Target: live, ModuleDir: moduleDir},
		{Target: missing, ModuleDir: moduleDir},
	}, "20260824-120000")
	require.NoError(t, err)
	require.Equal(t, 1, n, "only the existing target is backed up")

	got, err := os.ReadFile(backupPath(moduleDir, "20260824-120000", live))
	require.NoError(t, err)
	require.Equal(t, "current", string(got))
}

// File modes survive the copy so a restored executable or private key keeps
// its permissions.
func TestRun_preservesFileMode(t *testing.T) {
	tmp := t.TempDir()
	live := filepath.Join(tmp, "home", "cri", "bin", "tool")
	require.NoError(t, os.MkdirAll(filepath.Dir(live), 0o755))
	require.NoError(t, os.WriteFile(live, []byte("#!/bin/sh\n"), 0o755))

	moduleDir := filepath.Join(tmp, "modules", "scripts")
	_, err := backup.Run([]backup.File{{Target: live, ModuleDir: moduleDir}}, "g1")
	require.NoError(t, err)

	info, err := os.Stat(backupPath(moduleDir, "g1", live))
	require.NoError(t, err)
	require.Equal(t, os.FileMode(0o755), info.Mode().Perm())
}

// A directory target (a copy-mode entry whose source is a directory) is
// backed up recursively: every nested file lands under the mirrored path.
func TestRun_copiesDirectoryTargetRecursively(t *testing.T) {
	tmp := t.TempDir()
	db := filepath.Join(tmp, "home", "cri", ".config", "app", "db")
	require.NoError(t, os.MkdirAll(filepath.Join(db, "presets"), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(db, "easyeffectsrc"), []byte("root"), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(db, "presets", "p.json"), []byte("preset"), 0o644))

	moduleDir := filepath.Join(tmp, "modules", "app")
	n, err := backup.Run([]backup.File{{Target: db, ModuleDir: moduleDir}}, "g1")
	require.NoError(t, err)
	require.Equal(t, 1, n, "one directory unit")

	base := backupPath(moduleDir, "g1", db)
	for rel, want := range map[string]string{
		"easyeffectsrc":  "root",
		"presets/p.json": "preset",
	} {
		got, err := os.ReadFile(filepath.Join(base, rel))
		require.NoError(t, err, "%s must be backed up", rel)
		require.Equal(t, want, string(got))
	}
}

// A target that is a symlink (e.g. left from a mode switch) is backed up as
// the content it resolves to — the data mise would overwrite.
func TestRun_followsSymlinkedTarget(t *testing.T) {
	tmp := t.TempDir()
	real := filepath.Join(tmp, "real.conf")
	require.NoError(t, os.WriteFile(real, []byte("payload"), 0o644))
	link := filepath.Join(tmp, "home", "cri", ".config", "app", "config.toml")
	require.NoError(t, os.MkdirAll(filepath.Dir(link), 0o755))
	require.NoError(t, os.Symlink(real, link))

	moduleDir := filepath.Join(tmp, "modules", "app")
	_, err := backup.Run([]backup.File{{Target: link, ModuleDir: moduleDir}}, "g1")
	require.NoError(t, err)

	got, err := os.ReadFile(backupPath(moduleDir, "g1", link))
	require.NoError(t, err)
	require.Equal(t, "payload", string(got))
}

// An unreadable target is an error, not a skip: the caller asked for a
// backup before overwrite, so losing the file silently must never happen.
func TestRun_unreadableTargetFails(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("root ignores file permissions")
	}
	tmp := t.TempDir()
	live := filepath.Join(tmp, "secret.conf")
	require.NoError(t, os.WriteFile(live, []byte("x"), 0o600))
	require.NoError(t, os.Chmod(live, 0o000))
	t.Cleanup(func() { _ = os.Chmod(live, 0o600) })

	_, err := backup.Run([]backup.File{{Target: live, ModuleDir: filepath.Join(tmp, "modules", "m")}}, "g1")
	require.Error(t, err)
}
