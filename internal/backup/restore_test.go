package backup_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/thedataflows/dotdrift/internal/backup"
)

// Generations lists a module's backup generation directories newest-first
// (generation names are timestamps, so lexical order is time order); a
// module without backups lists empty.
func TestGenerations_newestFirst(t *testing.T) {
	moduleDir := filepath.Join(t.TempDir(), "modules", "app")
	for _, gen := range []string{"20260823-101500", "20260824-153000"} {
		require.NoError(t, os.MkdirAll(filepath.Join(moduleDir, "backups", gen), 0o755))
	}

	gens, err := backup.Generations(moduleDir)
	require.NoError(t, err)
	require.Equal(t, []string{"20260824-153000", "20260823-101500"}, gens)

	empty, err := backup.Generations(filepath.Join(t.TempDir(), "modules", "none"))
	require.NoError(t, err)
	require.Empty(t, empty)
}

// RestoreItem is Run's inverse: the backed-up copy lands back at its live
// target, content and mode intact, and the target's existing (drifted)
// content is replaced.
func TestRestoreItem_roundtrip(t *testing.T) {
	tmp := t.TempDir()
	live := filepath.Join(tmp, "home", "cri", ".bashrc")
	require.NoError(t, os.MkdirAll(filepath.Dir(live), 0o755))
	require.NoError(t, os.WriteFile(live, []byte("precious local edits"), 0o600))
	moduleDir := filepath.Join(tmp, "modules", "shell")

	_, err := backup.Run([]backup.File{{Target: live, ModuleDir: moduleDir}}, "g1")
	require.NoError(t, err)
	genDir := filepath.Join(moduleDir, "backups", "g1")
	// Simulate the overwrite that loses the local edits, then restore.
	require.NoError(t, os.WriteFile(live, []byte("profile content"), 0o644))

	require.NoError(t, backup.RestoreItem(genDir, live))
	got, err := os.ReadFile(live)
	require.NoError(t, err)
	require.Equal(t, "precious local edits", string(got), "the backed-up content wins over the overwritten live file")
}

// A symlink sitting at the live target is removed, not followed: writing
// through it would clobber the profile file it points at.
func TestRestoreItem_replacesSymlinkAtTarget(t *testing.T) {
	tmp := t.TempDir()
	precious := filepath.Join(tmp, "profile-source")
	require.NoError(t, os.WriteFile(precious, []byte("precious"), 0o644))
	target := filepath.Join(tmp, "home", "cri", ".bashrc")
	require.NoError(t, os.MkdirAll(filepath.Dir(target), 0o755))
	require.NoError(t, os.Symlink(precious, target))

	moduleDir := filepath.Join(tmp, "modules", "shell")
	// The generation holds the target's pre-symlink content, mirrored at
	// its full absolute path (Run's layout rule).
	require.NoError(t, os.MkdirAll(filepath.Dir(backupPath(moduleDir, "g1", target)), 0o755))
	require.NoError(t, os.WriteFile(backupPath(moduleDir, "g1", target), []byte("restored"), 0o644))

	require.NoError(t, backup.RestoreItem(filepath.Join(moduleDir, "backups", "g1"), target))

	got, err := os.ReadFile(target)
	require.NoError(t, err)
	require.Equal(t, "restored", string(got))
	info, err := os.Lstat(target)
	require.NoError(t, err)
	require.Equal(t, os.FileMode(0), info.Mode()&os.ModeSymlink, "the link is gone, a real file took its place")
	kept, err := os.ReadFile(precious)
	require.NoError(t, err)
	require.Equal(t, "precious", string(kept), "the link destination must not be touched")
}

// Missing parent directories of the target are created (the live file may
// have been deleted along with its directory).
func TestRestoreItem_createsMissingParents(t *testing.T) {
	tmp := t.TempDir()
	target := filepath.Join(tmp, "live-root", "home", "cri", ".config", "app", "db", "x.conf")
	moduleDir := filepath.Join(tmp, "modules", "app")
	bk := backupPath(moduleDir, "g1", target) // mirrors the full absolute path
	require.NoError(t, os.MkdirAll(filepath.Dir(bk), 0o755))
	require.NoError(t, os.WriteFile(bk, []byte("data"), 0o600))

	require.NoError(t, backup.RestoreItem(filepath.Join(moduleDir, "backups", "g1"), target))

	got, err := os.ReadFile(target)
	require.NoError(t, err)
	require.Equal(t, "data", string(got))
}

// A target with no backup in the generation is an error naming the target;
// a relative target is rejected (it cannot map to the mirrored layout).
func TestRestoreItem_missingOrRelativeTargetFails(t *testing.T) {
	genDir := filepath.Join(t.TempDir(), "modules", "app", "backups", "g1")
	require.NoError(t, os.MkdirAll(genDir, 0o755))

	err := backup.RestoreItem(genDir, "/no/such/backup")
	require.Error(t, err)
	require.Contains(t, err.Error(), "/no/such/backup")

	err = backup.RestoreItem(genDir, "relative/path")
	require.Error(t, err)
}
