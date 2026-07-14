package packages_test

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/thedataflows/dotdrift/internal/packages"
)

func TestApt_Present(t *testing.T) {
	r := &recordingRunner{}
	apt := &packages.Apt{Runner: r}
	require.NoError(t, apt.Present([]string{"neovim", "ripgrep"}))
	require.Equal(t, "apt-get", r.name)
	require.Equal(t, []string{"install", "-y", "neovim", "ripgrep"}, r.args)
}

func TestApt_Present_empty(t *testing.T) {
	r := &recordingRunner{}
	apt := &packages.Apt{Runner: r}
	require.NoError(t, apt.Present(nil))
	require.Empty(t, r.name)
}

func TestApt_Absent(t *testing.T) {
	r := &recordingRunner{}
	apt := &packages.Apt{Runner: r}
	require.NoError(t, apt.Absent([]string{"nano"}))
	require.Equal(t, "apt-get", r.name)
	require.Equal(t, []string{"remove", "-y", "nano"}, r.args)
}

func TestApt_IsInstalled(t *testing.T) {
	r := &recordingRunner{}
	apt := &packages.Apt{Runner: r}
	ok, err := apt.IsInstalled("neovim")
	require.NoError(t, err)
	require.True(t, ok)
	require.Equal(t, "dpkg", r.name)
	require.Equal(t, []string{"-l", "neovim"}, r.args)
}

func TestApt_IsInstalled_notFound(t *testing.T) {
	r := &recordingRunner{err: errors.New("exit status 1")}
	apt := &packages.Apt{Runner: r}
	ok, err := apt.IsInstalled("missing")
	require.NoError(t, err)
	require.False(t, ok)
}

type recordingRunner struct {
	name string
	args []string
	err  error
}

func (r *recordingRunner) Run(name string, args ...string) (string, error) {
	r.name = name
	r.args = args
	return "", r.err
}
