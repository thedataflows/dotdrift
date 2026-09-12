package generate

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func testRegistry(t *testing.T) *Registry {
	t.Helper()
	reg, err := Load()
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
		{"network type", MountChoice{Type: "nfs"}, KindNetwork},
		{"volume type", MountChoice{Type: "btrfs"}, KindVolume},
		{"family type", MountChoice{Type: "ntfs3"}, KindVolume},
		{"network-shaped source", MountChoice{Source: "server:/export"}, KindNetwork},
		{"cifs-shaped source", MountChoice{Source: "//server/share"}, KindNetwork},
		{"uuid source", MountChoice{Source: "UUID=abc"}, KindVolume},
		{"empty", MountChoice{}, KindVolume},
	} {
		t.Run(tc.name, func(t *testing.T) {
			require.Equal(t, tc.want, KindForDefaults(reg, tc.defaults))
		})
	}
}

// First-volume flag prefill: the first picked volume takes the flag values
// with empty fields filled from the volume's own defaults; later volumes
// always use their own defaults. Source always comes from the volume.
func TestDecisions_prefillForVolume(t *testing.T) {
	vc := VolumeChoice{
		Volume:      Volume{UUID: "u1", FSType: "btrfs", Label: "data"},
		Name:        "data",
		Destination: "/mnt/data",
		Type:        "btrfs",
	}
	full := MountChoice{Name: "win", Destination: "/mnt/win", Type: "ntfs3", Source: "UUID=flag"}

	got := PrefillForVolume(vc, full, true)
	require.Equal(t, full.Name, got.Name)
	require.Equal(t, full.Destination, got.Destination)
	require.Equal(t, full.Type, got.Type)
	require.Equal(t, "UUID=u1", got.Source, "source always comes from the picked volume")

	partial := MountChoice{Name: "custom"}
	got = PrefillForVolume(vc, partial, true)
	require.Equal(t, "custom", got.Name)
	require.Equal(t, vc.Destination, got.Destination, "empty fields fall back to the volume default")
	require.Equal(t, vc.Type, got.Type)

	got = PrefillForVolume(vc, MountChoice{}, true)
	require.Equal(t, vc.Name, got.Name)
	require.Equal(t, vc.Destination, got.Destination)

	got = PrefillForVolume(vc, full, false)
	require.Equal(t, vc.Name, got.Name, "non-first volumes ignore flag prefill")
	require.Equal(t, vc.Destination, got.Destination)
}

// Share loop end condition: declining to add another share ends the loop
// only when at least one share exists (CLI parity: no shareless modules).
func TestDecisions_shareLoopDone(t *testing.T) {
	require.False(t, ShareLoopDone(false, 0), "decline with zero shares keeps prompting")
	require.True(t, ShareLoopDone(false, 2), "decline with shares ends the loop")
	require.False(t, ShareLoopDone(true, 0), "accept always continues")
	require.False(t, ShareLoopDone(true, 2), "accept always continues")
}

// MountsWizard.Write rejects zero mounts (parity with the smb guard and
// the CLI's required --name).
func TestMountsWizard_writeRequiresMount(t *testing.T) {
	reg := testRegistry(t)
	w := NewMountsWizard(Selection{Layer: LayerBase, ModuleID: "mounts"}, reg)

	err := w.Write(t.TempDir(), 1000, 1000)
	require.Error(t, err, "zero mounts must not write a module (CLI parity)")
	require.Contains(t, err.Error(), "at least one mount")

	require.NoError(t, w.AddMount(MountChoice{Name: "x", Source: "s:/x", Destination: "/mnt/x", Type: "nfs"}))
	require.NoError(t, w.Write(t.TempDir(), 1000, 1000), "one mount writes fine")
}
