package smb

import (
	"bytes"
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/thedataflows/dotdrift/internal/profile"
	"github.com/thedataflows/dotdrift/internal/resolve"
)

func TestSmbStep_noTTY_warnsAndSkipsSmbpasswd(t *testing.T) {
	stubSeams(t, 1000, false)
	f := &fakeRunner{
		outputs: map[string]string{
			"sudo -E id -Gn carol": "carol smb\n",
			"sudo -E pdbedit -L":   "alice:1000:Alice\n",
		},
	}
	var out bytes.Buffer
	step := &Step{
		Runner: f,
		Out:    &out,
		Plan:   smbPlan(profile.SmbSpec{Users: []string{"carol"}}),
	}

	require.NoError(t, step.Run(context.Background()))
	require.Contains(t, out.String(), "samba password missing for carol; run: sudo smbpasswd -a carol")
	for _, c := range f.calls {
		require.NotContains(t, c.argv, "smbpasswd")
		require.False(t, c.interactive)
	}
}

func TestSmbStep_tty_runsSmbpasswd(t *testing.T) {
	stubSeams(t, 1000, true)
	f := &fakeRunner{
		outputs: map[string]string{
			"sudo -E id -Gn dave": "dave smb\n",
		},
	}
	var out bytes.Buffer
	step := &Step{
		Runner: f,
		Out:    &out,
		Plan:   smbPlan(profile.SmbSpec{Users: []string{"dave"}}),
	}

	require.NoError(t, step.Run(context.Background()))
	require.Empty(t, out.String())
	var interactive []recordedCall
	for _, c := range f.calls {
		if c.interactive {
			interactive = append(interactive, c)
		}
	}
	require.Len(t, interactive, 1)
	require.Equal(t, []string{"sudo", "-E", "smbpasswd", "-a", "dave"}, interactive[0].argv)
}

func TestSmbStep_avahiDisabled_noAvahiCall(t *testing.T) {
	stubSeams(t, 1000, false)
	off := false
	f := &fakeRunner{}
	step := &Step{
		Runner: f,
		Out:    &bytes.Buffer{},
		Plan:   smbPlan(profile.SmbSpec{Group: "media", Avahi: &off}),
	}

	require.NoError(t, step.Run(context.Background()))
	for _, line := range argvLines(f.calls) {
		require.NotContains(t, line, "avahi")
	}
}

func TestSmbStep_avahiDefaultEnabled(t *testing.T) {
	stubSeams(t, 1000, false)
	f := &fakeRunner{}
	step := &Step{
		Runner: f,
		Out:    &bytes.Buffer{},
		Plan:   smbPlan(profile.SmbSpec{Group: "media"}), // Avahi nil → default true
	}

	require.NoError(t, step.Run(context.Background()))
	require.Contains(t, argvLines(f.calls), "sudo -E systemctl enable --now avahi-daemon")
}

func TestSmbStep_defaultGroupSmb(t *testing.T) {
	stubSeams(t, 1000, false)
	f := &fakeRunner{
		failures: map[string]error{
			"sudo -E getent group smb": errors.New("exit status 2"),
		},
	}
	step := &Step{Runner: f, Out: &bytes.Buffer{}, Plan: smbPlan(profile.SmbSpec{})}

	require.NoError(t, step.Run(context.Background()))
	lines := argvLines(f.calls)
	require.Equal(t, "sudo -E getent group smb", lines[0])
	require.Equal(t, "sudo -E groupadd smb", lines[1])
}

func TestSmbStep_emptyPlanNoop(t *testing.T) {
	stubSeams(t, 1000, false)
	f := &fakeRunner{}
	step := &Step{Runner: f, Out: &bytes.Buffer{}, Plan: resolve.SmbStep{}}

	require.NoError(t, step.Run(context.Background()))
	require.Empty(t, f.calls)
}
