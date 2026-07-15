package packages_test

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/require"
	"context"

	"github.com/thedataflows/dotdrift/internal/packages"
	"github.com/thedataflows/dotdrift/internal/resolve"
)

type fakeRunner struct {
	calls []call
	err   error
}

type call struct {
	name string
	args []string
}

func (f *fakeRunner) Run(name string, args ...string) (string, error) {
	f.calls = append(f.calls, call{name: name, args: args})
	return "", f.err
}

func TestParu_presentCommandLine(t *testing.T) {
	f := &fakeRunner{}
	b := &packages.Paru{Runner: f}
	require.NoError(t, b.Present([]string{"neovim", "ripgrep", "neovim"}))
	require.Len(t, f.calls, 1)
	require.Equal(t, "paru", f.calls[0].name)
	require.Equal(t, []string{"-S", "--needed", "--noconfirm", "neovim", "ripgrep"}, f.calls[0].args)
}

func TestParu_presentNoPackages(t *testing.T) {
	f := &fakeRunner{}
	b := &packages.Paru{Runner: f}
	require.NoError(t, b.Present(nil))
	require.Empty(t, f.calls)
}

func TestParu_absent(t *testing.T) {
	f := &fakeRunner{}
	b := &packages.Paru{Runner: f}
	require.NoError(t, b.Absent([]string{"nano", "vim"}))
	require.Len(t, f.calls, 1)
	require.Equal(t, "paru", f.calls[0].name)
	require.Equal(t, []string{"-R", "--noconfirm", "nano", "vim"}, f.calls[0].args)
}

func TestParu_absentNoPackages(t *testing.T) {
	f := &fakeRunner{}
	b := &packages.Paru{Runner: f}
	require.NoError(t, b.Absent(nil))
	require.Empty(t, f.calls)
}

func TestPacman_isInstalled(t *testing.T) {
	f := &fakeRunner{}
	b := &packages.Paru{Runner: f}
	ok, err := b.IsInstalled("neovim")
	require.NoError(t, err)
	require.True(t, ok)
	require.Len(t, f.calls, 1)
	require.Equal(t, "pacman", f.calls[0].name)
	require.Equal(t, []string{"-Q", "neovim"}, f.calls[0].args)
}

func TestFor_paruFamily(t *testing.T) {
	for _, b := range []string{"paru", "arch", "cachyos", "manjaro"} {
		b := packages.For(b)
		require.Implements(t, (*packages.Backend)(nil), b)
	}
}

func TestFor_unknownIsNoop(t *testing.T) {
	b := packages.For("unknown")
	require.NoError(t, b.Present([]string{"x"}))
	require.NoError(t, b.Absent([]string{"x"}))
	ok, err := b.IsInstalled("x")
	require.NoError(t, err)
	require.False(t, ok)
}

func TestParu_presentError(t *testing.T) {
	f := &fakeRunner{err: errors.New("cancelled")}
	b := &packages.Paru{Runner: f}
	err := b.Present([]string{"x"})
	require.Error(t, err)
	require.Contains(t, err.Error(), "cancelled")
}


type fakeBackend struct {
	presentCalls [][]string
	absentCalls  [][]string
	err          error
}

func (f *fakeBackend) Present(pkgs []string) error {
	f.presentCalls = append(f.presentCalls, pkgs)
	return f.err
}

func (f *fakeBackend) Absent(pkgs []string) error {
	f.absentCalls = append(f.absentCalls, pkgs)
	return f.err
}

func (f *fakeBackend) IsInstalled(pkg string) (bool, error) { return false, nil }

func TestPackagesStep_callsAbsentAndPresent(t *testing.T) {
	b := &fakeBackend{}
	plan := &resolve.Plan{
		Packages: resolve.PackagesStep{
			Install: []string{"neovim"},
			Remove:  []string{"nano"},
		},
	}

	step := packages.NewStep(b, plan)
	require.NoError(t, step.Run(context.Background()))
	require.Len(t, b.absentCalls, 1, "absent should be called")
	require.Equal(t, []string{"nano"}, b.absentCalls[0])
	require.Len(t, b.presentCalls, 1, "present should be called")
	require.Equal(t, []string{"neovim"}, b.presentCalls[0])
}

func TestPackagesStep_removeErrorFails(t *testing.T) {
	b := &fakeBackend{err: errors.New("remove failed")}
	plan := &resolve.Plan{
		Packages: resolve.PackagesStep{
			Install: []string{"neovim"},
			Remove:  []string{"nano"},
		},
	}

	step := packages.NewStep(b, plan)
	err := step.Run(context.Background())
	require.Error(t, err)
	require.Contains(t, err.Error(), "remove failed")
}

func TestPackagesStep_noPackagesNoBackendCalls(t *testing.T) {
	b := &fakeBackend{}
	plan := &resolve.Plan{
		Packages: resolve.PackagesStep{},
	}

	step := packages.NewStep(b, plan)
	require.NoError(t, step.Run(context.Background()))
	require.Empty(t, b.presentCalls)
	require.Empty(t, b.absentCalls)
}
