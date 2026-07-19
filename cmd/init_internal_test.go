package dotdrift

import (
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

type recordedGitCall struct {
	dir  string
	args []string
}

func stubGit(t *testing.T, run func(dir string, args ...string) error) *[]recordedGitCall {
	t.Helper()
	calls := &[]recordedGitCall{}
	orig := runGit
	runGit = func(dir string, args ...string) error {
		*calls = append(*calls, recordedGitCall{dir: dir, args: args})
		if run != nil {
			return run(dir, args...)
		}
		return nil
	}
	t.Cleanup(func() { runGit = orig })
	return calls
}

func chdir(t *testing.T, dir string) {
	t.Helper()
	orig, err := os.Getwd()
	require.NoError(t, err)
	require.NoError(t, os.Chdir(dir))
	t.Cleanup(func() {
		if err := os.Chdir(orig); err != nil {
			t.Logf("restore dir: %v", err)
		}
	})
}

func TestInit_cloneRunsInTargetParent(t *testing.T) {
	work := t.TempDir()
	chdir(t, work)

	calls := stubGit(t, func(dir string, args ...string) error {
		target := args[len(args)-1]
		if err := os.MkdirAll(target, 0o755); err != nil {
			return err
		}
		return os.WriteFile(filepath.Join(target, "dotdrift.toml"), []byte("[modules]\n"), 0o644)
	})

	err := Run("dev", []string{"init", "https://github.com/foo/bar.git"})
	require.NoError(t, err)

	require.Len(t, *calls, 1)
	target := filepath.Join(work, "bar")
	require.Equal(t, filepath.Dir(target), (*calls)[0].dir)
	require.Equal(t, []string{"clone", "https://github.com/foo/bar.git", target}, (*calls)[0].args)
}

func TestInit_createRunsGitInit(t *testing.T) {
	t.Run("invokes git init in the profile dir", func(t *testing.T) {
		dir := filepath.Join(t.TempDir(), "profile")
		calls := stubGit(t, nil)

		err := Run("dev", []string{"init", dir})
		require.NoError(t, err)

		require.Len(t, *calls, 1)
		require.Equal(t, dir, (*calls)[0].dir)
		require.Equal(t, "init", (*calls)[0].args[0])
	})

	t.Run("git failure warns but does not fail init", func(t *testing.T) {
		dir := filepath.Join(t.TempDir(), "profile")
		stubGit(t, func(dir string, args ...string) error {
			return errors.New("git not found")
		})

		err := Run("dev", []string{"init", dir})
		require.NoError(t, err)
		require.FileExists(t, filepath.Join(dir, "dotdrift.toml"))
	})
}
