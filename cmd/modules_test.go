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

// A positional module filter limits the listing: the listed module stays
// selected; a module skipped for its own reason (disabled) keeps that reason.
func TestCLI_modules_filterKeepsListed(t *testing.T) {
	var buf bytes.Buffer
	c := &cmd.ModulesCmd{
		Profile: filepath.Join("..", "testdata", "profiles", "disabled"),
		Modules: []string{"b"},
		Out:     &buf,
	}
	err := c.Run()
	require.NoError(t, err)
	out := buf.String()
	require.Contains(t, out, "selected b b")
	require.Contains(t, out, "skipped  a disabled")
	require.NotContains(t, out, "module filter", "a was already disabled; the filter adds no skip")
}

// A selected-but-excluded module shows as skipped with reason "module
// filter". The scope fixture selects both demo and shell.
func TestCLI_modules_filterExcludesSelected(t *testing.T) {
	var buf bytes.Buffer
	c := &cmd.ModulesCmd{
		Profile: filepath.Join("..", "testdata", "profiles", "scope"),
		Modules: []string{"shell"},
		Out:     &buf,
	}
	err := c.Run()
	require.NoError(t, err)
	out := buf.String()
	require.Contains(t, out, "selected shell shell")
	require.Contains(t, out, "skipped  demo module filter")
}

// Unknown module ids are a loud error naming the unknown ids and the valid
// module ids.
func TestCLI_modules_filterUnknownErrors(t *testing.T) {
	var buf bytes.Buffer
	c := &cmd.ModulesCmd{
		Profile: filepath.Join("..", "testdata", "profiles", "disabled"),
		Modules: []string{"zzz"},
		Out:     &buf,
	}
	err := c.Run()
	require.Error(t, err)
	require.Contains(t, err.Error(), "zzz")
	require.Contains(t, err.Error(), "a")
	require.Contains(t, err.Error(), "b")
}

// Naming a disabled module errors, naming it and its skip reason — the
// filter never resurrects a disabled module.
func TestCLI_modules_filterDisabledErrors(t *testing.T) {
	var buf bytes.Buffer
	c := &cmd.ModulesCmd{
		Profile: filepath.Join("..", "testdata", "profiles", "disabled"),
		Modules: []string{"a"},
		Out:     &buf,
	}
	err := c.Run()
	require.Error(t, err)
	require.Contains(t, err.Error(), "a")
	require.Contains(t, err.Error(), "disabled")
}
