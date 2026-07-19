package profile_test

import (
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/thedataflows/dotdrift/internal/facts"
	"github.com/thedataflows/dotdrift/internal/profile"
)

// The shipped examples must stay loadable: parse, discover, and select
// without error under plausible facts.
func TestExamples_simpleLoads(t *testing.T) {
	root := filepath.Join("..", "..", "examples", "simple")
	p, err := profile.Load(root, &facts.Facts{Hostname: "myhost", Username: "cri", OS: "linux"})
	require.NoError(t, err)
	require.Contains(t, selectedIDs(p), "shell")
}

func TestExamples_profileLoads(t *testing.T) {
	root := filepath.Join("..", "..", "examples", "profile")
	// examples/profile ships hosts/myhost and users/cri overlays.
	p, err := profile.Load(root, &facts.Facts{Hostname: "myhost", Username: "cri", OS: "linux"})
	require.NoError(t, err)
	require.Contains(t, selectedIDs(p), "bash")
	require.Contains(t, selectedIDs(p), "nvim")
}
