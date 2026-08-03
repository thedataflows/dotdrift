package generate

import (
	"flag"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/thedataflows/dotdrift/internal/profile"
)

// updateGoldens rewrites golden files with the actual render output
// (go test ./internal/generate/ -update). Goldens derive from the
// reference templates in pimp-my-cachyos/mountpoints/ (.*.tpl) and the
// smb.sh shares heredoc; regenerate only after re-checking fidelity
// against envsubst-expanded reference output.
var updateGoldens = flag.Bool("update", false, "rewrite golden files with actual render output")

// requireGolden compares got against the checked-in golden file name.
func requireGolden(t *testing.T, name, got string) {
	t.Helper()
	path := filepath.Join("testdata", "golden", name)
	if *updateGoldens {
		require.NoError(t, os.WriteFile(path, []byte(got), 0o644))
	}
	want, err := os.ReadFile(path)
	require.NoError(t, err)
	require.Equal(t, string(want), got, "golden %s", name)
	t.Logf("golden %s matched (%d bytes)", name, len(got))
}

// mustEntry loads the embedded registry (no user override) and returns
// the preset for typ.
func mustEntry(t *testing.T, typ string) Entry {
	t.Helper()
	noOverride(t)
	reg, err := Load()
	require.NoError(t, err)
	e, ok := reg.Entry(typ)
	require.True(t, ok, "embedded registry must contain %q", typ)
	return e
}

// Given an NFS mount with a startat schedule and the embedded nfs
// preset, When the mount, service, and timer units are rendered, Then
// all three match the checked-in goldens derived from the reference
// templates (After= from the preset, oneshot wrapper, OnCalendar).
func TestRender_nfsMountWithTimer_golden(t *testing.T) {
	preset := mustEntry(t, "nfs")
	spec := profile.MountSpec{
		Source:      "synology.local:/volume1/syn01",
		Destination: "/mnt/synology/syn01",
		Type:        "nfs",
		StartAt:     "*-*-* 18:05:00",
	}
	name := "synology-syn01"
	stem := EscapePath(spec.Destination)

	requireGolden(t, "nfs.mount.golden", RenderMountUnit(name, spec, preset, 1000, 1000))
	requireGolden(t, "nfs.service.golden", RenderServiceUnit(stem+".mount"))
	requireGolden(t, "nfs.timer.golden", RenderTimerUnit(name, spec.StartAt, stem+".service"))
}

// Given a btrfs volume mount (no startat) and the embedded btrfs
// preset, When the mount unit is rendered, Then it matches the golden
// and carries no After= line (volume presets declare no dependencies).
func TestRender_volumeMountNoTimer_golden(t *testing.T) {
	preset := mustEntry(t, "btrfs")
	spec := profile.MountSpec{
		Source:      "UUID=2e768ab4-1e25-40c9-934e-b9e77af557ba",
		Destination: "/mnt/linux2",
		Type:        "btrfs",
	}

	got := RenderMountUnit("linux2", spec, preset, 1000, 1000)

	requireGolden(t, "volume.mount.golden", got)
	require.NotContains(t, got, "After=")
}

// Given an ntfs-family mount (ntfs3 on kernel >= 7.1, ntfs-3g on
// kernel < 7.1) and its registry preset, When the mount unit is
// rendered, Then Type= names the generic family "ntfs": the driver is
// chosen by the kernel/mount helper at mount time, not pinned in the
// unit. The preset-specific options still apply, and the driver-
// specific type still drives package selection at the writer layer.
func TestRender_ntfsFamilyRendersGenericType(t *testing.T) {
	for _, typ := range []string{"ntfs3", "ntfs-3g"} {
		t.Run(typ, func(t *testing.T) {
			preset := mustEntry(t, typ)
			spec := profile.MountSpec{
				Source:      "UUID=01D97D07D7C008F0",
				Destination: "/mnt/win/c",
				Type:        typ,
			}

			got := RenderMountUnit("win_c", spec, preset, 1000, 1000)

			require.Contains(t, got, "Type=ntfs\n",
				"ntfs-family mounts announce the generic ntfs type")
			require.NotContains(t, got, "Type=ntfs3\n")
			require.NotContains(t, got, "Type=ntfs-3g\n")
		})
	}
}

// Given a mount spec without options and a preset carrying bare uid/gid
// tokens, When the mount unit is rendered, Then the preset options are
// used with the tokens expanded to the numeric ids.
func TestRender_optionsFromPresetWhenOmitted(t *testing.T) {
	preset := mustEntry(t, "exfat")
	spec := profile.MountSpec{
		Source:      "UUID=7FFD-B6CF",
		Destination: "/mnt/ventoy",
		Type:        "exfat",
	}

	got := RenderMountUnit("ventoy", spec, preset, 1000, 1000)

	require.Contains(t, got, "Options=auto,rw,uid=1000,gid=1000,dmask=027,fmask=027,noatime\n")
}

// Given a mount spec with its own options, When the mount unit is
// rendered, Then the spec options win over the preset and bare uid/gid
// tokens still expand.
func TestRender_optionsOverrideWins(t *testing.T) {
	preset := mustEntry(t, "exfat")
	spec := profile.MountSpec{
		Source:      "UUID=7FFD-B6CF",
		Destination: "/mnt/ventoy",
		Type:        "exfat",
		Options:     []string{"ro", "uid", "gid", "noatime"},
	}

	got := RenderMountUnit("ventoy", spec, preset, 1000, 1000)

	require.Contains(t, got, "Options=ro,uid=1000,gid=1000,noatime\n")
	require.NotContains(t, got, "dmask", "preset options must not leak into an override render")
}

// Given network and volume presets, When mount units are rendered, Then
// a preset's After dependencies appear as a single space-separated
// After= line and volume presets produce none.
func TestRender_afterForNetworkTypes(t *testing.T) {
	spec := profile.MountSpec{
		Source:      "nas:/export",
		Destination: "/mnt/nas",
		Type:        "nfs",
	}
	tests := []struct {
		name      string
		preset    Entry
		wantAfter string
	}{
		{
			name:      "nfs preset dependencies",
			preset:    Entry{Type: "nfs", Kind: KindNetwork, After: []string{"network.target", "local-fs.target"}},
			wantAfter: "After=network.target local-fs.target\n",
		},
		{
			name:      "cifs preset dependencies",
			preset:    Entry{Type: "cifs", Kind: KindNetwork, After: []string{"network-online.target"}},
			wantAfter: "After=network-online.target\n",
		},
		{
			name:      "volume preset has none",
			preset:    Entry{Type: "btrfs", Kind: KindVolume, Options: []string{"auto", "rw", "relatime"}},
			wantAfter: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := RenderMountUnit("m", spec, tt.preset, 1000, 1000)
			if tt.wantAfter == "" {
				require.NotContains(t, got, "After=")
				return
			}
			require.Contains(t, got, tt.wantAfter)
		})
	}
}

// Given several shares exercising the comment and valid-users fallbacks
// plus public/writable rendering, When shares.conf is rendered, Then it
// matches the golden: stanzas sorted by share name, comment falling
// back to the share name, valid users falling back to @smb.
func TestRender_sharesConf_golden(t *testing.T) {
	smb := profile.SmbSpec{
		Shares: map[string]profile.ShareSpec{
			"photos": {Path: "/mnt/synology/photos", Comment: "Family photos", ValidUsers: "cri", Writable: true},
			"media":  {Path: "/mnt/synology/media", Public: true},
			"backup": {Path: "/mnt/backup", ValidUsers: "@backupops", Writable: true},
		},
	}

	requireGolden(t, "shares.conf.golden", RenderSharesConf(smb))
}

// Given an smb spec with an explicit group, When a share omits valid
// users, Then the fallback references that group.
func TestRender_sharesConf_groupFromSpec(t *testing.T) {
	smb := profile.SmbSpec{
		Group:  "family",
		Shares: map[string]profile.ShareSpec{"media": {Path: "/mnt/media"}},
	}

	got := RenderSharesConf(smb)

	require.Contains(t, got, "valid users = @family\n")
}

// Given an smb spec without shares, When shares.conf is rendered, Then
// the output is empty (no stray whitespace).
func TestRender_sharesConf_empty(t *testing.T) {
	require.Empty(t, RenderSharesConf(profile.SmbSpec{}))
}

// Given a destination needing systemd escaping, When the unit name is
// derived via EscapePath, Then it matches systemd-escape --path output
// while the rendered unit keeps the verbatim destination in Where= and
// the path conditions.
func TestRender_unitNameViaEscape(t *testing.T) {
	spec := profile.MountSpec{
		Source:      "UUID=7FFD-B6CF",
		Destination: "/mnt/My Disk",
		Type:        "exfat",
	}

	unitName := EscapePath(spec.Destination) + ".mount"
	require.Equal(t, `mnt-My\x20Disk.mount`, unitName)

	preset := Entry{Type: "exfat", Kind: KindVolume, Options: []string{"auto", "rw", "uid", "gid"}}
	got := RenderMountUnit("mydisk", spec, preset, 1000, 1000)
	require.Contains(t, got, "Where=/mnt/My Disk\n")
	require.Contains(t, got, "ConditionPathExists=/mnt/My Disk\n")
	require.Contains(t, got, "ConditionPathIsMountPoint=!/mnt/My Disk\n")
}
