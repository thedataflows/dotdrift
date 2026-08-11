package cmd_test

import (
	"bytes"
	"io"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
	cmd "github.com/thedataflows/dotdrift/cmd"
	"github.com/thedataflows/dotdrift/internal/executil"
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
	require.Contains(t, out, "+ b\n", "app column is omitted when app == id")
	require.Contains(t, out, "- a disabled")
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
	require.Contains(t, buf.String(), "+ foo (app: FooApp)\n")

	buf.Reset()
	c = &cmd.ModulesCmd{
		Profile: filepath.Join("..", "testdata", "profiles", "scope"),
		Out:     &buf,
	}
	require.NoError(t, c.Run())
	out := buf.String()
	require.Contains(t, out, "+ demo [system]\n")
	require.Contains(t, out, "+ shell\n", "user-scope modules render without a scope tag")
	require.NotContains(t, out, "+ shell [system]")
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
	require.Contains(t, out, "+ b\n")
	require.Contains(t, out, "- a disabled")
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
	require.Contains(t, out, "+ shell\n")
	require.Contains(t, out, "- demo module filter")
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

// A module whose module.toml carries a description shows it after the id and
// tags, for both selected and skipped modules. The `+`/`-` markers are
// colored (green/red) only on a TTY; piped output is plain.
func TestCLI_modules_descriptionAndColor(t *testing.T) {
	dir := t.TempDir()
	demoDir := filepath.Join(dir, "modules", "demo")
	require.NoError(t, os.MkdirAll(demoDir, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(demoDir, "module.toml"), []byte(`id = "demo"
scope = "system"
description = "System-wide demo configuration"
`), 0o644))
	editDir := filepath.Join(dir, "modules", "edit")
	require.NoError(t, os.MkdirAll(editDir, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(editDir, "module.toml"), []byte(`id = "edit"
description = "Editor setup"
`), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "dotdrift.toml"), []byte("[modules]\ndisable = [\"edit\"]\n"), 0o644))

	// Piped (no TTY): plain markers, description appended after an em-dash.
	var buf bytes.Buffer
	require.NoError(t, (&cmd.ModulesCmd{Profile: dir, Out: &buf}).Run())
	out := buf.String()
	require.Contains(t, out, "+ demo [system] — System-wide demo configuration\n")
	require.Contains(t, out, "- edit disabled — Editor setup\n")
	require.NotContains(t, out, "\033[", "no ANSI codes when piped")
	// On a TTY the markers are colored; descriptions are still present.
	// Both globals are controlled: another cmd test may drive the real CLI
	// parser which latches executil.NoColor=true for the process.
	origTerm, origNoColor := executil.IsTerminal, executil.NoColor
	executil.IsTerminal = func(io.Writer) bool { return true }
	executil.NoColor = false
	t.Cleanup(func() {
		executil.IsTerminal = origTerm
		executil.NoColor = origNoColor
	})

	buf.Reset()
	require.NoError(t, (&cmd.ModulesCmd{Profile: dir, Out: &buf}).Run())
	colored := buf.String()
	require.Contains(t, colored, "\033[32m+\033[0m demo [system]")
	require.Contains(t, colored, "\033[31m-\033[0m edit disabled")
	// Description is dimmed grey, independent of the marker color.
	require.Contains(t, colored, "\033[90m— System-wide demo configuration\033[0m")
	require.Contains(t, colored, "\033[90m— Editor setup\033[0m")
}
