package mise

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

// lookPathEnv isolates defaultLookPath from the host: PATH is pinned to a
// controlled dir and HOME to a fresh temp dir.
func lookPathEnv(t *testing.T) (binDir, home string) {
	t.Helper()
	binDir = t.TempDir()
	home = t.TempDir()
	t.Setenv("PATH", binDir)
	t.Setenv("HOME", home)
	return binDir, home
}

func touch(t *testing.T, path string) {
	t.Helper()
	require.NoError(t, os.MkdirAll(filepath.Dir(path), 0o755))
	require.NoError(t, os.WriteFile(path, []byte("#!/bin/sh\n"), 0o755))
}

func TestDefaultLookPath_foundInPath(t *testing.T) {
	binDir, home := lookPathEnv(t)
	want := filepath.Join(binDir, "mise")
	touch(t, want)
	// A well-known location also exists; PATH must win and skip the fallback.
	touch(t, filepath.Join(home, ".local", "bin", "mise"))

	got, err := defaultLookPath("mise")
	require.NoError(t, err)
	require.Equal(t, want, got)
}

func TestDefaultLookPath_fallbackLocalBin(t *testing.T) {
	_, home := lookPathEnv(t)
	want := filepath.Join(home, ".local", "bin", "mise")
	touch(t, want)

	got, err := defaultLookPath("mise")
	require.NoError(t, err)
	require.Equal(t, want, got)
}

func TestDefaultLookPath_fallbackShareMiseBin(t *testing.T) {
	_, home := lookPathEnv(t)
	want := filepath.Join(home, ".local", "share", "mise", "bin", "mise")
	touch(t, want)

	got, err := defaultLookPath("mise")
	require.NoError(t, err)
	require.Equal(t, want, got)
}

func TestDefaultLookPath_notFound(t *testing.T) {
	lookPathEnv(t)

	_, err := defaultLookPath("mise")
	require.Error(t, err)
	require.Contains(t, err.Error(), "mise not found")
}

func TestDefaultLookPath_otherNamesSkipFallback(t *testing.T) {
	binDir, home := lookPathEnv(t)
	// The well-known fallback applies only to "mise"; other names use PATH only.
	touch(t, filepath.Join(home, ".local", "bin", "other"))

	_, err := defaultLookPath("other")
	require.Error(t, err)

	want := filepath.Join(binDir, "other")
	touch(t, want)
	got, err := defaultLookPath("other")
	require.NoError(t, err)
	require.Equal(t, want, got)
}
