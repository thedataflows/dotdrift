package resolve_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/thedataflows/dotdrift/internal/facts"
	"github.com/thedataflows/dotdrift/internal/profile"
	"github.com/thedataflows/dotdrift/internal/resolve"
)

func writeBaseModule(t *testing.T, root, dir, moduleTOML string) string {
	t.Helper()
	d := filepath.Join(root, "modules", dir)
	require.NoError(t, os.MkdirAll(d, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(d, "module.toml"), []byte(moduleTOML), 0o644))
	return d
}

// A multi-word package entry (a description pasted where a name belongs)
// must fail at resolve time, naming the module, the layer, and the config
// file, instead of reaching mise as an opaque paru hook error (0152).
func TestResolve_invalidPackageName_namesModuleLayerAndConfig(t *testing.T) {
	root := t.TempDir()
	dir := writeBaseModule(t, root, "shell", "id = \"shell\"\n[packages]\npresent = [\"A modern compatibility tools manager\"]\n")

	f := &facts.Facts{Hostname: "myhost", Username: "cri"}
	p, err := profile.Load(root, f)
	require.NoError(t, err)
	_, err = resolve.Resolve(p, f)
	require.Error(t, err)
	require.Contains(t, err.Error(), "shell")
	require.Contains(t, err.Error(), "base")
	require.Contains(t, err.Error(), filepath.Join(dir, "module.toml"))
	require.Contains(t, err.Error(), "A modern compatibility tools manager")
}

func TestResolve_invalidPackageNameInHostOverlay_namesHostConfig(t *testing.T) {
	root := t.TempDir()
	writeBaseModule(t, root, "shell", "id = \"shell\"\n[packages]\npresent = [\"ripgrep\"]\n")
	overlay := writeOverlayModule(t, root, "hosts", "myhost", "shell", "[packages]\nabsent = [\"two words\"]\n")

	f := &facts.Facts{Hostname: "myhost", Username: "cri"}
	p, err := profile.Load(root, f)
	require.NoError(t, err)
	_, err = resolve.Resolve(p, f)
	require.Error(t, err)
	require.Contains(t, err.Error(), "host")
	require.Contains(t, err.Error(), filepath.Join(overlay, "module.toml"))
	require.Contains(t, err.Error(), "two words")
}
