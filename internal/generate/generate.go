package generate

import (
	"fmt"
	"os/exec"
	"strings"
)

// Registry conventions shared by the generate package.
//
// Filesystem mount-option presets live in the embedded registry.toml
// (see registry.go). Options are stored verbatim from the reference
// script except for the bare tokens "uid" and "gid", which are
// placeholders: renderers substitute the invoking user's numeric ids
// (uid=<n>, gid=<n>) at render time. A user override file merges
// whole-entry by filesystem type — a [filesystems.<type>] table in the
// override fully replaces the embedded entry; there is no field-level
// merge.

// Filesystem kinds.
const (
	// KindVolume is a local block-device filesystem (e.g. btrfs, ntfs3).
	KindVolume = "volume"
	// KindNetwork is a remote export filesystem (e.g. nfs, cifs).
	KindNetwork = "network"
)

// KernelRelease returns the running kernel release (`uname -r`). It is a
// package-level var so tests can stub the lookup.
var KernelRelease = func() (string, error) {
	out, err := exec.Command("uname", "-r").Output()
	if err != nil {
		return "", fmt.Errorf("query kernel release: %w", err)
	}
	return strings.TrimSpace(string(out)), nil
}
