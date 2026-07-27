package generate

import (
	"context"
	"encoding/json"
	"fmt"
	"os/exec"
	"slices"
	"strings"
)

// Volume is one local block-device filesystem discovered via lsblk.
type Volume struct {
	// UUID is the filesystem UUID, usable as a UUID= mount source.
	UUID string
	// FSType is the filesystem type as lsblk reports it (e.g. "btrfs",
	// "ntfs"), not necessarily a registry type key.
	FSType string
	// Label is the filesystem label; empty when unset.
	Label string
	// Size is lsblk's human-readable size (e.g. "949.9G").
	Size string
	// Mountpoints lists current mountpoints in lsblk order; empty when
	// the volume is not mounted.
	Mountpoints []string
	// Managed reports whether the volume's UUID already appears as a
	// UUID=<uuid> source in the existing mount sources passed to Volumes.
	Managed bool
}

// lsblkRun executes lsblk and returns its JSON listing. It is a
// package-level var so tests can stub the lookup (same seam style as
// KernelRelease).
var lsblkRun = func(ctx context.Context) ([]byte, error) {
	return exec.CommandContext(ctx, "lsblk",
		"-J", "-o", "NAME,UUID,FSTYPE,LABEL,SIZE,MOUNTPOINTS,TYPE").Output()
}

// Volumes lists local volumes discovered via lsblk, filtered to
// partitions (TYPE=part) with a non-empty UUID whose filesystem type the
// registry classifies as KindVolume — by entry type (btrfs) or by entry
// family (lsblk "ntfs" matches the ntfs family of ntfs3/ntfs-3g).
// Network-kind filesystems (nfs, cifs) and unknown types are excluded.
// sources marks already-managed volumes: an entry of the form
// "UUID=<uuid>" sets Managed on the matching volume. The result is
// sorted by UUID for deterministic output.
func Volumes(ctx context.Context, reg *Registry, sources []string) ([]Volume, error) {
	data, err := lsblkRun(ctx)
	if err != nil {
		return nil, fmt.Errorf("list block devices: %w", err)
	}
	vols, err := parseLsblk(data, reg, sources)
	if err != nil {
		return nil, err
	}
	return vols, nil
}

// lsblkDevice mirrors lsblk's JSON node; children nest disk→part trees.
type lsblkDevice struct {
	Name        string        `json:"name"`
	UUID        string        `json:"uuid"`
	FSType      string        `json:"fstype"`
	Label       string        `json:"label"`
	Size        string        `json:"size"`
	Mountpoints []string      `json:"mountpoints"`
	Type        string        `json:"type"`
	Children    []lsblkDevice `json:"children"`
}

// parseLsblk decodes lsblk output and extracts the managed-annotated
// volume list. JSON nulls unmarshal into Go strings as "", so nullable
// columns need no pointer plumbing.
func parseLsblk(data []byte, reg *Registry, sources []string) ([]Volume, error) {
	var out struct {
		BlockDevices []lsblkDevice `json:"blockdevices"`
	}
	if err := json.Unmarshal(data, &out); err != nil {
		return nil, fmt.Errorf("parse lsblk output: %w", err)
	}

	managed := make(map[string]bool, len(sources))
	for _, s := range sources {
		if uuid, ok := strings.CutPrefix(s, "UUID="); ok {
			managed[uuid] = true
		}
	}

	var vols []Volume
	var walk func(devs []lsblkDevice)
	walk = func(devs []lsblkDevice) {
		for _, d := range devs {
			if d.Type == "part" && d.UUID != "" && isVolumeFSType(reg, d.FSType) {
				vols = append(vols, Volume{
					UUID:        d.UUID,
					FSType:      d.FSType,
					Label:       d.Label,
					Size:        d.Size,
					Mountpoints: cleanMountpoints(d.Mountpoints),
					Managed:     managed[d.UUID],
				})
			}
			walk(d.Children)
		}
	}
	walk(out.BlockDevices)

	slices.SortFunc(vols, func(a, b Volume) int {
		return strings.Compare(a.UUID, b.UUID)
	})
	return vols, nil
}

// isVolumeFSType reports whether the registry classifies fstype as a
// local volume, matching either an entry's type or its family. An empty
// fstype is unclassifiable (and would otherwise match family-less
// entries like "default").
func isVolumeFSType(reg *Registry, fstype string) bool {
	if fstype == "" {
		return false
	}
	for _, e := range reg.Entries() {
		if e.Kind == KindVolume && (e.Type == fstype || e.Family == fstype) {
			return true
		}
	}
	return false
}

// cleanMountpoints drops null/empty entries: older util-linux emits
// mountpoints arrays containing null for unmounted child columns.
func cleanMountpoints(mps []string) []string {
	out := make([]string, 0, len(mps))
	for _, mp := range mps {
		if mp != "" {
			out = append(out, mp)
		}
	}
	return out
}
