package profile_test

import (
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/thedataflows/dotdrift/internal/facts"
	"github.com/thedataflows/dotdrift/internal/profile"
)

func TestDiscover_hostOnlyModuleSelected(t *testing.T) {
	p, err := profile.Load(fixture(t, "overlayonly"), &facts.Facts{Hostname: "myhost"})
	require.NoError(t, err)
	require.Contains(t, selectedIDs(p), "hostonly")
	require.Contains(t, selectedIDs(p), "both")
	require.NotContains(t, selectedIDs(p), "useronly")
}

func TestDiscover_userOnlyModuleSelected(t *testing.T) {
	p, err := profile.Load(fixture(t, "overlayonly"), &facts.Facts{Username: "cri"})
	require.NoError(t, err)
	require.Contains(t, selectedIDs(p), "useronly")
	require.Contains(t, selectedIDs(p), "both")
	require.NotContains(t, selectedIDs(p), "hostonly")
}

func TestDiscover_overlayOfBaseNotDuplicated(t *testing.T) {
	p, err := profile.Load(fixture(t, "overlayonly"), &facts.Facts{Hostname: "myhost", Username: "cri"})
	require.NoError(t, err)
	count := 0
	for _, m := range p.Modules {
		if m.ID == "both" {
			count++
		}
	}
	require.Equal(t, 1, count, "base module + host overlay must yield exactly one Module entry")
}

func TestDiscover_basePreferredRepresentative(t *testing.T) {
	root := fixture(t, "overlayonly")
	p, err := profile.Load(root, &facts.Facts{Hostname: "myhost", Username: "cri"})
	require.NoError(t, err)
	m := findModule(t, p, "both")
	require.Equal(t, filepath.Join(root, "modules", "both"), m.Path)
	require.Equal(t, "base-both", m.App)
	require.Equal(t, []string{"base-pkg"}, m.Config.Packages.Present)
}

func TestDiscover_conflictingIDsAcrossLayersStillFatal(t *testing.T) {
	_, err := profile.Load(fixture(t, "overlayonly"), &facts.Facts{Hostname: "evilhost"})
	require.Error(t, err)
	require.Contains(t, err.Error(), `"both"`)
	require.Contains(t, err.Error(), filepath.Join("modules", "both"))
	require.Contains(t, err.Error(), filepath.Join("hosts", "evilhost", "modules", "shadow"))
	t.Logf("error: %v", err)
}

func TestDiscover_missingOverlayDirsTolerated(t *testing.T) {
	p, err := profile.Load(fixture(t, "simple"), &facts.Facts{Hostname: "myhost", Username: "cri"})
	require.NoError(t, err)
	require.Len(t, p.Modules, 2)
}
