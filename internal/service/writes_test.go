package service

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/thedataflows/dotdrift/internal/facts"
	"github.com/thedataflows/dotdrift/internal/generate"
	"github.com/thedataflows/dotdrift/internal/mise"
	"github.com/thedataflows/dotdrift/internal/profile"
)

// The writes area owns the write orchestration the CLI adapters used to
// carry (0061-D6/D7's writes slice, T-tui-writes): onboard, restore, and
// generate run here once; the CLI translates flags and prints, the TUI
// speaks these types only.

// onboardFixture builds a writable profile root, one live file to adopt,
// and a writes area over faked seams (detect + mise runner).
func onboardFixture(t *testing.T) (root, live string, area *WritesArea, fake *mise.FakeRunner) {
	t.Helper()
	t.Setenv("XDG_STATE_HOME", t.TempDir())
	root = t.TempDir()
	live = filepath.Join(t.TempDir(), "live.conf")
	require.NoError(t, os.WriteFile(live, []byte("x=1\n"), 0o644))
	fake = &mise.FakeRunner{}
	area = NewWritesArea(WritesDeps{
		Detect: func() (*facts.Facts, error) {
			return &facts.Facts{Hostname: "testhost", Username: "cri"}, nil
		},
		NewMise: func(bool) mise.Runner { return fake },
	})
	return root, live, area, fake
}

// The overlay owner policy: explicit value wins, a bare flag falls back
// to the detected fact, an unset flag stays empty (base-layer path).
func TestWrites_onboard_overlayOwner(t *testing.T) {
	require.Equal(t, "detected-host", overlayOwner(true, "", "detected-host"))
	require.Equal(t, "other-host", overlayOwner(true, "other-host", "detected-host"))
	require.Equal(t, "", overlayOwner(false, "", "detected-host"))
}

// Onboard translates raw flag values (packages, overlay owners) and runs
// the onboard flow: the module lands at the detected host overlay, mise
// runs with --yes, and the live file is adopted into the module tree.
func TestWrites_onboard_translatesAndRuns(t *testing.T) {
	root, live, area, fake := onboardFixture(t)
	var out strings.Builder

	err := area.Onboard(OnboardOpts{
		ProfileRoot: root,
		Paths:       []string{live},
		Module:      "myapp",
		Mode:        "copy",
		Packages:    []string{"ripgrep"},
		Tools:       []string{"node=20"},
		HostSet:     true, // bare flag: the detected hostname fills the overlay
		Yes:         true,
		Out:         &out,
	})
	require.NoError(t, err)

	require.True(t, fake.InstallCalled, "mise install not called")
	require.True(t, fake.DotfilesCalled, "mise dotfiles apply not called")
	require.True(t, fake.Yes, "--yes must flow to mise dotfiles apply")

	cfg, err := os.ReadFile(filepath.Join(root, "hosts", "testhost", "modules", "myapp", "module.toml"))
	require.NoError(t, err)
	require.Contains(t, string(cfg), "ripgrep")
	require.Contains(t, string(cfg), `node = "20"`)
	require.Contains(t, string(cfg), `mode = "copy"`)
	copied := filepath.Join(root, "hosts", "testhost", "modules", "myapp",
		"system", strings.TrimPrefix(live, string(filepath.Separator)))
	require.FileExists(t, copied, "the live path is adopted into the module tree")
}

// A malformed package flag fails at the boundary, before any module is
// touched, with the CLI's exact wrap.
func TestWrites_onboard_packageParseError(t *testing.T) {
	root, live, area, _ := onboardFixture(t)

	err := area.Onboard(OnboardOpts{ProfileRoot: root, Paths: []string{live}, Module: "myapp", Packages: []string{"=no name"}})
	require.ErrorContains(t, err, "parse packages:")
	_, statErr := os.Stat(filepath.Join(root, "modules", "myapp"))
	require.True(t, os.IsNotExist(statErr), "no module is created on a parse error")
}

// DryRun is an option field: the area reports what would happen and
// writes nothing.
func TestWrites_onboard_dryRun(t *testing.T) {
	root, live, area, fake := onboardFixture(t)
	var out strings.Builder

	err := area.Onboard(OnboardOpts{
		ProfileRoot: root, Paths: []string{live}, Module: "myapp",
		DryRun: true, Out: &out,
	})
	require.NoError(t, err)
	require.False(t, fake.InstallCalled && fake.DotfilesCalled, "dry-run never touches mise")
	_, statErr := os.Stat(filepath.Join(root, "modules", "myapp"))
	require.True(t, os.IsNotExist(statErr), "dry-run creates no module")
}

// restoreFixture builds a minimal profile (two modules) and an isolated
// HOME; tests seed backups at the mirrored absolute paths.
func restoreFixture(t *testing.T) (root, home string, area *WritesArea) {
	t.Helper()
	root = t.TempDir()
	home = t.TempDir()
	t.Setenv("HOME", home)
	for _, m := range []string{"app", "other"} {
		require.NoError(t, os.MkdirAll(filepath.Join(root, "modules", m), 0o755))
		require.NoError(t, os.WriteFile(filepath.Join(root, "modules", m, "module.toml"), []byte(""), 0o644))
	}
	area = NewWritesArea(WritesDeps{
		Detect: func() (*facts.Facts, error) {
			return &facts.Facts{Hostname: "h", Username: "cri"}, nil
		},
		LoadProfile: func(root string, f *facts.Facts) (*profile.Profile, error) {
			return profile.Load(root, f)
		},
	})
	return root, home, area
}

// seedBackup writes one backed-up file for target into moduleDir's
// generation, at the mirrored layout (full absolute target path).
func seedBackup(t *testing.T, moduleDir, gen, target, content string, mode os.FileMode) {
	t.Helper()
	bk := filepath.Join(moduleDir, "backups", gen, strings.TrimPrefix(target, "/"))
	require.NoError(t, os.MkdirAll(filepath.Dir(bk), 0o755))
	require.NoError(t, os.WriteFile(bk, []byte(content), mode))
	require.NoError(t, os.Chmod(bk, mode))
}

// A writable target restores from the newest generation holding it, with
// the backup's mode; the report names target and backup location, and the
// drift note follows any restore.
func TestWrites_restore_restoresNewestOfWritableTarget(t *testing.T) {
	root, home, area := restoreFixture(t)
	target := filepath.Join(home, ".config", "app", "config.toml")
	require.NoError(t, os.MkdirAll(filepath.Dir(target), 0o755))
	require.NoError(t, os.WriteFile(target, []byte("overwritten"), 0o644))
	seedBackup(t, filepath.Join(root, "modules", "app"), "20260823-101500", target, "older backup", 0o644)
	seedBackup(t, filepath.Join(root, "modules", "app"), "20260824-153000", target, "newest backup", 0o600)

	var out strings.Builder
	handovers := 0
	err := area.Restore(RestoreOpts{
		ProfilePath: root,
		Targets:     []string{"~/.config/app/config.toml"},
		Out:         &out,
		Handover:    func(*exec.Cmd) error { handovers++; return nil },
	})
	require.NoError(t, err)
	require.Zero(t, handovers, "a writable target needs no elevation")

	got, rerr := os.ReadFile(target)
	require.NoError(t, rerr)
	require.Equal(t, "newest backup", string(got))
	info, _ := os.Stat(target)
	require.Equal(t, os.FileMode(0o600), info.Mode().Perm(), "the backup's mode is restored")
	require.Contains(t, out.String(), "restored: "+target+" (modules/app/backups/20260824-153000)")
	require.Contains(t, out.String(), "apply overwrites them again")
}

// --gen pins an explicit generation; an unknown generation names the
// generations that exist.
func TestWrites_restore_generationPinning(t *testing.T) {
	root, home, area := restoreFixture(t)
	target := filepath.Join(home, ".config", "app", "config.toml")
	require.NoError(t, os.MkdirAll(filepath.Dir(target), 0o755))
	require.NoError(t, os.WriteFile(target, []byte("overwritten"), 0o644))
	seedBackup(t, filepath.Join(root, "modules", "app"), "20260823-101500", target, "older", 0o644)
	seedBackup(t, filepath.Join(root, "modules", "app"), "20260824-153000", target, "newer", 0o644)

	var out strings.Builder
	err := area.Restore(RestoreOpts{
		ProfilePath: root, Targets: []string{target}, Gen: "20260823-101500", Out: &out,
	})
	require.NoError(t, err)
	got, _ := os.ReadFile(target)
	require.Equal(t, "older", string(got))

	err = area.Restore(RestoreOpts{ProfilePath: root, Targets: []string{target}, Gen: "20250101-000000"})
	require.ErrorContains(t, err, "generation 20250101-000000 holds no backup of "+target)
	require.ErrorContains(t, err, "have: 20260823-101500, 20260824-153000")
}

// A target two modules back up is refused; a target nothing backs up is
// refused with the browse hint.
func TestWrites_restore_ambiguousAndMissing(t *testing.T) {
	root, home, area := restoreFixture(t)
	target := filepath.Join(home, ".config", "app", "config.toml")
	require.NoError(t, os.MkdirAll(filepath.Dir(target), 0o755))
	require.NoError(t, os.WriteFile(target, []byte("live"), 0o644))
	seedBackup(t, filepath.Join(root, "modules", "app"), "g1", target, "a", 0o644)
	seedBackup(t, filepath.Join(root, "modules", "other"), "g1", target, "b", 0o644)

	err := area.Restore(RestoreOpts{ProfilePath: root, Targets: []string{target}})
	require.ErrorContains(t, err, "is backed up by multiple modules")
	require.ErrorContains(t, err, filepath.Join(root, "modules", "app"))
	require.ErrorContains(t, err, filepath.Join(root, "modules", "other"))
	require.ErrorContains(t, err, "remove the stale backup")

	err = area.Restore(RestoreOpts{ProfilePath: root, Targets: []string{filepath.Join(home, ".zshrc")}})
	require.ErrorContains(t, err, "no backup of")
	require.ErrorContains(t, err, "try --list")

	err = area.Restore(RestoreOpts{ProfilePath: root, Targets: []string{"relative/path"}})
	require.ErrorContains(t, err, `must be absolute or start with ~`)

	err = area.Restore(RestoreOpts{ProfilePath: root})
	require.ErrorContains(t, err, "specify at least one target path")
}

// A target the user cannot write goes through the Handover seam with the
// sudo install policy; a symlink at the target is removed through the
// same handover first.
func TestWrites_restore_elevatedThroughHandover(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("root writes everything; nothing is elevated")
	}
	root, home, area := restoreFixture(t)
	sysRoot := filepath.Join(home, "etc-root")
	target := filepath.Join(sysRoot, "app", "demo.conf")
	require.NoError(t, os.MkdirAll(filepath.Dir(target), 0o755))
	for dir := filepath.Dir(target); ; dir = filepath.Dir(dir) {
		require.NoError(t, os.Chmod(dir, 0o555))
		if dir == sysRoot {
			break
		}
	}
	t.Cleanup(func() {
		for dir := filepath.Dir(target); ; dir = filepath.Dir(dir) {
			_ = os.Chmod(dir, 0o755)
			if dir == sysRoot {
				break
			}
		}
	})
	seedBackup(t, filepath.Join(root, "modules", "app"), "g1", target, "system copy", 0o640)

	var joined []string
	err := area.Restore(RestoreOpts{
		ProfilePath: root,
		Targets:     []string{target},
		Handover: func(c *exec.Cmd) error {
			joined = append(joined, c.Path+" "+strings.Join(c.Args[1:], " "))
			return nil
		},
	})
	require.NoError(t, err)
	require.Len(t, joined, 1, "one elevated copy")
	require.Contains(t, joined[0], "install")
	require.Contains(t, joined[0], "-m 0640", "the backup's mode rides the install")
	require.Contains(t, joined[0], target)
}

// DryRun lists what would be restored and touches nothing.
func TestWrites_restore_dryRun(t *testing.T) {
	root, home, area := restoreFixture(t)
	target := filepath.Join(home, ".config", "app", "config.toml")
	require.NoError(t, os.MkdirAll(filepath.Dir(target), 0o755))
	require.NoError(t, os.WriteFile(target, []byte("live"), 0o644))
	seedBackup(t, filepath.Join(root, "modules", "app"), "g1", target, "backup", 0o644)

	var out strings.Builder
	err := area.Restore(RestoreOpts{ProfilePath: root, Targets: []string{target}, DryRun: true, Out: &out})
	require.NoError(t, err)
	require.Contains(t, out.String(), "would restore: "+target+" (modules/app/backups/g1)")
	got, _ := os.ReadFile(target)
	require.Equal(t, "live", string(got), "dry-run writes nothing")
}

// RestorePlan resolves targets without touching anything: newest
// generation, profile-relative label, and the elevated flag a front end
// needs to warn before running.
func TestWrites_restorePlan_reportsWithoutWriting(t *testing.T) {
	root, home, area := restoreFixture(t)
	writable := filepath.Join(home, ".config", "app", "config.toml")
	require.NoError(t, os.MkdirAll(filepath.Dir(writable), 0o755))
	require.NoError(t, os.WriteFile(writable, []byte("live"), 0o644))
	seedBackup(t, filepath.Join(root, "modules", "app"), "g1", writable, "backup", 0o644)

	plan, err := area.RestorePlan(RestoreOpts{ProfilePath: root, Targets: []string{writable}})
	require.NoError(t, err)
	require.Equal(t, []RestorePlanItem{{
		Target: writable, Label: "modules/app/backups/g1", Gen: "g1",
	}}, plan)
	got, _ := os.ReadFile(writable)
	require.Equal(t, "live", string(got), "the plan writes nothing")
}

// GenerateSelection fills hostname/username from detected facts only when
// the layer needs them and the selection lacks them.
func TestWrites_generate_selectionFillsFromFacts(t *testing.T) {
	var detected int
	area := NewWritesArea(WritesDeps{
		Detect: func() (*facts.Facts, error) {
			detected++
			return &facts.Facts{Hostname: "box", Username: "cri"}, nil
		},
	})

	sel, err := area.GenerateSelection(generate.Selection{Layer: generate.LayerHost, ModuleID: "mounts"})
	require.NoError(t, err)
	require.Equal(t, "box", sel.Hostname, "host layer fills the hostname")
	require.Equal(t, 1, detected)

	sel, err = area.GenerateSelection(generate.Selection{
		Layer: generate.LayerUser, ModuleID: "smb", Username: "given",
	})
	require.NoError(t, err)
	require.Equal(t, "given", sel.Username, "an explicit username wins; no detect call")
	require.Equal(t, 1, detected, "no detect when nothing needs filling")
}

// WriteGenerate assembles nothing of its own: the caller builds the
// generate input through the shared builders; the area writes the module
// and prints the summary the CLI shows.
func TestWrites_generate_writeAndSummary(t *testing.T) {
	root := t.TempDir()
	area := NewWritesArea(WritesDeps{})
	sel := generate.Selection{Layer: generate.LayerBase, ModuleID: "mounts"}
	uid, gid, _, err := generate.InvokingUser()
	require.NoError(t, err)
	input := generate.MountsInput(map[string]profile.MountSpec{
		"data": {Source: "UUID=x", Destination: "/mnt/data", Type: "vfat"},
	}, uid, gid)

	var out strings.Builder
	require.NoError(t, area.WriteGenerate(root, sel, input, &out))

	_, statErr := os.Stat(filepath.Join(root, "modules", "mounts", "module.toml"))
	require.NoError(t, statErr, "the generated module exists")
	require.Contains(t, out.String(), "mounts", "the summary names the module")
}

// Volumes lists the volumes the generate flow classifies, annotated
// against the sources already managed by the target module.
func TestWrites_generate_volumes(t *testing.T) {
	var seenSources []string
	area := NewWritesArea(WritesDeps{
		Detect: func() (*facts.Facts, error) {
			return &facts.Facts{Hostname: "box", Username: "cri"}, nil
		},
		ListVolumes: func(_ context.Context, _ *generate.Registry, sources []string) ([]generate.Volume, error) {
			seenSources = sources
			return []generate.Volume{{UUID: "u1", FSType: "ext4"}}, nil
		},
	})
	sel, err := area.GenerateSelection(generate.Selection{Layer: generate.LayerHost, ModuleID: "mounts"})
	require.NoError(t, err)

	vols, err := area.Volumes(t.TempDir(), sel)
	require.NoError(t, err)
	require.Len(t, vols, 1)
	require.Equal(t, "u1", vols[0].UUID)
	require.Empty(t, seenSources, "an absent module manages nothing")
}

// ExistingMountSources feeds the managed-set annotation inside
// generate.Volumes (its own tested contract); the area's job is the
// passthrough, asserted above.

// The restore index maps absolute targets to per-module newest-first
// generations — the browse view both front ends show.
func TestWrites_restoreIndex(t *testing.T) {
	root, home, _ := restoreFixture(t)
	target := filepath.Join(home, ".config", "app", "config.toml")
	seedBackup(t, filepath.Join(root, "modules", "app"), "g1", target, "older", 0o644)
	seedBackup(t, filepath.Join(root, "modules", "app"), "g2", target, "newer", 0o644)

	p, err := profile.Load(root, &facts.Facts{Hostname: "h", Username: "cri"})
	require.NoError(t, err)
	hits := IndexBackups(ModuleLayers(p))

	byModule := hits[target]
	require.Len(t, byModule, 1, "one module holds the target")
	list := byModule[filepath.Join(root, "modules", "app")]
	require.Equal(t, []string{"g2", "g1"}, []string{list[0].Gen, list[1].Gen}, "newest first")
}

// A front end renders browse labels through the same rel helper the
// restore reports use (out-of-profile dirs fall back to a best-effort
// relative path, exactly as the CLI always rendered).
func TestWrites_moduleRel(t *testing.T) {
	root := t.TempDir()
	dir := filepath.Join(root, "hosts", "h", "modules", "m")
	require.Equal(t, filepath.Join("hosts", "h", "modules", "m"), ModuleRel(root, dir))
	require.Equal(t, filepath.Join("..", "elsewhere"), ModuleRel(root, filepath.Join(root, "..", "elsewhere")))
}
