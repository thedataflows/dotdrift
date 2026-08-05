package paru_test

import (
	"context"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/thedataflows/dotdrift/internal/paru"
)

// fakeRunner records calls and returns canned output per command.
type fakeRunner struct {
	calls   []string
	outputs map[string]string // "cmd args" → stdout
	errs    map[string]error
}

func (f *fakeRunner) Run(_ context.Context, name string, args ...string) (string, error) {
	key := name + " " + strings.Join(args, " ")
	f.calls = append(f.calls, key)
	if err, ok := f.errs[key]; ok {
		return f.outputs[key], err
	}
	return f.outputs[key], nil
}

func TestPackageStatus_installedRepo(t *testing.T) {
	r := &fakeRunner{
		outputs: map[string]string{
			"pacman -Q neovim": "neovim\t0.10.0-1",
		},
	}
	states := paru.PackageStatus(context.Background(), r, []string{"neovim"})
	require.Len(t, states, 1)
	require.True(t, states[0].Installed)
	require.Equal(t, "0.10.0-1", states[0].Version)
}

func TestPackageStatus_missing(t *testing.T) {
	r := &fakeRunner{
		errs: map[string]error{
			"pacman -Q nonexistent": errNotFound("package not found"),
		},
	}
	states := paru.PackageStatus(context.Background(), r, []string{"nonexistent"})
	require.Len(t, states, 1)
	require.False(t, states[0].Installed)
	require.Empty(t, states[0].Version)
}

func TestPackageStatus_mixedBatch(t *testing.T) {
	r := &fakeRunner{
		outputs: map[string]string{
			"pacman -Q curl":   "curl\t8.0.1-1",
			"pacman -Q ripgrep": "ripgrep\t14.0.3-1",
		},
		errs: map[string]error{
			"pacman -Q ghost": errNotFound("not found"),
		},
	}
	states := paru.PackageStatus(context.Background(), r, []string{"curl", "ghost", "ripgrep"})
	require.Len(t, states, 3)
	require.True(t, states[0].Installed)
	require.Equal(t, "8.0.1-1", states[0].Version)
	require.False(t, states[1].Installed)
	require.True(t, states[2].Installed)
}

func TestFormatStatus_lineProtocol(t *testing.T) {
	states := []paru.State{
		{Name: "neovim", Installed: true, Version: "0.10.0-1"},
		{Name: "ghost"},
	}
	got := paru.FormatStatus(states)
	lines := strings.Split(strings.TrimRight(got, "\n"), "\n")
	require.Equal(t, "neovim\tinstalled\t0.10.0-1", lines[0])
	require.Equal(t, "ghost\tmissing", lines[1])
}

func TestInstallArgs_basic(t *testing.T) {
	args := paru.InstallArgs([]string{"neovim", "ripgrep"}, false, false)
	require.Equal(t, []string{"-S", "--needed", "--noconfirm", "neovim", "ripgrep"}, args)
}

func TestInstallArgs_update(t *testing.T) {
	args := paru.InstallArgs([]string{"curl"}, false, true)
	require.Equal(t, []string{"-S", "--needed", "--noconfirm", "-y", "curl"}, args)
}

func TestInstall_runsParu(t *testing.T) {
	r := &fakeRunner{outputs: map[string]string{}}
	err := paru.Install(context.Background(), r, []string{"neovim"}, false, false)
	require.NoError(t, err)
	require.Contains(t, r.calls[0], "paru -S --needed --noconfirm neovim")
}

func TestInstall_dryRunNoOp(t *testing.T) {
	r := &fakeRunner{}
	err := paru.Install(context.Background(), r, []string{"neovim"}, true, false)
	require.NoError(t, err)
	require.Empty(t, r.calls, "dry-run must not invoke paru")
}

func TestInstall_emptyNoOp(t *testing.T) {
	r := &fakeRunner{}
	err := paru.Install(context.Background(), r, nil, false, false)
	require.NoError(t, err)
	require.Empty(t, r.calls)
}

func TestInstall_propagatesError(t *testing.T) {
	r := &fakeRunner{
		errs: map[string]error{
			"paru -S --needed --noconfirm broken": errNotFound("build failed"),
		},
	}
	err := paru.Install(context.Background(), r, []string{"broken"}, false, false)
	require.Error(t, err)
}

// errNotFound is a trivial error for fake queries.
type errNotFound string

func (e errNotFound) Error() string { return string(e) }
