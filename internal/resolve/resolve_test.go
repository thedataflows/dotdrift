package resolve_test

import (
	"path/filepath"
	"sort"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/thedataflows/dotdrift/internal/facts"
	"github.com/thedataflows/dotdrift/internal/profile"
	"github.com/thedataflows/dotdrift/internal/resolve"
)

func fixture(t *testing.T, name string) string {
	t.Helper()
	return filepath.Join("..", "..", "testdata", "profiles", name)
}

func selectedIDs(p *profile.Profile) []string {
	ids := make([]string, len(p.Selected))
	for i, m := range p.Selected {
		ids[i] = m.ID
	}
	return ids
}

func TestMergePackages_userAbsentBeatsHostPresent(t *testing.T) {
	p, err := profile.Load(fixture(t, "resolve"), &facts.Facts{
		Hostname: "myhost",
		Username: "cri",
		OS:       "linux",
	})
	require.NoError(t, err)
	require.Contains(t, selectedIDs(p), "shell")

	plan, err := resolve.Resolve(p, &facts.Facts{Hostname: "myhost", Username: "cri"})
	require.NoError(t, err)

	require.NotContains(t, plan.Packages.Install, "nano", "user absent should remove nano")
	require.Contains(t, plan.Packages.Install, "neovim", "user present should restore neovim")
	require.Contains(t, plan.Packages.Install, "ripgrep", "base present should survive")
	require.Contains(t, plan.Packages.Install, "fd", "host present should survive")
	require.Contains(t, plan.Packages.Install, "eza", "user present should be added")
	require.NotContains(t, plan.Packages.Install, "emacs", "base absent should stay removed")
}

func TestMergePackages_presentIdempotent(t *testing.T) {
	p, err := profile.Load(fixture(t, "resolve"), &facts.Facts{
		Hostname: "myhost",
		Username: "cri",
		OS:       "linux",
	})
	require.NoError(t, err)

	plan, err := resolve.Resolve(p, &facts.Facts{Hostname: "myhost", Username: "cri"})
	require.NoError(t, err)

	sort.Strings(plan.Packages.Install)
	for i := 1; i < len(plan.Packages.Install); i++ {
		require.NotEqual(t, plan.Packages.Install[i-1], plan.Packages.Install[i], "duplicate package %q", plan.Packages.Install[i])
	}
}

func TestMergeDotfiles_userWinsSameTarget(t *testing.T) {
	p, err := profile.Load(fixture(t, "resolve"), &facts.Facts{
		Hostname: "myhost",
		Username: "cri",
		OS:       "linux",
	})
	require.NoError(t, err)

	plan, err := resolve.Resolve(p, &facts.Facts{Hostname: "myhost", Username: "cri"})
	require.NoError(t, err)

	var bashrc *resolve.DotfileEntry
	for i := range plan.Dotfiles.Entries {
		if plan.Dotfiles.Entries[i].Target == "~/.bashrc" {
			bashrc = &plan.Dotfiles.Entries[i]
			break
		}
	}
	require.NotNil(t, bashrc, "~/.bashrc entry should exist")
	require.Equal(t, "copy", bashrc.Mode, "user mode should win")
	require.Equal(t, "user", bashrc.Layer, "user layer should win")
}

func TestMergeTools_userWins(t *testing.T) {
	p, err := profile.Load(fixture(t, "resolve"), &facts.Facts{
		Hostname: "myhost",
		Username: "cri",
		OS:       "linux",
	})
	require.NoError(t, err)

	plan, err := resolve.Resolve(p, &facts.Facts{Hostname: "myhost", Username: "cri"})
	require.NoError(t, err)

	require.Equal(t, "22", plan.Tools.Versions["node"], "host override should win for node")
	require.Equal(t, "3.12", plan.Tools.Versions["python"], "user override should win for python")
}

func TestFileOverlay_userReplacesHost(t *testing.T) {
	p, err := profile.Load(fixture(t, "resolve"), &facts.Facts{
		Hostname: "myhost",
		Username: "cri",
		OS:       "linux",
	})
	require.NoError(t, err)

	plan, err := resolve.Resolve(p, &facts.Facts{Hostname: "myhost", Username: "cri"})
	require.NoError(t, err)

	var bashrc *resolve.DotfileEntry
	for i := range plan.Dotfiles.Entries {
		if plan.Dotfiles.Entries[i].Target == "~/.bashrc" {
			bashrc = &plan.Dotfiles.Entries[i]
			break
		}
	}
	require.NotNil(t, bashrc, "~/.bashrc entry should exist")
	require.True(t, strings.Contains(bashrc.Source, "users/cri/modules/shell"), "source file should be resolved from user layer: %s", bashrc.Source)
}

func TestSelectionFingerprint_stable(t *testing.T) {
	f1 := &facts.Facts{Hostname: "myhost"}
	p1, err := profile.Load(fixture(t, "whenfilter"), f1)
	require.NoError(t, err)

	fp1 := resolve.Fingerprint(p1, f1)
	fp2 := resolve.Fingerprint(p1, f1)
	require.Equal(t, fp1, fp2, "fingerprint should be stable for the same inputs")

	f2 := &facts.Facts{Username: "cri"}
	p2, err := profile.Load(fixture(t, "whenfilter"), f2)
	require.NoError(t, err)
	fp3 := resolve.Fingerprint(p2, f2)
	require.NotEqual(t, fp1, fp3, "different selection should produce different fingerprint")
}

func TestResolve_emptyProfile(t *testing.T) {
	plan, err := resolve.Resolve(&profile.Profile{}, &facts.Facts{})
	require.NoError(t, err)
	require.Empty(t, plan.Packages.Install)
	require.Empty(t, plan.Tools.Versions)
	require.Empty(t, plan.Dotfiles.Entries)
}
