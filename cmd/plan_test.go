package dotdrift_test

import (
	"bytes"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
	cmd "github.com/thedataflows/dotdrift/cmd"
	"github.com/thedataflows/dotdrift/internal/facts"
)

func TestCLI_plan_output(t *testing.T) {
	var buf bytes.Buffer
	c := &cmd.PlanCmd{
		Profile: filepath.Join("..", "testdata", "profiles", "resolve"),
		Facts:   &facts.Facts{Hostname: "myhost", Username: "cri", OS: "linux"},
		Out:     &buf,
	}
	err := c.Run()
	require.NoError(t, err)

	out := buf.String()
	require.Contains(t, out, "fingerprint:")
	require.Contains(t, out, "packages:")
	require.Contains(t, out, "neovim")
	require.Contains(t, out, "tools:")
	require.Contains(t, out, "node:")
	require.Contains(t, out, "dotfiles:")
	require.Contains(t, out, "~/.bashrc")
	require.True(t, strings.Contains(out, "users/cri/modules/shell"), "plan should resolve user overlay file")
}

func TestCLI_plan_noSideEffects(t *testing.T) {
	var buf bytes.Buffer
	c := &cmd.PlanCmd{
		Profile: filepath.Join("..", "testdata", "profiles", "resolve"),
		Facts:   &facts.Facts{Hostname: "myhost", Username: "cri", OS: "linux"},
		Out:     &buf,
	}
	err := c.Run()
	require.NoError(t, err)
	require.Contains(t, buf.String(), "fingerprint:")
}
