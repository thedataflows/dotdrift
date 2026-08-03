package generate

import (
	"fmt"
	"maps"
	"slices"
	"strings"

	"github.com/thedataflows/dotdrift/internal/profile"
)

// Pure renderers from resolved specs to file content. No I/O: callers
// decide file names and placement. Content follows the reference
// templates in pimp-my-cachyos (mountpoints/.mount.tpl, .service.tpl,
// .timer.tpl, and the smb.sh shares heredoc), byte-checked against
// envsubst-expanded reference output.

// RenderMountUnit renders the systemd .mount unit content for the mount
// named name. The unit's file name is derived by the caller as
// EscapePath(spec.Destination)+".mount" (systemd requires mount units
// to be named after their mount point).
//
// A single "After=<deps>" line listing preset.After space-separated is
// emitted only when the preset declares dependencies (network presets;
// the reference script set AFTER only for nfs). Options resolve to
// spec.Options when non-empty, else preset.Options, with the bare
// tokens "uid"/"gid" expanded to uid=<uid>/gid=<gid> (see generate.go)
// and joined with commas.
func RenderMountUnit(name string, spec profile.MountSpec, preset Entry, uid, gid int) string {
	var b strings.Builder
	b.WriteString("[Unit]\n")
	fmt.Fprintf(&b, "Description=Local mount unit for %s\n", name)
	if len(preset.After) > 0 {
		fmt.Fprintf(&b, "After=%s\n", strings.Join(preset.After, " "))
	}
	fmt.Fprintf(&b, "ConditionPathExists=%s\n", spec.Destination)
	fmt.Fprintf(&b, "ConditionPathIsMountPoint=!%s\n", spec.Destination)
	b.WriteString("\n[Mount]\n")
	fmt.Fprintf(&b, "What=%s\n", spec.Source)
	fmt.Fprintf(&b, "Where=%s\n", spec.Destination)
	fmt.Fprintf(&b, "Type=%s\n", unitType(spec, preset))
	fmt.Fprintf(&b, "Options=%s\n", resolveOptions(spec, preset, uid, gid))
	b.WriteString("\n[Install]\nWantedBy=multi-user.target\n")
	return b.String()
}

// RenderServiceUnit renders the oneshot .service wrapper that starts
// and stops the mount unit unitName (e.g. "mnt-synology-syn01.mount")
// and remains active so the mount survives the one-shot start. The
// caller writes the content to the same stem with a .service suffix.
func RenderServiceUnit(unitName string) string {
	var b strings.Builder
	b.WriteString("[Unit]\n")
	fmt.Fprintf(&b, "Description=Service to manage %s\n", unitName)
	b.WriteString("After=network-online.target\nRequires=network-online.target\n")
	b.WriteString("\n[Service]\nType=oneshot\n")
	fmt.Fprintf(&b, "ExecStart=/usr/bin/systemctl start %s\n", unitName)
	fmt.Fprintf(&b, "ExecStop=/usr/bin/systemctl stop %s\n", unitName)
	b.WriteString("RemainAfterExit=yes\nTimeoutStartSec=3\n")
	b.WriteString("\n[Install]\nWantedBy=multi-user.target\n")
	return b.String()
}

// RenderTimerUnit renders the .timer unit that activates the service
// unitName (e.g. "mnt-synology-syn01.service") on the OnCalendar
// expression startAt for the mount named name. Render it only when the
// mount's StartAt is non-empty — the caller decides and writes the
// content to the same stem with a .timer suffix so systemd pairs it
// with the service unit.
//
// The Description references unitName rather than "<name>.service":
// dotdrift names service units by escaped destination stem, so the
// reference template's $NAME slot corresponds to the activated service
// unit — using the mount name there would point at a unit that does
// not exist. name identifies the mount for the caller (file naming,
// logging); the unit content carries only the service reference.
func RenderTimerUnit(name, startAt, unitName string) string {
	var b strings.Builder
	b.WriteString("[Unit]\n")
	fmt.Fprintf(&b, "Description=Time-based mount attempt for %s\n", unitName)
	b.WriteString("\n[Timer]\n")
	fmt.Fprintf(&b, "OnCalendar=%s\n", startAt)
	b.WriteString("RandomizedDelaySec=5\nPersistent=true\n")
	b.WriteString("\n[Install]\nWantedBy=timers.target\n")
	return b.String()
}

// RenderSharesConf renders a Samba shares.conf with one [name] stanza
// per declared share, sorted by share name for byte-deterministic
// output and terminated by a single trailing newline. An empty Comment
// falls back to the share name (the reference script wrote
// "comment = $NAME"); an empty ValidUsers falls back to @<group> with
// the group defaulting to "smb" (the reference's SMB_DEFAULT_GROUP).
// An smb spec without shares renders empty.
func RenderSharesConf(smb profile.SmbSpec) string {
	group := smb.Group
	if group == "" {
		group = "smb"
	}
	var b strings.Builder
	for i, name := range slices.Sorted(maps.Keys(smb.Shares)) {
		if i > 0 {
			b.WriteByte('\n')
		}
		share := smb.Shares[name]
		comment := share.Comment
		if comment == "" {
			comment = name
		}
		validUsers := share.ValidUsers
		if validUsers == "" {
			validUsers = "@" + group
		}
		fmt.Fprintf(&b, "[%s]\n", name)
		fmt.Fprintf(&b, "comment = %s\n", comment)
		fmt.Fprintf(&b, "path = %s\n", share.Path)
		fmt.Fprintf(&b, "valid users = %s\n", validUsers)
		fmt.Fprintf(&b, "public = %s\n", yesNo(share.Public))
		fmt.Fprintf(&b, "writable = %s\n", yesNo(share.Writable))
	}
	return b.String()
}

// unitType is the Type= the .mount unit announces. A preset that
// declares a Family names the generic filesystem (e.g. "ntfs" for the
// ntfs3/ntfs-3g drivers): the driver is selected by the kernel/mount
// helper at mount time, so the unit must not pin a driver. The
// driver-specific spec.Type still drives registry lookup and package
// selection at the writer layer.
func unitType(spec profile.MountSpec, preset Entry) string {
	if preset.Family != "" {
		return preset.Family
	}
	return spec.Type
}

// resolveOptions picks the effective mount-option list — the spec's own
// options when given, else the preset's — expands the bare uid/gid
// tokens, and joins with commas.
func resolveOptions(spec profile.MountSpec, preset Entry, uid, gid int) string {
	opts := spec.Options
	if len(opts) == 0 {
		opts = preset.Options
	}
	out := make([]string, len(opts))
	for i, o := range opts {
		switch o {
		case "uid":
			o = fmt.Sprintf("uid=%d", uid)
		case "gid":
			o = fmt.Sprintf("gid=%d", gid)
		}
		out[i] = o
	}
	return strings.Join(out, ",")
}

// yesNo renders a bool as Samba's yes/no vocabulary.
func yesNo(v bool) string {
	if v {
		return "yes"
	}
	return "no"
}
