package generate

import (
	"github.com/thedataflows/dotdrift/internal/facts"
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

// KernelRelease returns the running kernel release (`uname -r`). It aliases
// facts.KernelRelease (the one implementation); kept as a package-level var
// so existing tests can stub the lookup.
var KernelRelease = facts.KernelRelease
