package profile_test

import (
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/thedataflows/dotdrift/internal/facts"
	"github.com/thedataflows/dotdrift/internal/profile"
)

// The shipped examples must stay loadable: parse, discover, and select
// without error under plausible facts. The conditional example exercises
// the full [when] expression grammar (leaves + or/not + installed-state
// probes); explicit facts keep the test hermetic — provided facts are
// never overwritten by a probe, so no real package-manager/mise query
// runs.
func TestExamples_simpleLoads(t *testing.T) {
	root := filepath.Join("..", "..", "examples", "simple")
	p, err := profile.Load(root, &facts.Facts{Hostname: "myhost", Username: "cri", OS: "linux"})
	require.NoError(t, err)
	require.Contains(t, selectedIDs(p), "shell")
}

func TestExamples_simpleConditional(t *testing.T) {
	root := filepath.Join("..", "..", "examples", "simple")
	base := facts.Facts{
		Hostname: "myhost", Username: "cri", OS: "linux", Kernel: "6.12.1",
		GPU: "nvidia",
		// Non-nil empty/false maps: explicit facts, no probing.
		InstalledPackages: map[string]bool{"nvidia-dkms": false},
		InstalledTools:    map[string]bool{"cuda": true},
	}
	p, err := profile.Load(root, &base)
	require.NoError(t, err)
	require.Contains(t, selectedIDs(p), "conditional", "kernel leaf, not-leaf, and or-branch all hold")

	notSatisfied := base
	notSatisfied.InstalledPackages = map[string]bool{"nvidia-dkms": true}
	p, err = profile.Load(root, &notSatisfied)
	require.NoError(t, err)
	require.Contains(t, skippedIDs(p), "conditional", "not = { packages = [...] } negated")

	oldKernel := base
	oldKernel.Kernel = "5.10.0"
	p, err = profile.Load(root, &oldKernel)
	require.NoError(t, err)
	require.Contains(t, skippedIDs(p), "conditional", "kernel leaf fails")
}

func TestExamples_profileLoads(t *testing.T) {
	root := filepath.Join("..", "..", "examples", "profile")
	// examples/profile ships hosts/myhost and users/cri overlays.
	p, err := profile.Load(root, &facts.Facts{Hostname: "myhost", Username: "cri", OS: "linux"})
	require.NoError(t, err)
	require.Contains(t, selectedIDs(p), "bash")
	require.Contains(t, selectedIDs(p), "nvim")
}
