package mounts

import (
	"bytes"
	"context"
	"errors"
	"testing"

	"github.com/rs/zerolog/log"
	"github.com/stretchr/testify/require"

	"github.com/thedataflows/dotdrift/internal/profile"
	"github.com/thedataflows/dotdrift/internal/resolve"
)

// swapEUID pins the effective-uid seam for the duration of a test.
func swapEUID(t *testing.T, euid int) {
	t.Helper()
	orig := geteuid
	geteuid = func() int { return euid }
	t.Cleanup(func() { geteuid = orig })
}

// fakeRunner records every argv (and ctx) it is asked to run; fail, when
// set, decides the error per invocation.
type fakeRunner struct {
	calls [][]string
	ctxs  []context.Context
	fail  func(argv []string) error
}

func (f *fakeRunner) Run(ctx context.Context, argv []string) error {
	f.ctxs = append(f.ctxs, ctx)
	f.calls = append(f.calls, append([]string(nil), argv...))
	if f.fail != nil {
		return f.fail(argv)
	}
	return nil
}

// captureLog swaps the global zerolog logger for a buffer for the duration
// of a test.
func captureLog(t *testing.T) *bytes.Buffer {
	t.Helper()
	var buf bytes.Buffer
	orig := log.Logger
	log.Logger = log.Output(&buf)
	t.Cleanup(func() { log.Logger = orig })
	return &buf
}

// Root: systemctl runs directly, no sudo invocation.
func TestSystemctlArgv_rootDirect(t *testing.T) {
	argv := systemctlArgv(0, "daemon-reload")
	require.Equal(t, []string{"systemctl", "daemon-reload"}, argv)
}

// Non-root: systemctl goes through sudo -E (mirroring the mise sudo pattern).
func TestSystemctlArgv_nonRootSudo(t *testing.T) {
	argv := systemctlArgv(1000, "enable", "--now", "mnt-a.mount")
	require.Equal(t, []string{"sudo", "-E", "systemctl", "enable", "--now", "mnt-a.mount"}, argv)
}

// Root: mkdir runs directly.
func TestMkdirArgv_rootDirect(t *testing.T) {
	argv := mkdirArgv(0, "/mnt/a")
	require.Equal(t, []string{"mkdir", "-p", "/mnt/a"}, argv)
}

// Non-root: mkdir is plain sudo mkdir -p (no systemctl wrapper, no -E).
func TestMkdirArgv_nonRootSudo(t *testing.T) {
	argv := mkdirArgv(1000, "/mnt/a")
	require.Equal(t, []string{"sudo", "mkdir", "-p", "/mnt/a"}, argv)
}

// The step name matches the pipeline step the plan aggregates for.
func TestMountsStep_name(t *testing.T) {
	require.Equal(t, "mounts", (&Step{}).Name())
}

// Exact command sequence for a two-entry plan (one with StartAt): all
// mkdirs in entry order, exactly one daemon-reload, then enable --now of
// mount and timer units per entry.
func TestMountsStep_orderingMkdirReloadEnable(t *testing.T) {
	swapEUID(t, 0)
	r := &fakeRunner{}
	step := &Step{
		Runner: r,
		Plan: resolve.MountsStep{Entries: []resolve.MountEntry{
			{Name: "a", Module: "m", Layer: "base", Spec: profile.MountSpec{Destination: "/mnt/a", StartAt: "daily"}},
			{Name: "b", Module: "m", Layer: "base", Spec: profile.MountSpec{Destination: "/mnt/b", State: "enabled"}},
		}},
	}

	require.NoError(t, step.Run(context.Background()))
	require.Equal(t, [][]string{
		{"mkdir", "-p", "/mnt/a"},
		{"mkdir", "-p", "/mnt/b"},
		{"systemctl", "daemon-reload"},
		{"systemctl", "enable", "--now", "mnt-a.mount"},
		{"systemctl", "enable", "--now", "mnt-a.timer"},
		{"systemctl", "enable", "--now", "mnt-b.mount"},
	}, r.calls)
}

// A disabled entry is disabled (mount and, when StartAt is set, timer)
// instead of enabled.
func TestMountsStep_stateDisabledUsesDisable(t *testing.T) {
	swapEUID(t, 0)
	r := &fakeRunner{}
	step := &Step{
		Runner: r,
		Plan: resolve.MountsStep{Entries: []resolve.MountEntry{
			{Name: "data", Module: "m", Layer: "base", Spec: profile.MountSpec{Destination: "/mnt/data", StartAt: "hourly", State: "disabled"}},
		}},
	}

	require.NoError(t, step.Run(context.Background()))
	require.Equal(t, [][]string{
		{"mkdir", "-p", "/mnt/data"},
		{"systemctl", "daemon-reload"},
		{"systemctl", "disable", "--now", "mnt-data.mount"},
		{"systemctl", "disable", "--now", "mnt-data.timer"},
	}, r.calls)
}

// Resume-safety: disable of an absent/already-disabled unit fails inside
// systemctl ("not loaded") — the step must log a warning and continue with
// the remaining entries rather than fail.
func TestMountsStep_disableToleratesAlreadyDisabled(t *testing.T) {
	swapEUID(t, 0)
	buf := captureLog(t)
	r := &fakeRunner{fail: func(argv []string) error {
		for _, a := range argv {
			if a == "mnt-a.mount" {
				return errors.New(`exit status 1: Failed to disable unit: Unit mnt-a.mount not loaded`)
			}
		}
		return nil
	}}
	step := &Step{
		Runner: r,
		Plan: resolve.MountsStep{Entries: []resolve.MountEntry{
			{Name: "a", Module: "m", Layer: "base", Spec: profile.MountSpec{Destination: "/mnt/a", State: "disabled"}},
			{Name: "b", Module: "m", Layer: "base", Spec: profile.MountSpec{Destination: "/mnt/b", State: "disabled"}},
		}},
	}

	require.NoError(t, step.Run(context.Background()))
	require.Equal(t, [][]string{
		{"mkdir", "-p", "/mnt/a"},
		{"mkdir", "-p", "/mnt/b"},
		{"systemctl", "daemon-reload"},
		{"systemctl", "disable", "--now", "mnt-a.mount"},
		{"systemctl", "disable", "--now", "mnt-b.mount"},
	}, r.calls, "the failed disable must not stop the remaining entries")
	require.Contains(t, buf.String(), `"level":"warn"`)
	require.Contains(t, buf.String(), "mnt-a.mount")
}

// Empty plan: no-op success, zero commands run.
func TestMountsStep_emptyPlanNoop(t *testing.T) {
	r := &fakeRunner{}
	step := &Step{Runner: r, Plan: resolve.MountsStep{}}

	require.NoError(t, step.Run(context.Background()))
	require.Empty(t, r.calls)
}

// A failing mkdir aborts the step before any systemctl invocation; the
// error names the destination.
func TestMountsStep_mkdirFailureStopsStep(t *testing.T) {
	swapEUID(t, 0)
	r := &fakeRunner{fail: func(argv []string) error {
		if argv[0] == "mkdir" {
			return errors.New("permission denied")
		}
		return nil
	}}
	step := &Step{
		Runner: r,
		Plan: resolve.MountsStep{Entries: []resolve.MountEntry{
			{Name: "a", Module: "m", Layer: "base", Spec: profile.MountSpec{Destination: "/mnt/a"}},
			{Name: "b", Module: "m", Layer: "base", Spec: profile.MountSpec{Destination: "/mnt/b"}},
		}},
	}

	err := step.Run(context.Background())
	require.Error(t, err)
	require.Contains(t, err.Error(), "/mnt/a")
	require.Equal(t, [][]string{
		{"mkdir", "-p", "/mnt/a"},
	}, r.calls, "daemon-reload and enable must never run after a failed mkdir")
}

// The caller's context is propagated to every command invocation.
func TestMountsStep_ctxPropagated(t *testing.T) {
	swapEUID(t, 0)
	type ctxKey struct{}
	ctx := context.WithValue(context.Background(), ctxKey{}, "marker")
	r := &fakeRunner{}
	step := &Step{
		Runner: r,
		Plan: resolve.MountsStep{Entries: []resolve.MountEntry{
			{Name: "a", Module: "m", Layer: "base", Spec: profile.MountSpec{Destination: "/mnt/a"}},
		}},
	}

	require.NoError(t, step.Run(ctx))
	require.NotEmpty(t, r.ctxs)
	for i, c := range r.ctxs {
		require.Equal(t, "marker", c.Value(ctxKey{}), "call %d received a different context", i)
	}
}

// Unit names are derived with generate.EscapePath: a destination with a
// space yields the \xNN-escaped unit name.
func TestMountsStep_unitNameEscapesDestination(t *testing.T) {
	swapEUID(t, 0)
	r := &fakeRunner{}
	step := &Step{
		Runner: r,
		Plan: resolve.MountsStep{Entries: []resolve.MountEntry{
			{Name: "files", Module: "m", Layer: "base", Spec: profile.MountSpec{Destination: "/mnt/my files"}},
		}},
	}

	require.NoError(t, step.Run(context.Background()))
	require.Equal(t, [][]string{
		{"mkdir", "-p", "/mnt/my files"},
		{"systemctl", "daemon-reload"},
		{"systemctl", "enable", "--now", `mnt-my\x20files.mount`},
	}, r.calls)
}
