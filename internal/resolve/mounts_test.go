package resolve_test

import (
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/thedataflows/dotdrift/internal/facts"
	"github.com/thedataflows/dotdrift/internal/profile"
	"github.com/thedataflows/dotdrift/internal/resolve"
)

func mountEntryByName(t *testing.T, entries []resolve.MountEntry, name string) resolve.MountEntry {
	t.Helper()
	for _, e := range entries {
		if e.Name == name {
			return e
		}
	}
	t.Fatalf("mount entry %q not found", name)
	return resolve.MountEntry{}
}

func smbByModule(t *testing.T, mods []resolve.SmbModuleSpec, module string) resolve.SmbModuleSpec {
	t.Helper()
	for _, m := range mods {
		if m.Module == module {
			return m
		}
	}
	t.Fatalf("smb spec for module %q not found", module)
	return resolve.SmbModuleSpec{}
}

// Mounts merge whole-entry by name: a higher layer's [mounts.<name>] fully
// replaces the lower layer's entry — identical winner-map semantics to
// dotfiles per-target, with no field-level merge.
func TestMergeMounts_wholeEntryByName(t *testing.T) {
	root := t.TempDir()
	writeModule(t, root, "mnt", `
[mounts.a]
source = "UUID=base-a"
destination = "/mnt/a"
type = "ext4"
options = ["noatime"]

[mounts.b]
source = "UUID=base-b"
destination = "/mnt/b"
type = "btrfs"
`)
	writeOverlayModule(t, root, "hosts", "myhost", "mnt", `
[mounts.a]
source = "UUID=host-a"
destination = "/mnt/a"
type = "ext4"
`)
	writeOverlayModule(t, root, "users", "cri", "mnt", `
[mounts.c]
source = "//server/share"
destination = "/mnt/c"
type = "cifs"
`)

	plan, err := loadAndResolve(t, root, &facts.Facts{Hostname: "myhost", Username: "cri"})
	require.NoError(t, err)
	require.Len(t, plan.Mounts.Entries, 3)

	a := mountEntryByName(t, plan.Mounts.Entries, "a")
	require.Equal(t, "UUID=host-a", a.Spec.Source, "higher layer entry wins wholesale")
	require.Empty(t, a.Spec.Options, "no field-level merge: host entry did not declare options")
	require.Equal(t, "host", a.Layer)

	b := mountEntryByName(t, plan.Mounts.Entries, "b")
	require.Equal(t, "UUID=base-b", b.Spec.Source, "base-only entry survives")
	require.Equal(t, "base", b.Layer)

	c := mountEntryByName(t, plan.Mounts.Entries, "c")
	require.Equal(t, "user", c.Layer)
}

// Smb shares merge whole-entry by name like mounts; scalar fields (group,
// users, avahi) replace when the higher layer sets them — users is replaced
// wholesale, never appended (the deliberate contrast to hooks).
func TestMergeSmb_sharesWholeEntryAndScalarReplace(t *testing.T) {
	root := t.TempDir()
	writeModule(t, root, "srv", `
[smb]
group = "BASEWG"
users = ["base-user"]

[smb.shares.media]
path = "/srv/media"
comment = "base media"
writable = true

[smb.shares.baseonly]
path = "/srv/base"
`)
	writeOverlayModule(t, root, "hosts", "myhost", "srv", `
[smb]
group = "HOSTWG"
avahi = false

[smb.shares.media]
path = "/srv/media-host"
`)

	plan, err := loadAndResolve(t, root, &facts.Facts{Hostname: "myhost", Username: "cri"})
	require.NoError(t, err)
	require.Len(t, plan.Smb.Modules, 1)

	smb := smbByModule(t, plan.Smb.Modules, "srv")
	require.Equal(t, "HOSTWG", smb.Spec.Group, "higher-layer group replaces")
	require.Equal(t, []string{"base-user"}, smb.Spec.Users, "host layer did not set users: base value survives (replace when set)")
	require.NotNil(t, smb.Spec.Avahi, "explicit avahi = false must be distinguishable from unset")
	require.False(t, *smb.Spec.Avahi)

	media := smb.Spec.Shares["media"]
	require.Equal(t, "/srv/media-host", media.Path, "share replaced whole-entry by name")
	require.Empty(t, media.Comment, "no field-level merge: host share did not declare comment")
	require.False(t, media.Writable, "no field-level merge: host share did not declare writable")
	require.Contains(t, smb.Spec.Shares, "baseonly", "base-only share survives")
}

// A higher-layer users list replaces the lower layer's list wholesale; it
// is never appended (unlike hooks, which append across layers).
func TestMergeSmb_usersReplaceNotAppend(t *testing.T) {
	root := t.TempDir()
	writeModule(t, root, "srv", `
[smb]
users = ["base-a", "base-b"]
`)
	writeOverlayModule(t, root, "users", "cri", "srv", `
[smb]
users = ["user-c"]
`)

	plan, err := loadAndResolve(t, root, &facts.Facts{Hostname: "myhost", Username: "cri"})
	require.NoError(t, err)

	smb := smbByModule(t, plan.Smb.Modules, "srv")
	require.Equal(t, []string{"user-c"}, smb.Spec.Users, "user layer replaces the list, no append")
}

func TestResolveMounts_missingSourceErrors(t *testing.T) {
	root := t.TempDir()
	writeModule(t, root, "mnt", `
[mounts.x]
destination = "/mnt/x"
type = "ext4"
`)
	_, err := loadAndResolve(t, root, &facts.Facts{Hostname: "h", Username: "u"})
	require.Error(t, err)
	require.Contains(t, err.Error(), "mnt", "error must name the module")
	require.Contains(t, err.Error(), "x", "error must name the mount")
	require.Contains(t, err.Error(), "source", "error must name the missing field")
}

func TestResolveMounts_missingDestinationErrors(t *testing.T) {
	root := t.TempDir()
	writeModule(t, root, "mnt", `
[mounts.x]
source = "UUID=1"
type = "ext4"
`)
	_, err := loadAndResolve(t, root, &facts.Facts{Hostname: "h", Username: "u"})
	require.Error(t, err)
	require.Contains(t, err.Error(), "mnt")
	require.Contains(t, err.Error(), "x")
	require.Contains(t, err.Error(), "destination")
}

func TestResolveMounts_missingTypeErrors(t *testing.T) {
	root := t.TempDir()
	writeModule(t, root, "mnt", `
[mounts.x]
source = "UUID=1"
destination = "/mnt/x"
`)
	_, err := loadAndResolve(t, root, &facts.Facts{Hostname: "h", Username: "u"})
	require.Error(t, err)
	require.Contains(t, err.Error(), "mnt")
	require.Contains(t, err.Error(), "x")
	require.Contains(t, err.Error(), "type")
}

func TestResolveMounts_unknownStateErrors(t *testing.T) {
	root := t.TempDir()
	writeModule(t, root, "mnt", `
[mounts.x]
source = "UUID=1"
destination = "/mnt/x"
type = "ext4"
state = "paused"
`)
	_, err := loadAndResolve(t, root, &facts.Facts{Hostname: "h", Username: "u"})
	require.Error(t, err, "only \"\" / enabled / disabled are valid state values")
	require.Contains(t, err.Error(), "mnt")
	require.Contains(t, err.Error(), "x")
	require.Contains(t, err.Error(), "paused")
}

// Type is NEVER validated against a registry: the registry lives outside
// resolve and evolves independently, so an unknown type resolves fine.
func TestResolveMounts_unknownTypePasses(t *testing.T) {
	root := t.TempDir()
	writeModule(t, root, "mnt", `
[mounts.z]
source = "tank/data"
destination = "/mnt/z"
type = "zfs"
`)
	plan, err := loadAndResolve(t, root, &facts.Facts{Hostname: "h", Username: "u"})
	require.NoError(t, err, "resolve must not validate mount type against any registry")
	require.Len(t, plan.Mounts.Entries, 1)
	require.Equal(t, "zfs", plan.Mounts.Entries[0].Spec.Type)
}

func TestResolveSmb_shareMissingPathErrors(t *testing.T) {
	root := t.TempDir()
	writeModule(t, root, "srv", `
[smb.shares.media]
comment = "no path"
`)
	_, err := loadAndResolve(t, root, &facts.Facts{Hostname: "h", Username: "u"})
	require.Error(t, err)
	require.Contains(t, err.Error(), "srv", "error must name the module")
	require.Contains(t, err.Error(), "media", "error must name the share")
	require.Contains(t, err.Error(), "path", "error must name the missing field")
}

// Each mount entry carries the module id, the winning layer, and the
// module's scope for the later apply steps.
func TestResolveMounts_entriesCarryModuleLayerScope(t *testing.T) {
	root := t.TempDir()
	writeModule(t, root, "sysmnt", `
scope = "system"

[mounts.x]
source = "UUID=1"
destination = "/mnt/x"
type = "ext4"
`)
	plan, err := loadAndResolve(t, root, &facts.Facts{Hostname: "h", Username: "u"})
	require.NoError(t, err)
	require.Len(t, plan.Mounts.Entries, 1)

	e := plan.Mounts.Entries[0]
	require.Equal(t, "sysmnt", e.Module)
	require.Equal(t, "base", e.Layer)
	require.Equal(t, "system", e.Scope)
}

// Plan mount entries are sorted by (Module, Name) regardless of declaration
// order, so downstream output is deterministic.
func TestResolveMounts_deterministicOrder(t *testing.T) {
	root := t.TempDir()
	writeModule(t, root, "zeta", `
[mounts.b]
source = "s"
destination = "/d"
type = "t"
[mounts.a]
source = "s"
destination = "/d"
type = "t"
`)
	writeModule(t, root, "alpha", `
[mounts.z]
source = "s"
destination = "/d"
type = "t"
[mounts.a]
source = "s"
destination = "/d"
type = "t"
`)
	plan, err := loadAndResolve(t, root, &facts.Facts{Hostname: "h", Username: "u"})
	require.NoError(t, err)

	var got []string
	for _, e := range plan.Mounts.Entries {
		got = append(got, e.Module+"/"+e.Name)
	}
	require.Equal(t, []string{"alpha/a", "alpha/z", "zeta/a", "zeta/b"}, got)
}

// S3 regression guard: profiles without [mounts]/[smb] produce empty
// aggregates and no behavior change.
func TestResolve_noMountsNoSmb_emptyAggregates(t *testing.T) {
	p, err := profile.Load(fixture(t, "resolve"), &facts.Facts{Hostname: "myhost", Username: "cri"})
	require.NoError(t, err)

	plan, err := resolve.Resolve(p, &facts.Facts{Hostname: "myhost", Username: "cri"})
	require.NoError(t, err)
	require.Empty(t, plan.Mounts.Entries)
	require.Empty(t, plan.Smb.Modules)
}

// A module with no smb section contributes nothing to the smb aggregate.
func TestResolveSmb_moduleWithoutSmbContributesNothing(t *testing.T) {
	root := t.TempDir()
	writeModule(t, root, "plain", `[packages]
present = ["vim"]
`)
	plan, err := loadAndResolve(t, root, &facts.Facts{Hostname: "h", Username: "u"})
	require.NoError(t, err)
	require.Empty(t, plan.Smb.Modules)
}
