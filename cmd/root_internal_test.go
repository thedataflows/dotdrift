package cmd

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/alecthomas/kong"
	"github.com/stretchr/testify/require"
)

func TestProfilingListenOn_defaultIsLoopback(t *testing.T) {
	field, ok := reflect.TypeOf(RootFlags{}).FieldByName("ProfilingListenOn")
	require.True(t, ok, "RootFlags.ProfilingListenOn must exist")
	require.Equal(t, "127.0.0.1:6060", field.Tag.Get("default"),
		"pprof must not bind to all interfaces by default")
}

// Positional module-filter args land on the command's Modules slice as raw
// argv tokens; comma splitting happens later in profile.ParseModuleFilter.
func TestKong_applyPositionalModules(t *testing.T) {
	var cli CLI
	parser, err := kong.New(&cli)
	require.NoError(t, err)
	_, err = parser.Parse([]string{"apply", "vim,git", "extra"})
	require.NoError(t, err)
	require.Equal(t, []string{"vim,git", "extra"}, cli.Apply.Modules)
}

// "dotdrift add" is an alias for "dotdrift onboard": it parses onto the
// same OnboardCmd and carries the same flags.
func TestKong_addAliasRoutesToOnboard(t *testing.T) {
	var cli CLI
	parser, err := kong.New(&cli, kong.Name(appName))
	require.NoError(t, err)
	_, err = parser.Parse([]string{"add", "/tmp/x", "--app", "foo"})
	require.NoError(t, err)
	require.Equal(t, []string{"/tmp/x"}, cli.Onboard.Paths)
	require.Equal(t, "foo", cli.Onboard.App)
}

// --verbose parses on apply and onboard (and only there).
func TestKong_verboseFlagParses(t *testing.T) {
	var cli CLI
	parser, err := kong.New(&cli)
	require.NoError(t, err)
	_, err = parser.Parse([]string{"apply", "--verbose"})
	require.NoError(t, err)
	require.True(t, cli.Apply.Verbose)
	require.False(t, cli.Onboard.Verbose)

	cli = CLI{}
	parser, err = kong.New(&cli)
	require.NoError(t, err)
	_, err = parser.Parse([]string{"onboard", "--verbose"})
	require.NoError(t, err)
	require.True(t, cli.Onboard.Verbose)
	require.False(t, cli.Apply.Verbose)
}

// kong's DefaultEnvars("DD") maps DD_VERBOSE onto --verbose with no extra
// code.
func TestKong_verboseDefaultEnvar(t *testing.T) {
	t.Setenv("DD_VERBOSE", "1")
	var cli CLI
	parser, err := kong.New(&cli, kong.DefaultEnvars("DD"))
	require.NoError(t, err)
	_, err = parser.Parse([]string{"apply"})
	require.NoError(t, err)
	require.True(t, cli.Apply.Verbose, "DD_VERBOSE=1 must enable apply --verbose via DefaultEnvars")
}

// -v is the short alias for --verbose on apply and onboard (and collides
// with nothing: no other short flags exist, and version is a subcommand).
func TestKong_verboseShortFlagParses(t *testing.T) {
	var cli CLI
	parser, err := kong.New(&cli)
	require.NoError(t, err)
	_, err = parser.Parse([]string{"apply", "-v"})
	require.NoError(t, err)
	require.True(t, cli.Apply.Verbose, "-v must alias --verbose on apply")
	require.False(t, cli.Onboard.Verbose)

	cli = CLI{}
	parser, err = kong.New(&cli)
	require.NoError(t, err)
	_, err = parser.Parse([]string{"onboard", "-v"})
	require.NoError(t, err)
	require.True(t, cli.Onboard.Verbose, "-v must alias --verbose on onboard")
	require.False(t, cli.Apply.Verbose)
}

func TestLoadDotenvFiles_loadsEnvFiles(t *testing.T) {
	const marker = "DOTDRIFT_TEST_ROOT_MARKER"
	require.NoError(t, os.Unsetenv(marker))
	t.Cleanup(func() { _ = os.Unsetenv(marker) })

	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, ".env"), []byte(marker+"=loaded\n"), 0o600))
	t.Chdir(dir)

	loadDotenvFiles()
	require.Equal(t, "loaded", os.Getenv(marker))
}

func TestLoadDotenvFiles_optOut(t *testing.T) {
	const marker = "DOTDRIFT_TEST_ROOT_MARKER"
	require.NoError(t, os.Unsetenv(marker))
	t.Cleanup(func() { _ = os.Unsetenv(marker) })
	t.Setenv("DOTDRIFT_NO_ENV", "1")

	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, ".env"), []byte(marker+"=loaded\n"), 0o600))
	t.Chdir(dir)

	loadDotenvFiles()
	require.Empty(t, os.Getenv(marker), "DOTDRIFT_NO_ENV=1 must skip .env loading entirely")
}
