package drift_test

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/thedataflows/dotdrift/internal/drift"
	"github.com/thedataflows/dotdrift/internal/resolve"
)

// Symlink/symlink-each sources must still exist: a target link that points
// at the right path but whose source file is gone is DRIFT (a dangling
// link), not OK. Same for a stale symlink-each child left behind after its
// source file was removed from the source directory.

// A symlink entry whose target link points correctly but whose source file
// was deleted is drift ("source missing"), not OK.
func TestCheck_dotfilesSymlinkSourceMissing(t *testing.T) {
	root := t.TempDir()
	src := filepath.Join(root, "bashrc")
	require.NoError(t, os.WriteFile(src, []byte("x"), 0o644))
	tgt := filepath.Join(t.TempDir(), "bashrc")
	require.NoError(t, os.Symlink(src, tgt))
	require.NoError(t, os.Remove(src)) // source deleted after the link was made

	pr := fakeProbes()
	pr.HomeDir = "/home/u"
	fs := drift.Check(context.Background(), dotfilePlan(resolve.DotfileEntry{Target: tgt, Source: src, Mode: "symlink"}), root, pr, drift.CheckOptions{})
	require.Len(t, fs, 1)
	require.Equal(t, drift.Drift, fs[0].Status)
	require.Contains(t, fs[0].Detail, "source missing")
	t.Logf("finding: %+v", fs[0])
}

// A healthy symlink stays OK.
func TestCheck_dotfilesSymlinkSourcePresentStaysOK(t *testing.T) {
	root := t.TempDir()
	src := filepath.Join(root, "bashrc")
	require.NoError(t, os.WriteFile(src, []byte("x"), 0o644))
	tgt := filepath.Join(t.TempDir(), "bashrc")
	require.NoError(t, os.Symlink(src, tgt))

	pr := fakeProbes()
	pr.HomeDir = "/home/u"
	fs := drift.Check(context.Background(), dotfilePlan(resolve.DotfileEntry{Target: tgt, Source: src, Mode: "symlink"}), root, pr, drift.CheckOptions{})
	require.Len(t, fs, 1)
	require.Equal(t, drift.OK, fs[0].Status)
}

// A stale symlink-each child — a target-dir link pointing into the source
// dir whose source file no longer exists — is drift ("stale link"). The
// live child (source still present) stays OK, and a plain user file in the
// target dir is not flagged.
func TestCheck_dotfilesSymlinkEachStaleChild(t *testing.T) {
	root := t.TempDir()
	srcDir := filepath.Join(root, "units")
	require.NoError(t, os.MkdirAll(srcDir, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(srcDir, "a.mount"), []byte("x"), 0o644))
	tgtDir := filepath.Join(t.TempDir(), "system")
	require.NoError(t, os.MkdirAll(tgtDir, 0o755))
	require.NoError(t, os.Symlink(filepath.Join(srcDir, "a.mount"), filepath.Join(tgtDir, "a.mount")))
	// Stale child: link into the source dir, but the source file is gone.
	require.NoError(t, os.Symlink(filepath.Join(srcDir, "removed.mount"), filepath.Join(tgtDir, "removed.mount")))
	// Unrelated user file: must not be flagged.
	require.NoError(t, os.WriteFile(filepath.Join(tgtDir, "local.conf"), []byte("y"), 0o644))

	pr := fakeProbes()
	pr.HomeDir = "/home/u"
	fs := drift.Check(context.Background(), dotfilePlan(resolve.DotfileEntry{Target: tgtDir, Source: "units", Mode: "symlink-each"}), root, pr, drift.CheckOptions{})

	a := findByItem(t, fs, filepath.Join(tgtDir, "a.mount"))
	require.Equal(t, drift.OK, a.Status, "live child stays OK")

	stale := findByItem(t, fs, filepath.Join(tgtDir, "removed.mount"))
	require.Equal(t, drift.Drift, stale.Status)
	require.Contains(t, stale.Detail, "stale link")
	t.Logf("stale finding: %+v", stale)

	for _, f := range fs {
		require.NotContains(t, f.Item, "local.conf", "non-symlink files in the target dir are not the module's")
	}
}

// A stale link that points OUTSIDE the module's source dir is not ours to
// flag — only links into the source dir are module-managed.
func TestCheck_dotfilesSymlinkEachStaleChildForeignLinkIgnored(t *testing.T) {
	root := t.TempDir()
	srcDir := filepath.Join(root, "units")
	require.NoError(t, os.MkdirAll(srcDir, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(srcDir, "a.mount"), []byte("x"), 0o644))
	tgtDir := filepath.Join(t.TempDir(), "system")
	require.NoError(t, os.MkdirAll(tgtDir, 0o755))
	require.NoError(t, os.Symlink(filepath.Join(srcDir, "a.mount"), filepath.Join(tgtDir, "a.mount")))
	// Dangling link to somewhere else entirely (points to a missing file
	// outside the source dir) — not a dotdrift-managed link.
	require.NoError(t, os.Symlink("/gone/elsewhere.conf", filepath.Join(tgtDir, "foreign.conf")))

	pr := fakeProbes()
	pr.HomeDir = "/home/u"
	fs := drift.Check(context.Background(), dotfilePlan(resolve.DotfileEntry{Target: tgtDir, Source: "units", Mode: "symlink-each"}), root, pr, drift.CheckOptions{})
	require.Len(t, fs, 1, "only the live child; the foreign link is ignored")
	require.Equal(t, drift.OK, fs[0].Status)
}

// Missing probes degrade instead of panicking: nil Stat skips the dangling
// check, nil ListDir skips the stale-child scan.
func TestCheck_sourceProbesNilGuard(t *testing.T) {
	root := t.TempDir()
	src := filepath.Join(root, "bashrc")
	require.NoError(t, os.WriteFile(src, []byte("x"), 0o644))
	tgt := filepath.Join(t.TempDir(), "bashrc")
	require.NoError(t, os.Symlink(src, tgt))
	require.NoError(t, os.Remove(src))

	pr := fakeProbes()
	pr.Stat = nil
	pr.HomeDir = "/home/u"
	fs := drift.Check(context.Background(), dotfilePlan(resolve.DotfileEntry{Target: tgt, Source: src, Mode: "symlink"}), root, pr, drift.CheckOptions{})
	require.Len(t, fs, 1, "no panic, no extra findings")

	srcDir := filepath.Join(root, "units")
	require.NoError(t, os.MkdirAll(srcDir, 0o755))
	pr2 := fakeProbes()
	pr2.ListDir = nil
	pr2.HomeDir = "/home/u"
	// symlink-each against a MISSING source dir (nope): expansion error is
	// still reported; nil ListDir must not panic.
	fs = drift.Check(context.Background(), dotfilePlan(resolve.DotfileEntry{Target: tgt, Source: "nope", Mode: "symlink-each"}), root, pr2, drift.CheckOptions{})
	require.NotEmpty(t, fs, "expansion-error finding still emitted; no panic")
}
