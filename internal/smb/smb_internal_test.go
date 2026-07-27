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
