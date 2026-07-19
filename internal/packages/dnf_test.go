package packages_test

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/thedataflows/dotdrift/internal/packages"
)

func TestDnf_Present(t *testing.T) {
	r := &recordingRunner{}
	dnf := &packages.Dnf{Runner: r}
	require.NoError(t, dnf.Present(context.Background(), []string{"neovim", "ripgrep"}))
	require.Equal(t, "dnf", r.name)
	require.Equal(t, []string{"install", "-y", "neovim", "ripgrep"}, r.args)
}

func TestDnf_Present_empty(t *testing.T) {
	r := &recordingRunner{}
	dnf := &packages.Dnf{Runner: r}
	require.NoError(t, dnf.Present(context.Background(), nil))
	require.Empty(t, r.name)
}

func TestDnf_Absent(t *testing.T) {
	r := &recordingRunner{}
	dnf := &packages.Dnf{Runner: r}
	require.NoError(t, dnf.Absent(context.Background(), []string{"nano"}))
	require.Equal(t, "dnf", r.name)
	require.Equal(t, []string{"remove", "-y", "nano"}, r.args)
}

func TestDnf_IsInstalled(t *testing.T) {
	r := &recordingRunner{}
	dnf := &packages.Dnf{Runner: r}
	ok, err := dnf.IsInstalled(context.Background(), "neovim")
	require.NoError(t, err)
	require.True(t, ok)
	require.Equal(t, "rpm", r.name)
	require.Equal(t, []string{"-q", "neovim"}, r.args)
}

func TestDnf_IsInstalled_notFound(t *testing.T) {
	r := &recordingRunner{err: execExitErr(t, 1)}
	dnf := &packages.Dnf{Runner: r}
	ok, err := dnf.IsInstalled(context.Background(), "missing")
	require.NoError(t, err)
	require.False(t, ok)
}

func TestDnf_IsInstalled_errorPropagates(t *testing.T) {
	boom := errors.New("rpm database corrupted")
	r := &recordingRunner{err: boom}
	dnf := &packages.Dnf{Runner: r}
	ok, err := dnf.IsInstalled(context.Background(), "neovim")
	require.ErrorIs(t, err, boom)
	require.False(t, ok)
}
