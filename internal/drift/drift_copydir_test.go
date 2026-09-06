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

// Whole-directory copy targets compare trees (issue 0035): previously the
// probe read the source with ReadFile, failed with "is a directory", and
// reported unknown forever.

// copyDirTrees writes a source tree and a target tree from the given file
// maps (rel path -> content).
func copyDirTrees(t *testing.T, srcFiles, tgtFiles map[string]string) (src, tgt string) {
	t.Helper()
	src = filepath.Join(t.TempDir(), "src")
	tgt = filepath.Join(t.TempDir(), "tgt")
	for _, pair := range []struct {
		dir   string
		files map[string]string
	}{{src, srcFiles}, {tgt, tgtFiles}} {
		for rel, content := range pair.files {
			p := filepath.Join(pair.dir, rel)
			require.NoError(t, os.MkdirAll(filepath.Dir(p), 0o755))
			require.NoError(t, os.WriteFile(p, []byte(content), 0o644))
		}
	}
	return src, tgt
}

func copyDirCheck(t *testing.T, src, tgt string) []drift.Finding {
	t.Helper()
	pr := fakeProbes()
	pr.HomeDir = "/home/u"
	return drift.Check(context.Background(),
		dotfilePlan(resolve.DotfileEntry{Target: tgt, Source: src, Mode: "copy"}), filepath.Dir(src), pr, drift.CheckOptions{})
}

func TestCheck_dotfilesCopyDirEqual(t *testing.T) {
	src, tgt := copyDirTrees(t,
		map[string]string{"a.conf": "x", "sub/b.conf": "y"},
		map[string]string{"a.conf": "x", "sub/b.conf": "y"})

	fs := copyDirCheck(t, src, tgt)
	require.Equal(t, drift.OK, fs[0].Status)
}

func TestCheck_dotfilesCopyDirDiffers(t *testing.T) {
	src, tgt := copyDirTrees(t,
		map[string]string{"a.conf": "new", "sub/b.conf": "y"},
		map[string]string{"a.conf": "old", "sub/b.conf": "y"})

	fs := copyDirCheck(t, src, tgt)
	require.Equal(t, drift.Drift, fs[0].Status)
	require.Equal(t, "1 file(s) differ", fs[0].Detail)
}

func TestCheck_dotfilesCopyDirMissingFiles(t *testing.T) {
	src, tgt := copyDirTrees(t,
		map[string]string{"a.conf": "x", "sub/b.conf": "y"},
		map[string]string{"a.conf": "x"}) // sub/b.conf absent at target

	fs := copyDirCheck(t, src, tgt)
	require.Equal(t, drift.Drift, fs[0].Status)
	require.Equal(t, "1 file(s) missing", fs[0].Detail)
}

func TestCheck_dotfilesCopyDirMissingAndDiffer(t *testing.T) {
	src, tgt := copyDirTrees(t,
		map[string]string{"a.conf": "new", "b.conf": "y"},
		map[string]string{"a.conf": "old"})

	fs := copyDirCheck(t, src, tgt)
	require.Equal(t, drift.Drift, fs[0].Status)
	require.Equal(t, "1 file(s) missing, 1 file(s) differ", fs[0].Detail)
}

func TestCheck_dotfilesCopyDirMissingTarget(t *testing.T) {
	src, tgt := copyDirTrees(t,
		map[string]string{"a.conf": "x"},
		nil)
	require.NoError(t, os.RemoveAll(tgt)) // target dir absent entirely

	fs := copyDirCheck(t, src, tgt)
	require.Equal(t, drift.Drift, fs[0].Status)
	require.Equal(t, "missing", fs[0].Detail)
}

func TestCheck_dotfilesCopyDirTargetOnlyExtrasIgnored(t *testing.T) {
	src, tgt := copyDirTrees(t,
		map[string]string{"a.conf": "x"},
		map[string]string{"a.conf": "x", "extra.conf": "target only"})

	fs := copyDirCheck(t, src, tgt)
	require.Equal(t, drift.OK, fs[0].Status, "copy never deletes: target-only files are not drift")
}
