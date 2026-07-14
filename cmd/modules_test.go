package dotdrift_test

import (
	"bytes"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
	cmd "github.com/thedataflows/dotdrift/cmd"
)

func TestCLI_modules_listsStatus(t *testing.T) {
	var buf bytes.Buffer
	c := &cmd.ModulesCmd{
		Profile: filepath.Join("..", "testdata", "profiles", "disabled"),
		Out:     &buf,
	}
	err := c.Run()
	require.NoError(t, err)
	out := buf.String()
	require.Contains(t, out, "selected b b")
	require.Contains(t, out, "skipped  a disabled")
}
