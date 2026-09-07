package profile

// SystemdSpec is the [systemd] table of a module.toml: declarative systemd
// USER units (services and timers), emitted as
// [bootstrap.linux.systemd.units] (issue 0048). Directive tables are
// passthrough — dotdrift validates structure only (kind derivation, required
// keys, name charset; see internal/resolve); mise owns directive semantics,
// so new mise directives work without a dotdrift schema change.
type SystemdSpec struct {
	Units map[string]SystemdUnit `toml:"units"`
}

// SystemdUnit is one user unit's passthrough directive table. Values keep
// their TOML types (string, bool, int64, []any, map[string]any).
type SystemdUnit map[string]any

// MountSpec describes a single filesystem attachment declared in
// module.toml under [mounts.<name>]. Resolve validates structure only
// (non-empty source/destination/type, known state); Type is never checked
// against any registry — the registry lives outside resolve and evolves
// independently.
type MountSpec struct {
	Source      string   `toml:"source"`
	Destination string   `toml:"destination"`
	Type        string   `toml:"type"`
	Options     []string `toml:"options"`
	StartAt     string   `toml:"startat"`
	State       string   `toml:"state"`
}

// ShareSpec describes one Samba share declared under [smb.shares.<name>].
// A share's path may coincide with a mount's destination, but shares and
// mounts are declared independently — no derivation exists between them.
type ShareSpec struct {
	Path       string `toml:"path"`
	Comment    string `toml:"comment"`
	ValidUsers string `toml:"valid_users"`
	Writable   bool   `toml:"writable"`
	Public     bool   `toml:"public"`
}

// SmbSpec is the [smb] table of a module.toml. Avahi is a *bool so an
// unset key (nil) is distinguishable from an explicit false; downstream
// consumers treat nil as the default (avahi enabled). Layers merge the
// scalar fields by replacement-when-set and Shares whole-entry by name
// (see internal/resolve).
type SmbSpec struct {
	Group  string               `toml:"group"`
	Users  []string             `toml:"users"`
	Avahi  *bool                `toml:"avahi"`
	Shares map[string]ShareSpec `toml:"shares"`
}
