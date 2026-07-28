package tui

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/thedataflows/dotdrift/internal/generate"
)

// ExistingMountSources: sorted sources from the target module, nil for an
// absent module, decode errors propagated (fail loud, never swallowed).
func TestExistingMountSources(t *testing.T) {
	root := t.TempDir()
	sel := generate.Selection{Layer: generate.LayerBase, ModuleID: "media"}

	sources, err := ExistingMountSources(root, sel)
	require.NoError(t, err)
	require.Nil(t, sources, "absent module yields nil, nil")

	dir, err := generate.ModuleDir(root, sel)
	require.NoError(t, err)
	require.NoError(t, os.MkdirAll(dir, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "module.toml"), []byte(`
[mounts.zeta]
source = "UUID=z"
destination = "/mnt/z"
type = "btrfs"

[mounts.alpha]
source = "server:/a"
destination = "/mnt/a"
type = "nfs"
`), 0o644))

	sources, err = ExistingMountSources(root, sel)
	require.NoError(t, err)
	require.Equal(t, []string{"server:/a", "UUID=z"}, sources, "sources sorted by mount name")

	require.NoError(t, os.WriteFile(filepath.Join(dir, "module.toml"), []byte("not = [valid"), 0o644))
	_, err = ExistingMountSources(root, sel)
	require.Error(t, err, "malformed module.toml is a loud error, not swallowed")
	require.Contains(t, err.Error(), "module.toml")
}

// PrintSummary: the materialized module dir and its files, name-sorted,
// written to the caller's writer (stdout for CLI, stderr for the wizard).
func TestPrintSummary(t *testing.T) {
	root := t.TempDir()
	sel := generate.Selection{Layer: generate.LayerBase, ModuleID: "media"}
	dir, err := generate.ModuleDir(root, sel)
	require.NoError(t, err)

	var out bytes.Buffer
	require.Error(t, PrintSummary(&out, root, sel), "missing module dir is an error")

	require.NoError(t, os.MkdirAll(dir, 0o755))
	for _, f := range []string{"b.mount", "a.mount", "module.toml"} {
		require.NoError(t, os.WriteFile(filepath.Join(dir, f), []byte("x"), 0o644))
	}
	require.NoError(t, PrintSummary(&out, root, sel))
	require.Equal(t, "wrote "+dir+":\n  a.mount\n  b.mount\n  module.toml\n", out.String())
}

// InvokingUser resolves the current OS user's numeric ids and name.
func TestInvokingUser(t *testing.T) {
	uid, gid, username, err := InvokingUser()
	require.NoError(t, err)
	require.GreaterOrEqual(t, uid, 0)
	require.GreaterOrEqual(t, gid, 0)
	require.NotEmpty(t, username)
}
