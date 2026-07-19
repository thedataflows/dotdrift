package dotdrift

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestProfilingListenOn_defaultIsLoopback(t *testing.T) {
	field, ok := reflect.TypeOf(RootFlags{}).FieldByName("ProfilingListenOn")
	require.True(t, ok, "RootFlags.ProfilingListenOn must exist")
	require.Equal(t, "127.0.0.1:6060", field.Tag.Get("default"),
		"pprof must not bind to all interfaces by default")
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
