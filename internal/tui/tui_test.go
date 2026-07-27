package tui

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/BurntSushi/toml"
	"github.com/stretchr/testify/require"
	"github.com/thedataflows/dotdrift/internal/generate"
	"github.com/thedataflows/dotdrift/internal/profile"
)

// stubKernelRelease pins generate.KernelRelease for the test duration.
func stubKernelRelease(t *testing.T, release string) {
	t.Helper()
	orig := generate.KernelRelease
	generate.KernelRelease = func() (string, error) { return release, nil }
	t.Cleanup(func() { generate.KernelRelease = orig })
}

func TestWizard_networkSourceRegexValidation(t *testing.T) {
	valid := []string{
		"synology.local:/volume1/syn01",
		"192.168.1.10:/export",
		"//nas.local/media",
		"//192.168.1.10/share",
	}
	for _, s := range valid {
		t.Run("valid/"+s, func(t *testing.T) {
			require.NoError(t, ValidateNetworkSource(s))
		})
	}
	invalid := []string{
		"",
		"server",
		"server:",
		":/export",
		"//server",
		"//server/",
		"/local/path",
		"UUID=1234-5678",
	}
	for _, s := range invalid {
		t.Run("invalid/"+s, func(t *testing.T) {
			require.Error(t, ValidateNetworkSource(s), "%q must be rejected", s)
		})
	}
}

func TestWizard_kernelRecommendedTypeIsDefault(t *testing.T) {
	reg, err := generate.Load()
	require.NoError(t, err)

	for _, tc := range []struct {
		kernel string
		want   string
	}{
		{"7.2.1-arch1-1", "ntfs3"},
		{"6.9.3", "ntfs-3g"},
	} {
		t.Run(tc.kernel, func(t *testing.T) {
			stubKernelRelease(t, tc.kernel)
			vols := []generate.Volume{{UUID: "aaaa-bbbb", FSType: "ntfs", Label: "data"}}
			choices := VolumeChoices(reg, vols)
			require.Len(t, choices, 1)
			require.Equal(t, tc.want, choices[0].Type,
				"the kernel-recommended ntfs-family entry is the preselected type")
			wantEntry, err := reg.Recommend("ntfs")
			require.NoError(t, err)
			require.Equal(t, wantEntry.Type, choices[0].Type)
		})
	}
}

func TestWizard_prefillFromFlags(t *testing.T) {
	p := MountsParams{
		Profile:     "/tmp/prof",
		Layer:       "host",
		ModuleID:    "nas",
		Hostname:    "h1",
		Name:        "syn01",
		Source:      "synology.local:/volume1/syn01",
		Destination: "/mnt/synology/syn01",
		Type:        "nfs",
		Options:     []string{"rw", "soft"},
		StartAt:     "*-*-* 18:05:00",
		State:       "disabled",
	}
	pre := p.Prefill()
	require.Equal(t, MountChoice{
		Name:        "syn01",
		Source:      "synology.local:/volume1/syn01",
		Destination: "/mnt/synology/syn01",
		Type:        "nfs",
		Options:     []string{"rw", "soft"},
		StartAt:     "*-*-* 18:05:00",
		State:       "disabled",
	}, pre, "parsed input flags pre-fill the wizard's first mount defaults")
}

func TestWizard_alreadyManagedVolumesPreMarked(t *testing.T) {
	reg, err := generate.Load()
	require.NoError(t, err)
	stubKernelRelease(t, "7.2.0")

	vols := []generate.Volume{
		{UUID: "aaaa-1111", FSType: "btrfs", Label: "managed-one", Managed: true},
		{UUID: "bbbb-2222", FSType: "btrfs", Label: "free-one"},
	}
	choices := VolumeChoices(reg, vols)
	require.Len(t, choices, 2)
	require.Contains(t, choices[0].Label(), "managed",
		"an already-managed volume is marked in its option label")
	require.NotContains(t, choices[1].Label(), "managed",
		"an unmanaged volume carries no managed marker")
}

func TestWizard_multipleMountsSingleWrite(t *testing.T) {
	reg, err := generate.Load()
	require.NoError(t, err)
	root := t.TempDir()

	w := NewMountsWizard(generate.Selection{Layer: generate.LayerBase, ModuleID: "mounts"}, reg)
	require.NoError(t, w.AddMount(MountChoice{
		Name:        "syn01",
		Source:      "synology.local:/volume1/syn01",
		Destination: "/mnt/synology/syn01",
		Type:        "nfs",
	}))
	require.NoError(t, w.AddMount(MountChoice{
		Name:        "media",
		Source:      "//nas.local/media",
		Destination: "/mnt/media",
		Type:        "cifs",
		StartAt:     "*-*-* 18:05:00",
	}))

	// One write at the end: WriteModule replaces [mounts] wholesale, so
	// a mid-loop write would have wiped the first mount.
	require.NoError(t, w.Write(root, 1000, 1000))

	dir := filepath.Join(root, "modules", "mounts")
	var cfg profile.ModuleConfig
	_, err = toml.DecodeFile(filepath.Join(dir, "module.toml"), &cfg)
	require.NoError(t, err)
	require.Len(t, cfg.Mounts, 2, "a single final write keeps every accumulated mount")
	require.Equal(t, "nfs", cfg.Mounts["syn01"].Type)
	require.Equal(t, "enabled", cfg.Mounts["syn01"].State, "state defaults to enabled")
	require.Equal(t, "cifs", cfg.Mounts["media"].Type)
	require.Equal(t, "*-*-* 18:05:00", cfg.Mounts["media"].StartAt)

	for _, f := range []string{
		"mnt-synology-syn01.mount",
		"mnt-media.mount", "mnt-media.service", "mnt-media.timer",
	} {
		require.FileExists(t, filepath.Join(dir, f))
	}
}

func TestWizard_addMountValidation(t *testing.T) {
	reg, err := generate.Load()
	require.NoError(t, err)
	w := NewMountsWizard(generate.Selection{Layer: generate.LayerBase, ModuleID: "mounts"}, reg)

	require.Error(t, w.AddMount(MountChoice{Source: "s:/x", Destination: "/mnt/x", Type: "nfs"}),
		"missing name")
	require.Error(t, w.AddMount(MountChoice{Name: "x", Destination: "/mnt/x", Type: "nfs"}),
		"missing source")
	require.Error(t, w.AddMount(MountChoice{Name: "x", Source: "s:/x", Type: "nfs"}),
		"missing destination")
	require.Error(t, w.AddMount(MountChoice{Name: "x", Source: "s:/x", Destination: "/mnt/x", Type: "btrfs9000"}),
		"unknown filesystem type is rejected early")
	require.Error(t, w.AddMount(MountChoice{Name: "x", Source: "s:/x", Destination: "/mnt/x", Type: "nfs", State: "maybe"}),
		"unknown state is rejected early")

	// A valid mount whose profile root cannot be created surfaces at Write.
	bad := NewMountsWizard(generate.Selection{Layer: generate.LayerBase, ModuleID: "mounts"}, reg)
	require.NoError(t, bad.AddMount(MountChoice{Name: "x", Source: "s:/x", Destination: "/mnt/x", Type: "nfs"}))
	blocker := filepath.Join(t.TempDir(), "blocker")
	require.NoError(t, os.WriteFile(blocker, []byte("file"), 0o644))
	require.Error(t, bad.Write(blocker, 1000, 1000), "write under a file path fails")
}
