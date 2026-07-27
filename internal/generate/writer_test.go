package generate

import (
	"crypto/sha256"
	"fmt"
	"io/fs"
	"maps"
	"os"
	"path/filepath"
	"slices"
	"testing"

	"github.com/BurntSushi/toml"
	"github.com/stretchr/testify/require"
	"github.com/thedataflows/dotdrift/internal/profile"
)

// --- helpers ---------------------------------------------------------------

// nfsMount returns a valid nfs mount spec for the given destination.
func nfsMount(destination, startAt string) profile.MountSpec {
	return profile.MountSpec{
		Source:      "nas:/volume1/share",
		Destination: destination,
		Type:        "nfs",
		StartAt:     startAt,
	}
}

// smbInput returns a minimal smb spec with one share.
func smbInput() *profile.SmbSpec {
	return &profile.SmbSpec{
		Group: "media",
		Shares: map[string]profile.ShareSpec{
			"data": {Path: "/mnt/data", Writable: true},
		},
	}
}

// moduleDirFor returns the expected module directory for a selection.
func moduleDirFor(root string, sel Selection) string {
	switch sel.Layer {
	case LayerBase:
		return filepath.Join(root, "modules", sel.ModuleID)
	case LayerHost:
		return filepath.Join(root, "hosts", sel.Hostname, "modules", sel.ModuleID)
	case LayerUser:
		return filepath.Join(root, "users", sel.Username, "modules", sel.ModuleID)
	default:
		panic("bad layer in test")
	}
}

// treeManifest returns a relpath -> sha256 map of every file under dir.
func treeManifest(t *testing.T, dir string) map[string]string {
	t.Helper()
	out := map[string]string{}
	err := filepath.WalkDir(dir, func(path string, d fs.DirEntry, err error) error {
		require.NoError(t, err)
		if d.IsDir() {
			return nil
		}
		data, err := os.ReadFile(path)
		require.NoError(t, err)
		rel, err := filepath.Rel(dir, path)
		require.NoError(t, err)
		out[rel] = fmt.Sprintf("%x", sha256.Sum256(data))
		return nil
	})
	require.NoError(t, err)
	return out
}

// treeFiles returns the sorted relative file paths under dir.
func treeFiles(t *testing.T, dir string) []string {
	t.Helper()
	m := treeManifest(t, dir)
	return slices.Sorted(maps.Keys(m))
}

// decodeModule reads and decodes the module.toml under dir.
func decodeModule(t *testing.T, dir string) profile.ModuleConfig {
	t.Helper()
	var cfg profile.ModuleConfig
	_, err := toml.DecodeFile(filepath.Join(dir, "module.toml"), &cfg)
	require.NoError(t, err)
	return cfg
}

// writeModuleToml writes raw module.toml content into dir (pre-seeding).
func writeModuleToml(t *testing.T, dir, content string) {
	t.Helper()
	require.NoError(t, os.MkdirAll(dir, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "module.toml"), []byte(content), 0o644))
}

// --- layer placement -------------------------------------------------------

func TestWriter_createsModuleAtBaseLayer(t *testing.T) {
	noOverride(t)
	root := t.TempDir()
	sel := Selection{Layer: LayerBase, ModuleID: "media"}
	withTokens := nfsMount("/mnt/data", "Mon *-*-* 04:00:00")
	withTokens.Options = []string{"rw", "uid", "gid"}
	input := Input{
		Mounts: map[string]profile.MountSpec{"data": withTokens},
		Smb:    smbInput(),
		UID:    1000, GID: 1000,
	}
	require.NoError(t, WriteModule(root, sel, input))

	dir := moduleDirFor(root, sel)
	stem := EscapePath("/mnt/data")
	require.Equal(t, []string{
		stem + ".mount",
		stem + ".service",
		stem + ".timer",
		"module.toml",
		"shares.conf",
		"smb.conf",
	}, treeFiles(t, dir))

	// Rendered unit content matches the pure renderers.
	reg, err := Load()
	require.NoError(t, err)
	preset, ok := reg.Entry("nfs")
	require.True(t, ok)
	mountUnit, err := os.ReadFile(filepath.Join(dir, stem+".mount"))
	require.NoError(t, err)
	require.Equal(t, RenderMountUnit("data", input.Mounts["data"], preset, 1000, 1000), string(mountUnit))
	require.Contains(t, string(mountUnit), "uid=1000")

	// smb.conf seed carries the static include line.
	smbConf, err := os.ReadFile(filepath.Join(dir, "smb.conf"))
	require.NoError(t, err)
	require.Contains(t, string(smbConf), "[global]")
	require.Contains(t, string(smbConf), "include = /etc/samba/smb.conf.d/shares.conf")

	cfg := decodeModule(t, dir)
	require.Equal(t, profile.ScopeSystem, cfg.Scope)
	require.Equal(t, input.Mounts, cfg.Mounts)
	require.Equal(t, *input.Smb, cfg.Smb)
	require.Equal(t, []string{"avahi", "nfs-utils", "nfsidmap", "samba"}, cfg.Packages.Present)

	// Dotfile entries: units to the unit dir, shares/smb.conf to /etc/samba.
	for _, f := range []string{stem + ".mount", stem + ".service", stem + ".timer"} {
		df, ok := cfg.Dotfiles["/usr/local/lib/systemd/system/"+f]
		require.True(t, ok, "dotfile entry for %s", f)
		require.Equal(t, profile.Dotfile{Source: f, Mode: "copy"}, df)
	}
	require.Equal(t, profile.Dotfile{Source: "shares.conf", Mode: "copy"},
		cfg.Dotfiles["/etc/samba/smb.conf.d/shares.conf"])
	require.Equal(t, profile.Dotfile{Source: "smb.conf", Mode: "copy"},
		cfg.Dotfiles["/etc/samba/smb.conf"])
}

func TestWriter_createsModuleAtHostLayer(t *testing.T) {
	noOverride(t)
	root := t.TempDir()
	sel := Selection{Layer: LayerHost, Hostname: "linux2", ModuleID: "media"}
	input := Input{Mounts: map[string]profile.MountSpec{"data": nfsMount("/mnt/data", "")}, UID: 1, GID: 1}
	require.NoError(t, WriteModule(root, sel, input))

	dir := moduleDirFor(root, sel)
	require.Equal(t, filepath.Join(root, "hosts", "linux2", "modules", "media"), dir)
	stem := EscapePath("/mnt/data")
	require.Equal(t, []string{stem + ".mount", "module.toml"}, treeFiles(t, dir),
		"no startat: only the .mount unit is rendered")
	cfg := decodeModule(t, dir)
	require.Equal(t, profile.ScopeSystem, cfg.Scope)

	err := WriteModule(root, Selection{Layer: LayerHost, ModuleID: "media"}, input)
	require.Error(t, err, "host layer without a hostname must fail")
	require.Contains(t, err.Error(), "hostname")
}

func TestWriter_createsModuleAtUserLayer(t *testing.T) {
	noOverride(t)
	root := t.TempDir()
	sel := Selection{Layer: LayerUser, Username: "cri", ModuleID: "media"}
	input := Input{Mounts: map[string]profile.MountSpec{"data": nfsMount("/mnt/data", "")}, UID: 1, GID: 1}
	require.NoError(t, WriteModule(root, sel, input))

	dir := moduleDirFor(root, sel)
	require.Equal(t, filepath.Join(root, "users", "cri", "modules", "media"), dir)
	require.FileExists(t, filepath.Join(dir, "module.toml"))

	err := WriteModule(root, Selection{Layer: LayerUser, ModuleID: "media"}, input)
	require.Error(t, err, "user layer without a username must fail")
	require.Contains(t, err.Error(), "username")

	err = WriteModule(root, Selection{Layer: "bogus", ModuleID: "media"}, input)
	require.Error(t, err, "unknown layer must fail")

	err = WriteModule(root, Selection{Layer: LayerBase}, input)
	require.Error(t, err, "empty module id must fail")
}

// --- idempotency -----------------------------------------------------------

func TestWriter_rerunByteIdentical(t *testing.T) {
	noOverride(t)
	root := t.TempDir()
	sel := Selection{Layer: LayerBase, ModuleID: "media"}
	input := Input{
		Mounts: map[string]profile.MountSpec{
			"data":  nfsMount("/mnt/data", "daily"),
			"extra": nfsMount("/mnt/extra", ""),
		},
		Smb: smbInput(),
		UID: 1000, GID: 1000,
	}
	require.NoError(t, WriteModule(root, sel, input))
	first := treeManifest(t, moduleDirFor(root, sel))

	require.NoError(t, WriteModule(root, sel, input))
	second := treeManifest(t, moduleDirFor(root, sel))

	require.Equal(t, first, second, "same input must produce a byte-identical module tree")
}

// --- preservation ----------------------------------------------------------

func TestWriter_preservesUnrelatedSections(t *testing.T) {
	noOverride(t)
	root := t.TempDir()
	sel := Selection{Layer: LayerBase, ModuleID: "media"}
	dir := moduleDirFor(root, sel)
	writeModuleToml(t, dir, `
# a hand-written comment that will not survive re-encoding
id = "media"
app = "mediabox"
scope = "user"

[when]
hosts = ["linux2"]

[packages]
present = ["curl"]
absent = ["nano"]

[tools]
go = "1.26"

[hooks]
pre = ["echo hi"]
post = ["echo done"]

[dotfiles]
"~/.mediarc" = { source = "mediarc", mode = "link" }
`)
	input := Input{Mounts: map[string]profile.MountSpec{"data": nfsMount("/mnt/data", "")}, UID: 1, GID: 1}
	require.NoError(t, WriteModule(root, sel, input))

	cfg := decodeModule(t, dir)
	require.Equal(t, "media", cfg.ID)
	require.Equal(t, "mediabox", cfg.App)
	require.Equal(t, []string{"linux2"}, cfg.When.Hosts)
	require.Equal(t, []string{"nano"}, cfg.Packages.Absent)
	require.Contains(t, cfg.Packages.Present, "curl")
	require.Equal(t, map[string]string{"go": "1.26"}, cfg.Tools)
	require.Equal(t, profile.Hooks{Pre: []string{"echo hi"}, Post: []string{"echo done"}}, cfg.Hooks)
	require.Equal(t, profile.Dotfile{Source: "mediarc", Mode: "link"}, cfg.Dotfiles["~/.mediarc"],
		"unrelated dotfile target must survive")
}

func TestWriter_unionsPackages(t *testing.T) {
	noOverride(t)
	root := t.TempDir()
	sel := Selection{Layer: LayerBase, ModuleID: "media"}
	dir := moduleDirFor(root, sel)
	writeModuleToml(t, dir, `
[packages]
present = ["curl", "nfs-utils"]
`)
	input := Input{
		Mounts: map[string]profile.MountSpec{"data": nfsMount("/mnt/data", "")},
		Smb:    smbInput(),
		UID:    1, GID: 1,
	}
	require.NoError(t, WriteModule(root, sel, input))

	cfg := decodeModule(t, dir)
	require.Equal(t, []string{"avahi", "curl", "nfs-utils", "nfsidmap", "samba"}, cfg.Packages.Present,
		"union of existing, registry-required, and smb packages; sorted and deduped")
}

func TestWriter_scopeSystemSet(t *testing.T) {
	noOverride(t)
	root := t.TempDir()
	sel := Selection{Layer: LayerBase, ModuleID: "media"}
	input := Input{Mounts: map[string]profile.MountSpec{"data": nfsMount("/mnt/data", "")}, UID: 1, GID: 1}
	require.NoError(t, WriteModule(root, sel, input))
	require.Equal(t, profile.ScopeSystem, decodeModule(t, moduleDirFor(root, sel)).Scope)
}

// --- smb seeding -----------------------------------------------------------

func TestWriter_smbConfSeededOnceNotClobbered(t *testing.T) {
	noOverride(t)
	root := t.TempDir()
	sel := Selection{Layer: LayerBase, ModuleID: "media"}
	dir := moduleDirFor(root, sel)
	input := Input{
		Mounts: map[string]profile.MountSpec{"data": nfsMount("/mnt/data", "")},
		Smb:    smbInput(),
		UID:    1, GID: 1,
	}
	require.NoError(t, WriteModule(root, sel, input))

	// Hand-edit smb.conf; change the declared shares for the rerun.
	handEdited := "# my local tweaks\n[global]\nworkgroup = HOME\n"
	require.NoError(t, os.WriteFile(filepath.Join(dir, "smb.conf"), []byte(handEdited), 0o644))
	input.Smb.Shares["backup"] = profile.ShareSpec{Path: "/mnt/backup"}
	require.NoError(t, WriteModule(root, sel, input))

	smbConf, err := os.ReadFile(filepath.Join(dir, "smb.conf"))
	require.NoError(t, err)
	require.Equal(t, handEdited, string(smbConf), "smb.conf is user-owned after seeding: never clobbered")

	sharesConf, err := os.ReadFile(filepath.Join(dir, "shares.conf"))
	require.NoError(t, err)
	require.Equal(t, RenderSharesConf(*input.Smb), string(sharesConf),
		"shares.conf is always regenerated from the declared shares")
	require.Contains(t, string(sharesConf), "[backup]")
}

func TestWriter_smbNilLeavesExistingAlone(t *testing.T) {
	noOverride(t)
	root := t.TempDir()
	sel := Selection{Layer: LayerBase, ModuleID: "media"}
	dir := moduleDirFor(root, sel)

	// First run WITH smb: establishes smb section + shares.conf + smb.conf.
	withSmb := Input{
		Mounts: map[string]profile.MountSpec{"data": nfsMount("/mnt/data", "")},
		Smb:    smbInput(),
		UID:    1, GID: 1,
	}
	require.NoError(t, WriteModule(root, sel, withSmb))
	sharesBefore, err := os.ReadFile(filepath.Join(dir, "shares.conf"))
	require.NoError(t, err)
	smbConfBefore, err := os.ReadFile(filepath.Join(dir, "smb.conf"))
	require.NoError(t, err)
	smbSectionBefore := decodeModule(t, dir).Smb

	// Rerun with Smb nil: nil means "not managed by this run" — smb section,
	// shares.conf, and smb.conf stay exactly as they were.
	withoutSmb := Input{
		Mounts: map[string]profile.MountSpec{"other": nfsMount("/mnt/other", "")},
		Smb:    nil,
		UID:    1, GID: 1,
	}
	require.NoError(t, WriteModule(root, sel, withoutSmb))

	sharesAfter, err := os.ReadFile(filepath.Join(dir, "shares.conf"))
	require.NoError(t, err)
	require.Equal(t, string(sharesBefore), string(sharesAfter), "shares.conf untouched when Smb is nil")
	smbConfAfter, err := os.ReadFile(filepath.Join(dir, "smb.conf"))
	require.NoError(t, err)
	require.Equal(t, string(smbConfBefore), string(smbConfAfter), "smb.conf untouched when Smb is nil")

	cfg := decodeModule(t, dir)
	require.Equal(t, smbSectionBefore, cfg.Smb, "smb section untouched when Smb is nil")
	require.Contains(t, cfg.Dotfiles, "/etc/samba/smb.conf.d/shares.conf")
	require.Contains(t, cfg.Dotfiles, "/etc/samba/smb.conf")
	require.Contains(t, cfg.Packages.Present, "samba", "previously unioned smb packages persist")
}

// --- stale artifact GC -----------------------------------------------------

func TestWriter_staleArtifactsRemoved(t *testing.T) {
	noOverride(t)
	root := t.TempDir()
	sel := Selection{Layer: LayerBase, ModuleID: "media"}
	dir := moduleDirFor(root, sel)

	first := Input{
		Mounts: map[string]profile.MountSpec{
			"data":  nfsMount("/mnt/data", "daily"),
			"extra": nfsMount("/mnt/extra", ""),
		},
		Smb: smbInput(),
		UID: 1, GID: 1,
	}
	require.NoError(t, WriteModule(root, sel, first))
	dataStem := EscapePath("/mnt/data")
	extraStem := EscapePath("/mnt/extra")

	// User-owned files in the module dir: an arbitrary file, an untracked
	// unit-looking file, and smb.conf (hand-editable after seeding).
	require.NoError(t, os.WriteFile(filepath.Join(dir, "notes.txt"), []byte("keep me"), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "custom.mount"), []byte("# mine"), 0o644))

	// Rerun: data loses its startat, extra is gone entirely.
	second := Input{
		Mounts: map[string]profile.MountSpec{"data": nfsMount("/mnt/data", "")},
		Smb:    smbInput(),
		UID:    1, GID: 1,
	}
	require.NoError(t, WriteModule(root, sel, second))

	files := treeFiles(t, dir)
	require.Contains(t, files, dataStem+".mount")
	require.NotContains(t, files, dataStem+".service", "dropped startat: .service must be GC'd")
	require.NotContains(t, files, dataStem+".timer", "dropped startat: .timer must be GC'd")
	require.NotContains(t, files, extraStem+".mount", "removed mount: units must be GC'd")
	require.Contains(t, files, "notes.txt", "unrelated user file survives")
	require.Contains(t, files, "custom.mount", "untracked unit-looking user file survives")
	require.Contains(t, files, "smb.conf", "smb.conf is user-owned: never GC'd")
	require.Contains(t, files, "shares.conf")

	cfg := decodeModule(t, dir)
	require.Contains(t, cfg.Dotfiles, "/usr/local/lib/systemd/system/"+dataStem+".mount")
	require.NotContains(t, cfg.Dotfiles, "/usr/local/lib/systemd/system/"+dataStem+".service")
	require.NotContains(t, cfg.Dotfiles, "/usr/local/lib/systemd/system/"+dataStem+".timer")
	require.NotContains(t, cfg.Dotfiles, "/usr/local/lib/systemd/system/"+extraStem+".mount")

	// GC is itself idempotent: a third identical run changes nothing.
	manifest := treeManifest(t, dir)
	require.NoError(t, WriteModule(root, sel, second))
	require.Equal(t, manifest, treeManifest(t, dir))
}

// --- validation ------------------------------------------------------------

func TestWriter_unknownTypeErrors(t *testing.T) {
	noOverride(t)
	root := t.TempDir()
	sel := Selection{Layer: LayerBase, ModuleID: "media"}
	input := Input{
		Mounts: map[string]profile.MountSpec{
			"good": nfsMount("/mnt/good", ""),
			"bad":  {Source: "zpool/zfs", Destination: "/mnt/zfs", Type: "zfs"},
		},
		UID: 1, GID: 1,
	}
	err := WriteModule(root, sel, input)
	require.Error(t, err)
	require.Contains(t, err.Error(), "bad", "error must name the mount")
	require.Contains(t, err.Error(), "zfs", "error must name the unknown type")

	dir := moduleDirFor(root, sel)
	entries, readErr := os.ReadDir(dir)
	if readErr == nil {
		require.Empty(t, entries, "validation happens before any write: nothing half-written")
	} else {
		require.ErrorIs(t, readErr, os.ErrNotExist, "module dir must not even be created")
	}
}
