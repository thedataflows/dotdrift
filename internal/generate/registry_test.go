package generate

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

// noOverride isolates a test from any real user override file by pointing
// the override path at a file that does not exist.
func noOverride(t *testing.T) {
	t.Helper()
	t.Setenv(OverrideEnvVar, filepath.Join(t.TempDir(), "absent.toml"))
}

// stubKernelRelease swaps the KernelRelease seam for the test duration.
func stubKernelRelease(t *testing.T, release string) {
	t.Helper()
	prev := KernelRelease
	KernelRelease = func() (string, error) { return release, nil }
	t.Cleanup(func() { KernelRelease = prev })
}

// writeOverride writes a user registry override file and points the
// resolution env var at it.
func writeOverride(t *testing.T, content string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "generate.toml")
	require.NoError(t, os.WriteFile(path, []byte(content), 0o644))
	t.Setenv(OverrideEnvVar, path)
	return path
}

func TestRegistry_embeddedLoads(t *testing.T) {
	noOverride(t)
	reg, err := Load()
	require.NoError(t, err)

	tests := []struct {
		typ           string
		kind          string
		family        string
		packages      []string
		recommendedIf string
	}{
		{"default", "volume", "", nil, ""},
		{"btrfs", "volume", "", nil, ""},
		{"exfat", "volume", "", nil, ""},
		{"vfat", "volume", "", nil, ""},
		{"ntfs3", "volume", "ntfs", []string{"ntfsprogs-plus"}, "kernel >= 7.1"},
		{"ntfs-3g", "volume", "ntfs", []string{"ntfs-3g"}, "kernel < 7.1"},
		{"nfs", "network", "", []string{"nfs-utils", "nfsidmap"}, ""},
		{"cifs", "network", "", []string{"cifs-utils"}, ""},
	}

	for _, tt := range tests {
		t.Run(tt.typ, func(t *testing.T) {
			e, ok := reg.Entry(tt.typ)
			require.True(t, ok, "embedded registry must contain %q", tt.typ)
			require.Equal(t, tt.typ, e.Type)
			require.Equal(t, tt.kind, e.Kind)
			require.Equal(t, tt.family, e.Family)
			require.Equal(t, tt.packages, e.Packages)
			require.Equal(t, tt.recommendedIf, e.RecommendedIf)
			require.NotEmpty(t, e.Options)
		})
	}

	nfs, ok := reg.Entry("nfs")
	require.True(t, ok)
	require.Equal(t, []string{"network.target", "local-fs.target"}, nfs.After)

	_, ok = reg.Entry("nonexistent")
	require.False(t, ok)

	require.Len(t, reg.Entries(), len(tests), "embedded registry must hold exactly the expected types")
}

func TestRegistry_presetOptionsMatchReferenceScript(t *testing.T) {
	noOverride(t)
	reg, err := Load()
	require.NoError(t, err)

	// Golden source of truth: the option strings from the reference script
	// (pimp-my-cachyos mise-tasks/system/mountpoints.sh), verbatim.
	// The registry stores the same options with $(id -u)/$(id -g) replaced
	// by the bare tokens "uid"/"gid" (substituted at render time).
	golden := map[string]string{
		"ntfs3":   "auto,rw,uid=$(id -u),gid=$(id -g),dmask=027,fmask=027,dev,exec,noatime,iocharset=utf8,windows_names,suid,discard",
		"ntfs-3g": "auto,rw,uid=$(id -u),gid=$(id -g),dmask=027,fmask=027,dev,exec,noatime,iocharset=utf8,windows_names,big_writes,suid",
		"exfat":   "auto,rw,uid=$(id -u),gid=$(id -g),dmask=027,fmask=027,noatime",
		"vfat":    "auto,rw,uid=$(id -u),gid=$(id -g),dmask=027,fmask=027,noatime",
		"nfs":     "rw,nosuid,soft,nfsvers=4,noacl,async,nocto,nconnect=16,_netdev,timeo=3,retrans=2,bg",
		// btrfs and default fall into the script's default branch.
		"btrfs":   "auto,rw,relatime",
		"default": "auto,rw,relatime",
		// cifs has no script preset; these are the task-specified
		// sensible defaults.
		"cifs": "rw,nosuid,soft,vers=3.0,_netdev,noacl",
	}

	for typ, scriptOptions := range golden {
		t.Run(typ, func(t *testing.T) {
			want := strings.Split(scriptOptions, ",")
			for i, opt := range want {
				switch opt {
				case "uid=$(id -u)":
					want[i] = "uid"
				case "gid=$(id -g)":
					want[i] = "gid"
				}
			}
			e, ok := reg.Entry(typ)
			require.True(t, ok, "embedded registry must contain %q", typ)
			require.Equal(t, want, e.Options)
		})
	}
}

func TestRegistry_userOverrideMergesWholeEntry(t *testing.T) {
	writeOverride(t, `
[filesystems.nfs]
kind = "network"
options = ["ro"]
packages = ["custom-nfs"]
`)
	reg, err := Load()
	require.NoError(t, err)

	nfs, ok := reg.Entry("nfs")
	require.True(t, ok)
	require.Equal(t, []string{"ro"}, nfs.Options, "override replaces options wholesale")
	require.Equal(t, []string{"custom-nfs"}, nfs.Packages)
	require.Empty(t, nfs.After, "whole-entry merge: fields absent from the override are gone")

	cifs, ok := reg.Entry("cifs")
	require.True(t, ok)
	require.Equal(t, []string{"rw", "nosuid", "soft", "vers=3.0", "_netdev", "noacl"}, cifs.Options,
		"entries not mentioned in the override stay untouched")
}

func TestRegistry_userOverrideUnknownTypeAdded(t *testing.T) {
	writeOverride(t, `
[filesystems.ceph]
kind = "network"
options = ["rw", "_netdev"]
packages = ["ceph-utils"]
`)
	reg, err := Load()
	require.NoError(t, err)

	ceph, ok := reg.Entry("ceph")
	require.True(t, ok, "override must be able to add a brand-new type")
	require.Equal(t, "network", ceph.Kind)
	require.Equal(t, []string{"rw", "_netdev"}, ceph.Options)
	require.Equal(t, []string{"ceph-utils"}, ceph.Packages)

	_, ok = reg.Entry("nfs")
	require.True(t, ok, "embedded entries survive an additive override")
}

func TestRecommend_kernelOps(t *testing.T) {
	tests := []struct {
		name    string
		expr    string
		release string
		want    bool
	}{
		{"lt true", "kernel < 7.1", "6.12", true},
		{"lt false at boundary", "kernel < 7.1", "7.1", false},
		{"lt false above", "kernel < 7.1", "7.10", false},
		{"lte boundary", "kernel <= 7.1", "7.1", true},
		{"lte zero-padded release", "kernel <= 7.1", "7.1.0", true},
		{"lte below", "kernel <= 7.1", "6.12", true},
		{"lte above", "kernel <= 7.1", "7.2", false},
		{"gt numeric segment compare", "kernel > 7.1", "7.10", true},
		{"gt false at boundary", "kernel > 7.1", "7.1", false},
		{"gte boundary", "kernel >= 7.1", "7.1", true},
		{"gte below", "kernel >= 7.1", "6.12", false},
		{"gte numeric segment compare", "kernel >= 7.1", "7.10", true},
		{"eq", "kernel == 7.1", "7.1", true},
		{"eq false", "kernel == 7.1", "7.2", false},
		{"ne", "kernel != 7.1", "6.12", true},
		{"ne false", "kernel != 7.1", "7.1", false},
		{"distro-suffixed release", "kernel < 7.1", "6.12.1-arch1-1", true},
		{"multi-segment ordering", "kernel < 7.1", "6.12", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := evalRecommended(tt.expr, tt.release)
			require.NoError(t, err)
			require.Equal(t, tt.want, got)
		})
	}
}

func TestRecommend_kernelSeamInjected(t *testing.T) {
	noOverride(t)
	reg, err := Load()
	require.NoError(t, err)

	stubKernelRelease(t, "6.12.68-1-lts")
	e, err := reg.Recommend("ntfs")
	require.NoError(t, err)
	require.Equal(t, "ntfs-3g", e.Type, "kernel < 7.1 selects ntfs-3g")

	stubKernelRelease(t, "7.1.2-arch1-1")
	e, err = reg.Recommend("ntfs")
	require.NoError(t, err)
	require.Equal(t, "ntfs3", e.Type, "kernel >= 7.1 selects ntfs3")
}

func TestRecommend_unconstrainedEntryAlwaysEligible(t *testing.T) {
	noOverride(t)
	stubKernelRelease(t, "6.12.1-arch1-1")
	reg, err := Load()
	require.NoError(t, err)

	e, err := reg.Recommend("nfs")
	require.NoError(t, err)
	require.Equal(t, "nfs", e.Type)
}

func TestRecommend_unknownFamily(t *testing.T) {
	noOverride(t)
	stubKernelRelease(t, "6.12.1-arch1-1")
	reg, err := Load()
	require.NoError(t, err)

	_, err = reg.Recommend("plan9")
	require.ErrorIs(t, err, ErrNoRecommendation)
}

func TestRecommend_malformedExpressionErrors(t *testing.T) {
	t.Run("evaluator rejects non-kernel expressions", func(t *testing.T) {
		for _, expr := range []string{
			"cpu >= 1",
			"kernel >> 7.1",
			"kernel >=",
			"kernel",
			"kernel >= seven",
			"kernel >= 7.1 extra",
			"",
		} {
			_, err := evalRecommended(expr, "7.1")
			require.Error(t, err, "expression %q must be rejected loudly", expr)
		}
	})

	t.Run("Recommend surfaces override expression errors", func(t *testing.T) {
		writeOverride(t, `
[filesystems.ntfs3]
kind = "volume"
family = "ntfs"
options = ["auto"]
recommended_if = "kernel ~ 7.1"
`)
		stubKernelRelease(t, "7.1.0")
		reg, err := Load()
		require.NoError(t, err)

		_, err = reg.Recommend("ntfs")
		require.Error(t, err)
		require.Contains(t, err.Error(), "ntfs3")
	})
}

func TestRegistry_malformedTOMLErrors(t *testing.T) {
	path := writeOverride(t, "this is [not valid toml")
	_, err := Load()
	require.Error(t, err)
	require.Contains(t, err.Error(), path, "error must name the offending file")
}

func TestRegistry_ntfs3AbsentCarriesRemoval(t *testing.T) {
	noOverride(t)
	reg, err := Load()
	require.NoError(t, err)
	e, ok := reg.Entry("ntfs3")
	require.True(t, ok)
	require.Equal(t, []string{"ntfs-3g"}, e.Absent,
		"ntfs3 preset carries the legacy ntfs-3g removal (kernel >= 7.1)")
}
