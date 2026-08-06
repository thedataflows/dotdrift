package cmd_test

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
	require.Contains(t, out, "selected b\n", "app column is omitted when app == id")
	require.Contains(t, out, "skipped  a disabled")
}

// A module with an explicit app different from its id keeps the app
// visible, tagged; system-scope modules carry the [system] tag.
func TestCLI_modules_appAndScopeTags(t *testing.T) {
	var buf bytes.Buffer
	c := &cmd.ModulesCmd{
		Profile: filepath.Join("..", "testdata", "profiles", "appname"),
		Out:     &buf,
	}
	require.NoError(t, c.Run())
	require.Contains(t, buf.String(), "selected foo (app: FooApp)\n")

	buf.Reset()
	c = &cmd.ModulesCmd{
		Profile: filepath.Join("..", "testdata", "profiles", "scope"),
		Out:     &buf,
	}
	require.NoError(t, c.Run())
	out := buf.String()
	require.Contains(t, out, "selected demo [system]\n")
	require.Contains(t, out, "selected shell\n", "user-scope modules render without a scope tag")
	require.NotContains(t, out, "selected shell [system]")
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
	require.Contains(t, out, "selected b\n")
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
	require.Contains(t, out, "selected shell\n")
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
