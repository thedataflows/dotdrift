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
	// Present must refresh the apt index before installing: a fresh
	// machine/container has an empty or stale index and `apt-get install`
	// fails with "Unable to locate package".
	require.Len(t, r.calls, 2)
	require.Equal(t, "apt-get", r.calls[0].Name)
	require.Equal(t, []string{"update"}, r.calls[0].Args)
	require.Equal(t, "apt-get", r.calls[1].Name)
	require.Equal(t, []string{"install", "-y", "neovim", "ripgrep"}, r.calls[1].Args)
}

func TestApt_Present_updateErrorStopsInstall(t *testing.T) {
	boom := errors.New("apt index unreachable")
	r := &recordingRunner{err: boom}
	apt := &packages.Apt{Runner: r}
	require.ErrorIs(t, apt.Present(context.Background(), []string{"curl"}), boom)
	require.Len(t, r.calls, 1)
	require.Equal(t, []string{"update"}, r.calls[0].Args)
}

func TestApt_Present_empty(t *testing.T) {
	r := &recordingRunner{}
	apt := &packages.Apt{Runner: r}
	require.NoError(t, apt.Present(context.Background(), nil))
	require.Empty(t, r.calls)
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

type recordedCall struct {
	Name string
	Args []string
}

type recordingRunner struct {
	ctx   context.Context
	name  string
	args  []string
	calls []recordedCall
	err   error
}

func (r *recordingRunner) Run(ctx context.Context, name string, args ...string) (string, error) {
	r.ctx = ctx
	r.name = name
	r.args = args
	r.calls = append(r.calls, recordedCall{Name: name, Args: args})
	return "", r.err
}
