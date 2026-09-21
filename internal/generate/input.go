package generate

import (
	"fmt"
	"strings"

	"github.com/thedataflows/dotdrift/internal/profile"
)

// Shared spec assembly: every producer of a generate module — the
// flag-driven CLI (cmd) and the tui editors — builds Input through
// these helpers, which is what keeps contract invariant 15's
// single-path assembly true.

// MountsInput assembles a mounts Input, defaulting each mount's empty
// state to "enabled" (the CLI's --state default).
func MountsInput(mounts map[string]profile.MountSpec, uid, gid int) Input {
	out := make(map[string]profile.MountSpec, len(mounts))
	for name, spec := range mounts {
		if spec.State == "" {
			spec.State = "enabled"
		}
		out[name] = spec
	}
	return Input{Mounts: out, UID: uid, GID: gid}
}

// SmbInput assembles an smb Input, applying the shared defaults: an
// empty user list becomes the invoking user, an empty group becomes
// "smb". Avahi passes through untouched: nil keeps the default-on
// semantics, an explicit false records --no-avahi.
func SmbInput(group string, users []string, avahi *bool, shares map[string]profile.ShareSpec, defaultUser string, uid, gid int) Input {
	if len(users) == 0 {
		users = []string{defaultUser}
	}
	if group == "" {
		group = "smb"
	}
	return Input{
		Smb: &profile.SmbSpec{
			Group:  group,
			Users:  users,
			Avahi:  avahi,
			Shares: shares,
		},
		UID: uid,
		GID: gid,
	}
}

// ResolveWritable applies the shared writable-flag semantics: writable
// defaults on, an explicit --writable/--no-writable wins, and
// --readonly is the shorthand for --no-writable (winning when both are
// given).
func ResolveWritable(writableFlag *bool, readonly bool) bool {
	writable := true
	if writableFlag != nil {
		writable = *writableFlag
	}
	if readonly {
		writable = false
	}
	return writable
}

// ParseShareFlags validates and decodes --share name=path values into
// share specs with the resolved writable/public applied to every share.
// At least one share is required; both sides of the "=" must be
// non-empty.
func ParseShareFlags(raw []string, writable, public bool) (map[string]profile.ShareSpec, error) {
	if len(raw) == 0 {
		return nil, fmt.Errorf("generate smb: missing required flag: --share name=path (at least one share)")
	}
	shares := make(map[string]profile.ShareSpec, len(raw))
	for _, s := range raw {
		name, path, ok := strings.Cut(s, "=")
		if !ok || name == "" || path == "" {
			return nil, fmt.Errorf("generate smb: invalid --share %q: want name=path with both name and path non-empty", s)
		}
		shares[name] = profile.ShareSpec{Path: path, Writable: writable, Public: public}
	}
	return shares, nil
}
