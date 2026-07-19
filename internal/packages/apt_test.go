package packages_test

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/thedataflows/dotdrift/internal/packages"
)

func TestApt_Present(t *testing.T) {
	r := &recordingRunner{}
	apt := &packages.Apt{Runner: r}
	require.NoError(t, apt.Present(context.Background(), []string{"neovim", "ripgrep"}))
	require.Equal(t, "apt-get", r.name)
	require.Equal(t, []string{"install", "-y", "neovim", "ripgrep"}, r.args)
}

func TestApt_Present_empty(t *testing.T) {
	r := &recordingRunner{}
	apt := &packages.Apt{Runner: r}
	require.NoError(t, apt.Present(context.Background(), nil))
	require.Empty(t, r.name)
}

func TestApt_Absent(t *testing.T) {
	r := &recordingRunner{}
	apt := &packages.Apt{Runner: r}
	require.NoError(t, apt.Absent(context.Background(), []string{"nano"}))
	require.Equal(t, "apt-get", r.name)
	require.Equal(t, []string{"remove", "-y", "nano"}, r.args)
}

func TestApt_IsInstalled(t *testing.T) {
	r := &recordingRunner{}
	apt := &packages.Apt{Runner: r}
	ok, err := apt.IsInstalled(context.Background(), "neovim")
	require.NoError(t, err)
	require.True(t, ok)
	require.Equal(t, "dpkg", r.name)
	require.Equal(t, []string{"-l", "neovim"}, r.args)
}

func TestApt_IsInstalled_notFound(t *testing.T) {
	r := &recordingRunner{err: execExitErr(t, 1)}
	apt := &packages.Apt{Runner: r}
	ok, err := apt.IsInstalled(context.Background(), "missing")
	require.NoError(t, err)
	require.False(t, ok)
}

func TestApt_IsInstalled_errorPropagates(t *testing.T) {
	boom := errors.New("dpkg database locked")
	r := &recordingRunner{err: boom}
	apt := &packages.Apt{Runner: r}
	ok, err := apt.IsInstalled(context.Background(), "neovim")
	require.ErrorIs(t, err, boom)
	require.False(t, ok)
}

type recordingRunner struct {
	ctx  context.Context
	name string
	args []string
	err  error
}

func (r *recordingRunner) Run(ctx context.Context, name string, args ...string) (string, error) {
	r.ctx = ctx
	r.name = name
	r.args = args
	return "", r.err
}
