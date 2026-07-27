package generate

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

// stubLsblk swaps the lsblkRun seam for the test duration.
func stubLsblk(t *testing.T, data []byte, err error) {
	t.Helper()
	prev := lsblkRun
	lsblkRun = func(context.Context) ([]byte, error) { return data, err }
	t.Cleanup(func() { lsblkRun = prev })
}

func readFixture(t *testing.T, name string) []byte {
	t.Helper()
	data, err := os.ReadFile(filepath.Join("testdata", name))
	require.NoError(t, err)
	return data
}

func loadEmbeddedRegistry(t *testing.T) *Registry {
	t.Helper()
	noOverride(t)
	reg, err := Load()
	require.NoError(t, err)
	return reg
}

func volumeUUIDs(vols []Volume) []string {
	out := make([]string, len(vols))
	for i, v := range vols {
		out[i] = v.UUID
	}
	return out
}

func findVolume(t *testing.T, vols []Volume, uuid string) Volume {
	t.Helper()
	for _, v := range vols {
		if v.UUID == uuid {
			return v
		}
	}
	t.Fatalf("volume %q not found in %v", uuid, volumeUUIDs(vols))
	return Volume{}
}

func TestLsblk_parsesRealFixture(t *testing.T) {
	stubLsblk(t, readFixture(t, "lsblk-real.json"), nil)
	reg := loadEmbeddedRegistry(t)

	vols, err := Volumes(context.Background(), reg, nil)
	require.NoError(t, err)

	// Recorded on the development machine: one vfat ESP and one btrfs
	// system partition under nvme0n1; the zram0 swap disk is excluded.
	require.Equal(t,
		[]string{"DDC5-1595", "d83cb072-48f1-4d25-bef5-147b27c41fb7"},
		volumeUUIDs(vols),
		"real fixture yields exactly the two partitions, sorted by UUID",
	)

	esp := findVolume(t, vols, "DDC5-1595")
	require.Equal(t, "vfat", esp.FSType)
	require.Equal(t, "", esp.Label)
	require.Equal(t, "4G", esp.Size)
	require.Equal(t, []string{"/boot"}, esp.Mountpoints)
	require.False(t, esp.Managed)

	sys := findVolume(t, vols, "d83cb072-48f1-4d25-bef5-147b27c41fb7")
	require.Equal(t, "btrfs", sys.FSType)
	require.Equal(t, "949.9G", sys.Size)
	require.Equal(t,
		[]string{"/var/log", "/home", "/var/cache", "/root", "/var/tmp", "/srv", "/"},
		sys.Mountpoints,
		"mountpoint order from lsblk is preserved",
	)
	t.Logf("real fixture: %d volumes parsed", len(vols))
}

// syntheticUUIDs is the expected include set of testdata/lsblk-synthetic.json,
// sorted by UUID (the documented Volumes ordering).
var syntheticUUIDs = []string{
	"AAAA-1111",             // sda1 vfat
	"CCCC3333DDDD4444",      // sda3 ntfs (family match)
	"bbbb2222-1111-4000-8000-000000000002", // sda2 btrfs, multi-mount
	"dddd4444-5555-4000-8000-000000000004", // sdb1 btrfs under second disk
}

func syntheticVolumes(t *testing.T, sources []string) []Volume {
	t.Helper()
	stubLsblk(t, readFixture(t, "lsblk-synthetic.json"), nil)
	vols, err := Volumes(context.Background(), loadEmbeddedRegistry(t), sources)
	require.NoError(t, err)
	return vols
}

func TestLsblk_filtersNonPartitions(t *testing.T) {
	vols := syntheticVolumes(t, nil)

	// disk (sda/sdb), rom (sr0), loop (loop0) and crypt (sdb1_crypt)
	// devices must never surface, however valid their fstype looks.
	require.Equal(t, syntheticUUIDs, volumeUUIDs(vols),
		"only TYPE=part devices survive, sorted by UUID")
	for _, excluded := range []string{
		"99999999-aaaa-4000-8000-000000000009", // sr0 rom
		"00000000-bbbb-4000-8000-000000000000", // loop0
		"88888888-9999-4000-8000-000000000008", // sdb1_crypt nested under a part
	} {
		require.NotContains(t, volumeUUIDs(vols), excluded)
	}
	t.Logf("kept %d partitions out of the whole device tree", len(vols))
}

func TestLsblk_skipsEmptyUUID(t *testing.T) {
	vols := syntheticVolumes(t, nil)

	for _, v := range vols {
		require.NotEmpty(t, v.UUID, "a partition without a UUID is unusable as a mount source")
	}
	require.Len(t, vols, len(syntheticUUIDs),
		"sda6 (vfat, empty UUID) must be dropped")
}

func TestLsblk_skipsUnknownFstype(t *testing.T) {
	vols := syntheticVolumes(t, nil)

	require.NotContains(t, volumeUUIDs(vols), "ffff6666-7777-4000-8000-000000000006",
		"sda5: zfs is in no registry entry, neither type nor family")
	require.NotContains(t, volumeUUIDs(vols), "aaaa7777-8888-4000-8000-000000000007",
		"sda7: null fstype cannot be classified")
}

func TestLsblk_skipsNetworkKind(t *testing.T) {
	vols := syntheticVolumes(t, nil)

	require.NotContains(t, volumeUUIDs(vols), "eeee5555-6666-4000-8000-000000000005",
		"sda4: nfs is a network-kind registry entry, not a local volume")
}

func TestLsblk_familyMatch(t *testing.T) {
	vols := syntheticVolumes(t, nil)

	// lsblk reports "ntfs"; the registry has no "ntfs" type, but both
	// ntfs3 and ntfs-3g declare family "ntfs" — the volume is kept.
	ntfs := findVolume(t, vols, "CCCC3333DDDD4444")
	require.Equal(t, "ntfs", ntfs.FSType)
	require.Equal(t, "Windows", ntfs.Label)
	require.Equal(t, "300G", ntfs.Size)
	require.Empty(t, ntfs.Mountpoints)
	t.Logf("ntfs fstype matched via family: %+v", ntfs)
}

func TestLsblk_marksAlreadyManaged(t *testing.T) {
	const btrfsUUID = "bbbb2222-1111-4000-8000-000000000002"

	t.Run("UUID source marks its volume", func(t *testing.T) {
		vols := syntheticVolumes(t, []string{
			"UUID=" + btrfsUUID,
			"UUID=deadbeef-0000-4000-8000-000000000000", // no such volume
			"LABEL=EFI",   // not a UUID= source: ignored
			"/dev/sda1",   // device-path source: ignored
		})

		for _, v := range vols {
			require.Equal(t, v.UUID == btrfsUUID, v.Managed,
				"volume %q Managed=%v", v.UUID, v.Managed)
		}
	})

	t.Run("empty sources leave everything unmanaged", func(t *testing.T) {
		vols := syntheticVolumes(t, nil)
		for _, v := range vols {
			require.False(t, v.Managed)
		}
	})
}

func TestLsblk_commandFailurePropagates(t *testing.T) {
	errBoom := errors.New("lsblk: exit status 1")
	stubLsblk(t, nil, errBoom)

	_, err := Volumes(context.Background(), loadEmbeddedRegistry(t), nil)
	require.ErrorIs(t, err, errBoom, "runner failure must propagate wrapped")
	t.Logf("runner error surfaced as: %v", err)
}

func TestLsblk_invalidJSONErrors(t *testing.T) {
	stubLsblk(t, []byte("{this is not json"), nil)

	_, err := Volumes(context.Background(), loadEmbeddedRegistry(t), nil)
	require.Error(t, err)
	require.Contains(t, err.Error(), "parse", "error must name the failing stage")
	t.Logf("invalid JSON surfaced as: %v", err)
}
