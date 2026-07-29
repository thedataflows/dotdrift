package smb

import (
	"bytes"
	"context"
	"errors"
	"slices"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/thedataflows/dotdrift/internal/profile"
	"github.com/thedataflows/dotdrift/internal/resolve"
)

// recordedCall is one runner invocation; interactive marks the
// terminal-attached (smbpasswd) path.
type recordedCall struct {
	argv        []string
	interactive bool
}

// fakeRunner records argv per call; outputs/failures are keyed by the
// space-joined argv. A missing key means success with empty output.
type fakeRunner struct {
	calls       []recordedCall
	outputs     map[string]string
	failures    map[string]error
	interactErr error
}

func (f *fakeRunner) Run(_ context.Context, name string, args ...string) (string, error) {
	argv := append([]string{name}, args...)
	f.calls = append(f.calls, recordedCall{argv: argv})
	key := strings.Join(argv, " ")
	return f.outputs[key], f.failures[key]
}

func (f *fakeRunner) RunInteractive(_ context.Context, name string, args ...string) error {
	f.calls = append(f.calls, recordedCall{argv: append([]string{name}, args...), interactive: true})
	return f.interactErr
}

func argvLines(calls []recordedCall) []string {
	lines := make([]string, 0, len(calls))
	for _, c := range calls {
		lines = append(lines, strings.Join(c.argv, " "))
	}
	return lines
}

// stubSeams pins the privilege (geteuid) and terminal (isTTY) probes.
func stubSeams(t *testing.T, euid int, tty bool) {
	t.Helper()
	origUID, origTTY := geteuid, isTTY
	geteuid = func() int { return euid }
	isTTY = func() bool { return tty }
	t.Cleanup(func() { geteuid, isTTY = origUID, origTTY })
}

func smbPlan(spec profile.SmbSpec) resolve.SmbStep {
	return resolve.SmbStep{Modules: []resolve.SmbModuleSpec{{Module: "media", Spec: spec}}}
}

func TestSmbStep_fullOrdering(t *testing.T) {
	stubSeams(t, 1000, false)
	f := &fakeRunner{
		outputs: map[string]string{
			"sudo -E id -Gn alice": "alice wheel\n",
			"sudo -E testparm -s":  "Load smb config files from /etc/samba/smb.conf\n",
			"sudo -E pdbedit -L":   "bob:1000:Bob\n",
		},
		failures: map[string]error{
			"sudo -E getent group smb":         errors.New("exit status 2"),
			"sudo -E systemctl is-enabled smb": errors.New("exit status 1"),
			"sudo -E systemctl is-active smb":  errors.New("exit status 3"),
		},
	}
	var out bytes.Buffer
	step := &Step{
		Runner: f,
		Out:    &out,
		Plan:   smbPlan(profile.SmbSpec{Users: []string{"alice"}}),
	}

	require.NoError(t, step.Run(context.Background()))
	require.Equal(t, []string{
		"sudo -E getent group smb",
		"sudo -E groupadd smb",
		"sudo -E id -Gn alice",
		"sudo -E usermod -aG smb alice",
		"sudo -E systemctl enable --now avahi-daemon",
		"sudo -E testparm -s",
		"sudo -E systemctl is-enabled smb",
		"sudo -E systemctl enable smb",
		"sudo -E systemctl is-active smb",
		"sudo -E systemctl restart smb",
		"sudo -E pdbedit -L",
	}, argvLines(f.calls))
	require.Contains(t, out.String(), "sudo smbpasswd -a alice")
}

func TestSmbStep_existingGroupNoGroupadd(t *testing.T) {
	stubSeams(t, 0, false) // root: direct argv, no sudo
	f := &fakeRunner{
		outputs: map[string]string{
			"id -Gn alice": "alice media\n",
			"testparm -s":  "ok\n",
			"pdbedit -L":   "alice:1000:Alice\n",
		},
	}
	var out bytes.Buffer
	step := &Step{
		Runner: f,
		Out:    &out,
		Plan:   smbPlan(profile.SmbSpec{Group: "media", Users: []string{"alice"}}),
	}

	require.NoError(t, step.Run(context.Background()))
	require.Equal(t, []string{
		"getent group media",
		"id -Gn alice",
		"systemctl enable --now avahi-daemon",
		"testparm -s",
		"systemctl is-enabled smb",
		"systemctl is-active smb",
		"pdbedit -L",
	}, argvLines(f.calls))
	require.Empty(t, out.String())
}

func TestSmbStep_testparmBeforeRestart(t *testing.T) {
	stubSeams(t, 1000, false)
	f := &fakeRunner{
		failures: map[string]error{
			"sudo -E systemctl is-active smb": errors.New("exit status 3"),
		},
	}
	step := &Step{Runner: f, Out: &bytes.Buffer{}, Plan: smbPlan(profile.SmbSpec{Group: "media"})}

	require.NoError(t, step.Run(context.Background()))
	lines := argvLines(f.calls)
	testparmIdx := slices.Index(lines, "sudo -E testparm -s")
	restartIdx := slices.Index(lines, "sudo -E systemctl restart smb")
	require.NotEqual(t, -1, testparmIdx)
	require.NotEqual(t, -1, restartIdx)
	require.Less(t, testparmIdx, restartIdx)
}

func TestSmbStep_testparmFailureStopsStep(t *testing.T) {
	stubSeams(t, 1000, false)
	f := &fakeRunner{
		outputs: map[string]string{
			"sudo -E testparm -s": "ERROR: bad parameter 'foo'",
		},
		failures: map[string]error{
			"sudo -E testparm -s": errors.New("exit status 1"),
		},
	}
	step := &Step{Runner: f, Out: &bytes.Buffer{}, Plan: smbPlan(profile.SmbSpec{Group: "media"})}

	err := step.Run(context.Background())
	require.Error(t, err)
	require.Contains(t, err.Error(), "ERROR: bad parameter 'foo'")
	lines := argvLines(f.calls)
	require.Contains(t, lines, "sudo -E testparm -s")
	require.NotContains(t, lines, "sudo -E systemctl is-enabled smb")
	require.NotContains(t, lines, "sudo -E systemctl enable smb")
	require.NotContains(t, lines, "sudo -E systemctl is-active smb")
	require.NotContains(t, lines, "sudo -E systemctl restart smb")
}

// Verbose ExecRunner streams child stdout/stderr live AND still returns the
// combined capture — callers parse (id -Gn, pdbedit -L) and append (testparm
// gate) the returned output, so streaming must never starve them.
func TestExecRunner_verboseStreamsAndCaptures(t *testing.T) {
	var out, errW bytes.Buffer
	r := ExecRunner{Verbose: true, Out: &out, Err: &errW}

	got, err := r.Run(context.Background(), "sh", "-c", "echo out-line; echo err-line >&2")
	require.NoError(t, err)
	require.Contains(t, out.String(), "out-line", "stdout must stream live")
	require.Contains(t, errW.String(), "err-line", "stderr must stream live")
	require.Contains(t, got, "out-line", "returned capture must survive streaming (parsing contract)")
	require.Contains(t, got, "err-line", "returned capture must survive streaming (parsing contract)")
}

// Non-verbose ExecRunner keeps today's contract: combined output captured
// and returned, nothing on the writers.
func TestExecRunner_nonVerboseCapturesOnly(t *testing.T) {
	var out, errW bytes.Buffer
	r := ExecRunner{Verbose: false, Out: &out, Err: &errW}

	got, err := r.Run(context.Background(), "sh", "-c", "echo out-line; echo err-line >&2")
	require.NoError(t, err)
	require.Contains(t, got, "out-line")
	require.Contains(t, got, "err-line")
	require.Empty(t, out.String())
	require.Empty(t, errW.String())
}

// Verbose ExecRunner echoes the command line (bash set -x style: "+ argv")
// to Err before streaming; the echo never pollutes the returned capture
// (id -Gn / pdbedit -L parsing contract) nor Out.
func TestExecRunner_verboseEchoesCommandLine(t *testing.T) {
	var out, errW bytes.Buffer
	r := ExecRunner{Verbose: true, Out: &out, Err: &errW}

	got, err := r.Run(context.Background(), "sh", "-c", "echo out-line; echo err-line >&2")
	require.NoError(t, err)

	echo := "+ sh -c 'echo out-line; echo err-line >&2'\n"
	require.Contains(t, errW.String(), echo, "verbose must echo the command line to Err")
	require.Less(t, strings.Index(errW.String(), echo), strings.Index(errW.String(), "err-line"),
		"the echo must precede the command's own stderr output")
	require.NotContains(t, out.String(), "+ ", "the echo goes to Err, never Out")
	require.NotContains(t, got, "+ ", "the echo must not enter the captured output (parsing contract)")
}

func TestExecRunner_setVerbose(t *testing.T) {
	r := &ExecRunner{}
	require.False(t, r.Verbose)
	r.SetVerbose(true)
	require.True(t, r.Verbose)
}
