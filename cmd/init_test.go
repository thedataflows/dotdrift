package dotdrift_test

import (
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
	cmd "github.com/thedataflows/dotdrift/cmd"
)

func TestInit_createsProfile(t *testing.T) {
	dir := t.TempDir()
	err := cmd.Run("dev", []string{"init", dir})
	require.NoError(t, err)
	require.FileExists(t, filepath.Join(dir, "dotdrift.toml"))
	data, err := os.ReadFile(filepath.Join(dir, "dotdrift.toml"))
	require.NoError(t, err)
	require.Equal(t, "[modules]\n", string(data))
	require.DirExists(t, filepath.Join(dir, "modules"))
	require.DirExists(t, filepath.Join(dir, "hosts"))
	require.DirExists(t, filepath.Join(dir, "users"))
}

func TestInit_alreadyExists(t *testing.T) {
	dir := t.TempDir()
	err := cmd.Run("dev", []string{"init", dir})
	require.NoError(t, err)

	err = cmd.Run("dev", []string{"init", dir})
	require.Error(t, err)
	require.Contains(t, err.Error(), "profile already exists")
}

func TestInit_clonesProfile(t *testing.T) {
	remote := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(remote, "dotdrift.toml"), []byte("[modules]\n"), 0o644))

	gitExec(t, remote, "init")
	gitExec(t, remote, "config", "user.email", "test@example.com")
	gitExec(t, remote, "config", "user.name", "Test")
	gitExec(t, remote, "add", ".")
	gitExec(t, remote, "commit", "-m", "initial")

	work := t.TempDir()
	orig, err := os.Getwd()
	require.NoError(t, err)
	require.NoError(t, os.Chdir(work))
	t.Cleanup(func() {
		if err := os.Chdir(orig); err != nil {
			t.Logf("restore dir: %v", err)
		}
	})

	url := "file://" + remote
	err = cmd.Run("dev", []string{"init", url})
	require.NoError(t, err)

	base := filepath.Base(remote)
	require.FileExists(t, filepath.Join(work, base, "dotdrift.toml"))
}

func TestInit_cloneStripsGitSuffix(t *testing.T) {
	remoteParent := t.TempDir()
	remote := filepath.Join(remoteParent, "bar.git")
	require.NoError(t, os.MkdirAll(remote, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(remote, "dotdrift.toml"), []byte("[modules]\n"), 0o644))

	gitExec(t, remote, "init")
	gitExec(t, remote, "config", "user.email", "test@example.com")
	gitExec(t, remote, "config", "user.name", "Test")
	gitExec(t, remote, "add", ".")
	gitExec(t, remote, "commit", "-m", "initial")

	work := t.TempDir()
	orig, err := os.Getwd()
	require.NoError(t, err)
	require.NoError(t, os.Chdir(work))
	t.Cleanup(func() {
		if err := os.Chdir(orig); err != nil {
			t.Logf("restore dir: %v", err)
		}
	})

	err = cmd.Run("dev", []string{"init", "file://" + remote})
	require.NoError(t, err)

	require.FileExists(t, filepath.Join(work, "bar", "dotdrift.toml"))
	require.NoDirExists(t, filepath.Join(work, "bar.git"))
}

func TestInit_cloneRequiresDotdriftToml(t *testing.T) {
	remote := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(remote, "README.md"), []byte("not a profile\n"), 0o644))

	gitExec(t, remote, "init")
	gitExec(t, remote, "config", "user.email", "test@example.com")
	gitExec(t, remote, "config", "user.name", "Test")
	gitExec(t, remote, "add", ".")
	gitExec(t, remote, "commit", "-m", "initial")

	work := t.TempDir()
	orig, err := os.Getwd()
	require.NoError(t, err)
	require.NoError(t, os.Chdir(work))
	t.Cleanup(func() {
		if err := os.Chdir(orig); err != nil {
			t.Logf("restore dir: %v", err)
		}
	})

	err = cmd.Run("dev", []string{"init", "file://" + remote})
	require.Error(t, err)
	require.Contains(t, err.Error(), "not a dotdrift profile")

	// The cloned directory is left in place for inspection.
	require.DirExists(t, filepath.Join(work, filepath.Base(remote)))
}

func gitExec(t *testing.T, dir string, args ...string) {
	t.Helper()
	c := exec.Command("git", args...)
	c.Dir = dir
	c.Env = append(os.Environ(), "GIT_AUTHOR_NAME=Test", "GIT_AUTHOR_EMAIL=test@example.com")
	out, err := c.CombinedOutput()
	require.NoError(t, err, string(out))
}

func TestExitCodes_usage(t *testing.T) {
	err := cmd.Run("dev", []string{"--invalid-flag"})
	require.Error(t, err)
	var exitErr *cmd.ExitError
	require.True(t, errors.As(err, &exitErr))
	require.Equal(t, 2, exitErr.Code)
}

func TestExitCodes_runtime(t *testing.T) {
	dir := t.TempDir()
	err := cmd.Run("dev", []string{"init", dir})
	require.NoError(t, err)

	err = cmd.Run("dev", []string{"init", dir})
	require.Error(t, err)
	var exitErr *cmd.ExitError
	require.True(t, errors.As(err, &exitErr))
	require.Equal(t, 1, exitErr.Code)
}
