package profile_test

// Tolerant discovery (M15, T-tui-workspace): the interactive shell must
// stay up when one module.toml is broken — the broken module surfaces as
// a skip naming the error, and the workspace opens the file read-only
// with its raw text. Strict Load keeps failing hard: plan/apply must
// never silently skip a module the user wrote.

import (
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/thedataflows/dotdrift/internal/facts"
	"github.com/thedataflows/dotdrift/internal/profile"
)

func TestLoadTolerant_brokenModuleSurfacesAsSkip(t *testing.T) {
	root := t.TempDir()
	writeModule(t, root, "modules/demo", "id = \"demo\"\n[broken\n")
	writeModule(t, root, "modules/ok", "id = \"ok\"\n")

	p, err := profile.LoadTolerant(root, &facts.Facts{})
	require.NoError(t, err, "one broken module must not kill the interactive load")
	require.Contains(t, selectedIDs(p), "ok", "healthy modules still load")
	require.NotContains(t, selectedIDs(p), "demo")

	require.Len(t, p.Skipped, 1)
	s := p.Skipped[0]
	require.Equal(t, "demo", s.Module.ID, "the skip names the broken module by its dir")
	require.Equal(t, filepath.Join(root, "modules", "demo"), s.Module.Path)
	require.Contains(t, s.Reason, "invalid module.toml", "the reason says why")
}

func TestLoadTolerant_brokenOverlayKeepsBaseModule(t *testing.T) {
	root := t.TempDir()
	writeModule(t, root, "modules/demo", "id = \"demo\"\n")
	writeModule(t, root, "users/cri/modules/demo", "id = \"demo\"\n[broken\n")

	p, err := profile.LoadTolerant(root, &facts.Facts{Username: "cri"})
	require.NoError(t, err)
	require.Contains(t, selectedIDs(p), "demo", "a broken overlay does not sink the base module")
	require.Len(t, p.Skipped, 1)
	require.Contains(t, p.Skipped[0].Reason, "invalid module.toml")
	require.Contains(t, p.Skipped[0].Module.Path, filepath.Join("users", "cri"))
}

func TestLoad_brokenModuleStillFatal(t *testing.T) {
	root := t.TempDir()
	writeModule(t, root, "modules/demo", "id = \"demo\"\n[broken\n")

	_, err := profile.Load(root, &facts.Facts{})
	require.Error(t, err, "strict Load (plan/apply) never silently skips a broken module")
}
