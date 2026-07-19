package packages_test

import (
	"context"
	"errors"
	"os/exec"
	"strconv"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/thedataflows/dotdrift/internal/packages"
	"github.com/thedataflows/dotdrift/internal/resolve"
)

// execExitErr returns a real *exec.ExitError with the given exit code by
// running a shell command that exits with it.
func execExitErr(t *testing.T, code int) error {
	t.Helper()
	cmd := exec.Command("sh", "-c", "exit "+strconv.Itoa(code))
	err := cmd.Run()
	require.Error(t, err)
	var exitErr *exec.ExitError
	require.ErrorAs(t, err, &exitErr)
	require.Equal(t, code, exitErr.ExitCode())
	return err
}

type fakeRunner struct {
	ctx   context.Context
	calls []call
	err   error
}

type call struct {
	name string
	args []string
}

func (f *fakeRunner) Run(ctx context.Context, name string, args ...string) (string, error) {
	f.ctx = ctx
	f.calls = append(f.calls, call{name: name, args: args})
	return "", f.err
}

func TestParu_presentCommandLine(t *testing.T) {
	f := &fakeRunner{}
	b := &packages.Paru{Runner: f}
	require.NoError(t, b.Present(context.Background(), []string{"neovim", "ripgrep", "neovim"}))
	require.Len(t, f.calls, 1)
	require.Equal(t, "paru", f.calls[0].name)
	require.Equal(t, []string{"-S", "--needed", "--noconfirm", "neovim", "ripgrep"}, f.calls[0].args)
}

func TestParu_presentNoPackages(t *testing.T) {
	f := &fakeRunner{}
	b := &packages.Paru{Runner: f}
	require.NoError(t, b.Present(context.Background(), nil))
	require.Empty(t, f.calls)
}

func TestParu_absent(t *testing.T) {
	f := &fakeRunner{}
	b := &packages.Paru{Runner: f}
	require.NoError(t, b.Absent(context.Background(), []string{"nano", "vim"}))
	require.Len(t, f.calls, 1)
	require.Equal(t, "paru", f.calls[0].name)
	require.Equal(t, []string{"-R", "--noconfirm", "nano", "vim"}, f.calls[0].args)
}

func TestParu_absentNoPackages(t *testing.T) {
	f := &fakeRunner{}
	b := &packages.Paru{Runner: f}
	require.NoError(t, b.Absent(context.Background(), nil))
	require.Empty(t, f.calls)
}

func TestParu_propagatesContext(t *testing.T) {
	f := &fakeRunner{}
	b := &packages.Paru{Runner: f}
	type ctxKey struct{}
	ctx := context.WithValue(context.Background(), ctxKey{}, "v")
	require.NoError(t, b.Present(ctx, []string{"neovim"}))
	require.Equal(t, "v", f.ctx.Value(ctxKey{}), "backend must pass ctx through to the runner")
}

func TestPacman_isInstalled(t *testing.T) {
	f := &fakeRunner{}
	b := &packages.Paru{Runner: f}
	ok, err := b.IsInstalled(context.Background(), "neovim")
	require.NoError(t, err)
	require.True(t, ok)
	require.Len(t, f.calls, 1)
	require.Equal(t, "pacman", f.calls[0].name)
	require.Equal(t, []string{"-Q", "neovim"}, f.calls[0].args)
}

func TestPacman_isInstalledNotFound(t *testing.T) {
	f := &fakeRunner{err: execExitErr(t, 1)}
	b := &packages.Paru{Runner: f}
	ok, err := b.IsInstalled(context.Background(), "missing")
	require.NoError(t, err)
	require.False(t, ok)
}

func TestPacman_isInstalledErrorPropagates(t *testing.T) {
	boom := errors.New("boom")
	f := &fakeRunner{err: boom}
	b := &packages.Paru{Runner: f}
	ok, err := b.IsInstalled(context.Background(), "neovim")
	require.ErrorIs(t, err, boom)
	require.False(t, ok)
}

func TestFor_paruFamily(t *testing.T) {
	for _, name := range []string{"paru", "arch", "cachyos", "manjaro"} {
		b := packages.For(name)
		require.Implements(t, (*packages.Backend)(nil), b)
	}
}

func TestParu_presentError(t *testing.T) {
	f := &fakeRunner{err: errors.New("cancelled")}
	b := &packages.Paru{Runner: f}
	err := b.Present(context.Background(), []string{"x"})
	require.Error(t, err)
	require.Contains(t, err.Error(), "cancelled")
}

func TestExecRunner_Run(t *testing.T) {
	out, err := packages.ExecRunner{}.Run(context.Background(), "echo", "hello")
	require.NoError(t, err)
	require.Equal(t, "hello\n", out)
}

func TestExecRunner_Run_alreadyCancelledContext(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err := packages.ExecRunner{}.Run(ctx, "sleep", "30")
	require.Error(t, err, "cancelled context must fail the command")
}

func TestExecRunner_Run_contextCancelKillsCommand(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	start := time.Now()
	go func() {
		time.Sleep(100 * time.Millisecond)
		cancel()
	}()
	_, err := packages.ExecRunner{}.Run(ctx, "sleep", "30")
	require.Error(t, err, "cancelling ctx must kill the running command")
	require.Less(t, time.Since(start), 10*time.Second, "command must be killed promptly, not run to completion")
}

type fakeBackend struct {
	ctx          context.Context
	presentCalls [][]string
	absentCalls  [][]string
	err          error
}

func (f *fakeBackend) Present(ctx context.Context, pkgs []string) error {
	f.ctx = ctx
	f.presentCalls = append(f.presentCalls, pkgs)
	return f.err
}

func (f *fakeBackend) Absent(ctx context.Context, pkgs []string) error {
	f.ctx = ctx
	f.absentCalls = append(f.absentCalls, pkgs)
	return f.err
}

func (f *fakeBackend) IsInstalled(ctx context.Context, pkg string) (bool, error) {
	return false, nil
}

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

func TestPackagesStep_propagatesContext(t *testing.T) {
	b := &fakeBackend{}
	plan := &resolve.Plan{
		Packages: resolve.PackagesStep{
			Install: []string{"neovim"},
		},
	}

	type ctxKey struct{}
	ctx := context.WithValue(context.Background(), ctxKey{}, "step-ctx")
	step := packages.NewStep(b, plan)
	require.NoError(t, step.Run(ctx))
	require.Equal(t, "step-ctx", b.ctx.Value(ctxKey{}), "step must pass its ctx through to the backend")
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
