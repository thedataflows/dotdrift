package tui

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/require"
)

// lsblk fallback decision: detection failure falls back only with user
// consent; an empty detection always falls back; otherwise the list is used.
func TestDecisions_volumePathAction(t *testing.T) {
	detectErr := errors.New("lsblk exploded")
	for _, tc := range []struct {
		name      string
		detectErr error
		volCount  int
		confirmed bool
		want      volumeAction
	}{
		{"error + consent", detectErr, 0, true, volumeManual},
		{"error + declined", detectErr, 0, false, volumeFail},
		{"empty detection", nil, 0, false, volumeManual},
		{"volumes found", nil, 3, false, volumeList},
	} {
		t.Run(tc.name, func(t *testing.T) {
			require.Equal(t, tc.want, volumePathAction(tc.detectErr, tc.volCount, tc.confirmed))
		})
	}
}

// Tab switching carries the target selection but never the module id —
// each wizard's own default module applies.
func TestDecisions_tabSwitchParams(t *testing.T) {
	mp := MountsParams{Profile: "/p", Layer: "host", ModuleID: "nas", Hostname: "h", Username: "u", Name: "x"}
	sp := smbParamsFromMounts(mp)
	require.Equal(t, "/p", sp.Profile)
	require.Equal(t, "host", sp.Layer)
	require.Equal(t, "h", sp.Hostname)
	require.Equal(t, "u", sp.Username)
	require.Empty(t, sp.ModuleID, "smb wizard uses its own default module id")

	mp2 := mountsParamsFromSmb(SmbParams{Profile: "/q", Layer: "user", ModuleID: "samba", Hostname: "h2", Username: "u2", Group: "g"})
	require.Equal(t, "/q", mp2.Profile)
	require.Equal(t, "user", mp2.Layer)
	require.Equal(t, "h2", mp2.Hostname)
	require.Equal(t, "u2", mp2.Username)
	require.Empty(t, mp2.ModuleID, "mounts wizard uses its own default module id")
}
