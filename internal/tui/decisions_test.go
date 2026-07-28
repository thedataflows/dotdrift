package tui

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/thedataflows/dotdrift/internal/generate"
)

func testRegistry(t *testing.T) *generate.Registry {
	t.Helper()
	reg, err := generate.Load()
	require.NoError(t, err)
	return reg
}

// Kind preselect: an explicit --type decides by registry kind, otherwise a
// network-shaped source implies network, else volume.
func TestDecisions_kindForDefaults(t *testing.T) {
	reg := testRegistry(t)
	for _, tc := range []struct {
		name     string
		defaults MountChoice
		want     string
	}{
		{"network type", MountChoice{Type: "nfs"}, generate.KindNetwork},
		{"volume type", MountChoice{Type: "btrfs"}, generate.KindVolume},
		{"family type", MountChoice{Type: "ntfs3"}, generate.KindVolume},
		{"network-shaped source", MountChoice{Source: "server:/export"}, generate.KindNetwork},
		{"cifs-shaped source", MountChoice{Source: "//server/share"}, generate.KindNetwork},
		{"uuid source", MountChoice{Source: "UUID=abc"}, generate.KindVolume},
		{"empty", MountChoice{}, generate.KindVolume},
	} {
		t.Run(tc.name, func(t *testing.T) {
			require.Equal(t, tc.want, kindForDefaults(reg, tc.defaults))
		})
	}
}

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

// First-volume flag prefill: the first picked volume takes the flag values
// with empty fields filled from the volume's own defaults; later volumes
// always use their own defaults. Source always comes from the volume.
func TestDecisions_prefillForVolume(t *testing.T) {
	vc := VolumeChoice{
		Volume:      generate.Volume{UUID: "u1", FSType: "btrfs", Label: "data"},
		Name:        "data",
		Destination: "/mnt/data",
		Type:        "btrfs",
	}
	full := MountChoice{Name: "win", Destination: "/mnt/win", Type: "ntfs3", Source: "UUID=flag"}

	got := prefillForVolume(vc, full, true)
	require.Equal(t, full.Name, got.Name)
	require.Equal(t, full.Destination, got.Destination)
	require.Equal(t, full.Type, got.Type)
	require.Equal(t, "UUID=u1", got.Source, "source always comes from the picked volume")

	partial := MountChoice{Name: "custom"}
	got = prefillForVolume(vc, partial, true)
	require.Equal(t, "custom", got.Name)
	require.Equal(t, vc.Destination, got.Destination, "empty fields fall back to the volume default")
	require.Equal(t, vc.Type, got.Type)

	got = prefillForVolume(vc, MountChoice{}, true)
	require.Equal(t, vc.Name, got.Name)
	require.Equal(t, vc.Destination, got.Destination)

	got = prefillForVolume(vc, full, false)
	require.Equal(t, vc.Name, got.Name, "non-first volumes ignore flag prefill")
	require.Equal(t, vc.Destination, got.Destination)
}

// Share loop end condition: declining to add another share ends the loop
// only when at least one share exists (CLI parity: no shareless modules).
func TestDecisions_shareLoopDone(t *testing.T) {
	require.False(t, shareLoopDone(false, 0), "decline with zero shares keeps prompting")
	require.True(t, shareLoopDone(false, 2), "decline with shares ends the loop")
	require.False(t, shareLoopDone(true, 0), "accept always continues")
	require.False(t, shareLoopDone(true, 2), "accept always continues")
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

// MountsWizard.Write rejects zero mounts (parity with the smb guard and
// the CLI's required --name).
func TestMountsWizard_writeRequiresMount(t *testing.T) {
	reg := testRegistry(t)
	w := NewMountsWizard(generate.Selection{Layer: generate.LayerBase, ModuleID: "mounts"}, reg)

	err := w.Write(t.TempDir(), 1000, 1000)
	require.Error(t, err, "zero mounts must not write a module (CLI parity)")
	require.Contains(t, err.Error(), "at least one mount")

	require.NoError(t, w.AddMount(MountChoice{Name: "x", Source: "s:/x", Destination: "/mnt/x", Type: "nfs"}))
	require.NoError(t, w.Write(t.TempDir(), 1000, 1000), "one mount writes fine")
}
