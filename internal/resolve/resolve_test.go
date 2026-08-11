package resolve_test

import (
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/thedataflows/dotdrift/internal/facts"
	"github.com/thedataflows/dotdrift/internal/profile"
	"github.com/thedataflows/dotdrift/internal/resolve"
)

func fixture(t *testing.T, name string) string {
	t.Helper()
	return filepath.Join("..", "..", "testdata", "profiles", name)
}

func selectedIDs(p *profile.Profile) []string {
	ids := make([]string, len(p.Selected))
	for i, m := range p.Selected {
		ids[i] = m.ID
	}
	return ids
}

func TestMergePackages_userAbsentBeatsHostPresent(t *testing.T) {
	p, err := profile.Load(fixture(t, "resolve"), &facts.Facts{
		Hostname: "myhost",
		Username: "cri",
		OS:       "linux",
	})
	require.NoError(t, err)
	require.Contains(t, selectedIDs(p), "shell")

	plan, err := resolve.Resolve(p, &facts.Facts{Hostname: "myhost", Username: "cri"})
	require.NoError(t, err)

	require.NotContains(t, plan.Packages.Install, "nano", "user absent should remove nano")
	require.Contains(t, plan.Packages.Install, "neovim", "user present should restore neovim")
	require.Contains(t, plan.Packages.Install, "ripgrep", "base present should survive")
	require.Contains(t, plan.Packages.Install, "fd", "host present should survive")
	require.Contains(t, plan.Packages.Install, "eza", "user present should be added")
	require.NotContains(t, plan.Packages.Install, "emacs", "base absent should stay removed")
}

func TestMergePackages_presentIdempotent(t *testing.T) {
	p, err := profile.Load(fixture(t, "resolve"), &facts.Facts{
		Hostname: "myhost",
		Username: "cri",
		OS:       "linux",
	})
	require.NoError(t, err)

	plan, err := resolve.Resolve(p, &facts.Facts{Hostname: "myhost", Username: "cri"})
	require.NoError(t, err)

	sort.Strings(plan.Packages.Install)
	for i := 1; i < len(plan.Packages.Install); i++ {
		require.NotEqual(t, plan.Packages.Install[i-1], plan.Packages.Install[i], "duplicate package %q", plan.Packages.Install[i])
	}
}

func TestMergeDotfiles_userWinsSameTarget(t *testing.T) {
	p, err := profile.Load(fixture(t, "resolve"), &facts.Facts{
		Hostname: "myhost",
		Username: "cri",
		OS:       "linux",
	})
	require.NoError(t, err)

	plan, err := resolve.Resolve(p, &facts.Facts{Hostname: "myhost", Username: "cri"})
	require.NoError(t, err)

	var bashrc *resolve.DotfileEntry
	for i := range plan.Dotfiles.Entries {
		if plan.Dotfiles.Entries[i].Target == "~/.bashrc" {
			bashrc = &plan.Dotfiles.Entries[i]
			break
		}
	}
	require.NotNil(t, bashrc, "~/.bashrc entry should exist")
	require.Equal(t, "copy", bashrc.Mode, "user mode should win")
	require.Equal(t, "user", bashrc.Layer, "user layer should win")
}

func TestMergeTools_userWins(t *testing.T) {
	p, err := profile.Load(fixture(t, "resolve"), &facts.Facts{
		Hostname: "myhost",
		Username: "cri",
		OS:       "linux",
	})
	require.NoError(t, err)

	plan, err := resolve.Resolve(p, &facts.Facts{Hostname: "myhost", Username: "cri"})
	require.NoError(t, err)

	require.Equal(t, "22", plan.Tools.Versions["node"], "host override should win for node")
	require.Equal(t, "3.12", plan.Tools.Versions["python"], "user override should win for python")
}

func TestFileOverlay_userReplacesHost(t *testing.T) {
	p, err := profile.Load(fixture(t, "resolve"), &facts.Facts{
		Hostname: "myhost",
		Username: "cri",
		OS:       "linux",
	})
	require.NoError(t, err)

	plan, err := resolve.Resolve(p, &facts.Facts{Hostname: "myhost", Username: "cri"})
	require.NoError(t, err)

	var bashrc *resolve.DotfileEntry
	for i := range plan.Dotfiles.Entries {
		if plan.Dotfiles.Entries[i].Target == "~/.bashrc" {
			bashrc = &plan.Dotfiles.Entries[i]
			break
		}
	}
	require.NotNil(t, bashrc, "~/.bashrc entry should exist")
	require.True(t, strings.Contains(bashrc.Source, "users/cri/modules/shell"), "source file should be resolved from user layer: %s", bashrc.Source)
}

func TestSelectionFingerprint_stable(t *testing.T) {
	f1 := &facts.Facts{Hostname: "myhost"}
	p1, err := profile.Load(fixture(t, "whenfilter"), f1)
	require.NoError(t, err)

	fp1 := resolve.Fingerprint(p1, f1)
	fp2 := resolve.Fingerprint(p1, f1)
	require.Equal(t, fp1, fp2, "fingerprint should be stable for the same inputs")

	f2 := &facts.Facts{Username: "cri"}
	p2, err := profile.Load(fixture(t, "whenfilter"), f2)
	require.NoError(t, err)
	fp3 := resolve.Fingerprint(p2, f2)
	require.NotEqual(t, fp1, fp3, "different selection should produce different fingerprint")

	f3 := &facts.Facts{Hostname: "myhost", Kernel: "7.2.0"}
	fp4 := resolve.Fingerprint(p1, f3)
	require.NotEqual(t, fp1, fp4, "kernel change should produce different fingerprint")
}

func TestResolve_emptyProfile(t *testing.T) {
	plan, err := resolve.Resolve(&profile.Profile{}, &facts.Facts{})
	require.NoError(t, err)
	require.Empty(t, plan.Packages.Install)
	require.Empty(t, plan.Tools.Versions)
	require.Empty(t, plan.Dotfiles.Entries)
}

func TestMergePackages_absentInRemoveList(t *testing.T) {
	p, err := profile.Load(fixture(t, "resolve"), &facts.Facts{
		Hostname: "myhost",
		Username: "cri",
		OS:       "linux",
	})
	require.NoError(t, err)
	require.Contains(t, selectedIDs(p), "shell")

	plan, err := resolve.Resolve(p, &facts.Facts{Hostname: "myhost", Username: "cri"})
	require.NoError(t, err)

	require.Contains(t, plan.Packages.Remove, "nano", "user absent should appear in remove list")
	require.Contains(t, plan.Packages.Remove, "emacs", "base absent should appear in remove list")
}

// Precedence is symmetric: a higher layer's present overrides a lower
// layer's absent, just as a higher absent overrides a lower present.
func TestMergePackages_baseAbsentBeatenByHostPresent(t *testing.T) {
	root := t.TempDir()
	writeModule(t, root, "shell", "[packages]\nabsent = [\"nano\"]\npresent = [\"ripgrep\"]\n")
	hostDir := filepath.Join(root, "hosts", "myhost", "modules", "shell")
	require.NoError(t, os.MkdirAll(hostDir, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(hostDir, "module.toml"), []byte("[packages]\npresent = [\"nano\"]\n"), 0o644))

	plan, err := loadAndResolve(t, root, &facts.Facts{Hostname: "myhost", Username: "cri"})
	require.NoError(t, err)
	require.Contains(t, plan.Packages.Install, "nano", "host present should override base absent")
	require.NotContains(t, plan.Packages.Remove, "nano")
	require.Contains(t, plan.Packages.Install, "ripgrep", "base present should survive")
}

func writeModule(t *testing.T, root, id, moduleTOML string) string {
	t.Helper()
	dir := filepath.Join(root, "modules", id)
	require.NoError(t, os.MkdirAll(dir, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "module.toml"), []byte(moduleTOML), 0o644))
	return dir
}

func loadAndResolve(t *testing.T, root string, f *facts.Facts) (*resolve.Plan, error) {
	t.Helper()
	p, err := profile.Load(root, f)
	require.NoError(t, err)
	return resolve.Resolve(p, f)
}

func TestResolveSource_traversalRejected(t *testing.T) {
	root := t.TempDir()
	writeModule(t, root, "evil", `
[dotfiles]
"~/.ssh/authorized_keys" = { source = "../../outside", mode = "copy" }
`)
	f := &facts.Facts{Hostname: "h", Username: "u"}

	_, err := loadAndResolve(t, root, f)
	require.Error(t, err, "source escaping the layer root must be rejected")
	require.Contains(t, err.Error(), "evil", "error should name the module")
	require.Contains(t, err.Error(), "../../outside", "error should name the offending source")
}

func TestResolveSource_missingFileErrors(t *testing.T) {
	root := t.TempDir()
	writeModule(t, root, "mod", `
[dotfiles]
"~/.x" = { source = "no-such-file", mode = "copy" }
`)
	f := &facts.Facts{Hostname: "h", Username: "u"}

	_, err := loadAndResolve(t, root, f)
	require.Error(t, err, "declared source file that does not exist must be an error")
	require.Contains(t, err.Error(), "mod", "error should name the module")
	require.Contains(t, err.Error(), "no-such-file", "error should name the missing source")
}

func TestResolveDotfileMode_unknownModeErrors(t *testing.T) {
	root := t.TempDir()
	dir := writeModule(t, root, "mod", `
[dotfiles]
"~/.x" = { source = "x", mode = "hardlink" }
`)
	require.NoError(t, os.WriteFile(filepath.Join(dir, "x"), []byte("data"), 0o644))
	f := &facts.Facts{Hostname: "h", Username: "u"}

	_, err := loadAndResolve(t, root, f)
	require.Error(t, err, "unknown dotfile mode must be rejected at resolve time")
	require.Contains(t, err.Error(), "mod", "error should name the module")
	require.Contains(t, err.Error(), "hardlink", "error should name the offending mode")
}

// mise ignores entries with an empty mode ("unknown mode ”, ignoring
// entry", exit 0), so an omitted mode is the same silent-breakage class as
// an unknown one and must fail loudly at resolve time.
func TestResolveDotfileMode_emptyModeErrors(t *testing.T) {
	root := t.TempDir()
	dir := writeModule(t, root, "mod", `
[dotfiles]
"~/.x" = { source = "x" }
`)
	require.NoError(t, os.WriteFile(filepath.Join(dir, "x"), []byte("data"), 0o644))
	f := &facts.Facts{Hostname: "h", Username: "u"}

	_, err := loadAndResolve(t, root, f)
	require.Error(t, err, "omitted dotfile mode must be rejected at resolve time")
	require.Contains(t, err.Error(), "mod", "error should name the module")
}

// Every mode documented in docs/product/profile-layout.md must resolve.
func TestResolveDotfileMode_documentedModesPass(t *testing.T) {
	for _, mode := range []string{"symlink", "symlink-each", "copy", "template"} {
		t.Run(mode, func(t *testing.T) {
			root := t.TempDir()
			dir := writeModule(t, root, "mod", `
[dotfiles]
"~/.x" = { source = "x", mode = "`+mode+`" }
`)
			require.NoError(t, os.WriteFile(filepath.Join(dir, "x"), []byte("data"), 0o644))
			f := &facts.Facts{Hostname: "h", Username: "u"}

			plan, err := loadAndResolve(t, root, f)
			require.NoError(t, err)
			require.Len(t, plan.Dotfiles.Entries, 1)
			require.Equal(t, mode, plan.Dotfiles.Entries[0].Mode,
				"the plan keeps dotdrift vocabulary; translation to mise happens at generation")
		})
	}
}

func TestResolve_overlayTOMLErrorPropagated(t *testing.T) {
	for _, layer := range []string{"hosts", "users"} {
		t.Run(layer, func(t *testing.T) {
			root := t.TempDir()
			writeModule(t, root, "shell", "[packages]\npresent = [\"ripgrep\"]\n")

			var name string
			if layer == "hosts" {
				name = "myhost"
			} else {
				name = "cri"
			}
			overlayDir := filepath.Join(root, layer, name, "modules", "shell")
			require.NoError(t, os.MkdirAll(overlayDir, 0o755))
			require.NoError(t, os.WriteFile(filepath.Join(overlayDir, "module.toml"), []byte("not = [valid"), 0o644))

			f := &facts.Facts{Hostname: "myhost", Username: "cri"}
			_, err := loadAndResolve(t, root, f)
			require.Error(t, err, "malformed %s overlay module.toml must propagate", layer)
			require.Contains(t, err.Error(), "shell", "error should name the module")
			require.Contains(t, err.Error(), filepath.Join(layer, name), "error should identify the overlay path")
		})
	}
}

func TestResolve_crossModulePackageConflict(t *testing.T) {
	root := t.TempDir()
	writeModule(t, root, "moda", "[packages]\npresent = [\"vim\"]\n")
	writeModule(t, root, "modb", "[packages]\nabsent = [\"vim\"]\n")
	f := &facts.Facts{Hostname: "h", Username: "u"}

	_, err := loadAndResolve(t, root, f)
	require.Error(t, err, "present in one module and absent in another is a conflict")
	require.Contains(t, err.Error(), "vim", "error should name the conflicting package")
	require.Contains(t, err.Error(), "moda", "error should name the present module")
	require.Contains(t, err.Error(), "modb", "error should name the absent module")
}

// Two modules claiming the same dotfile target would emit a duplicate-key
// mise.toml that mise rejects with a parse error (dogfooded against a real
// profile: mediamtx + systemd both targeted ~/.config/systemd). Resolve must
// fail loudly naming the target and every module claiming it, symmetric with
// cross-module package conflicts (contract invariant #16).
func TestResolve_crossModuleDotfileTargetConflict(t *testing.T) {
	root := t.TempDir()
	dirA := writeModule(t, root, "moda", `
[dotfiles]
"~/.config/systemd" = { source = "systemd", mode = "copy" }
`)
	require.NoError(t, os.WriteFile(filepath.Join(dirA, "systemd"), []byte("a"), 0o644))
	dirB := writeModule(t, root, "modb", `
[dotfiles]
"~/.config/systemd" = { source = "systemd", mode = "symlink-each" }
`)
	require.NoError(t, os.MkdirAll(filepath.Join(dirB, "systemd"), 0o755))
	f := &facts.Facts{Hostname: "h", Username: "u"}

	_, err := loadAndResolve(t, root, f)
	require.Error(t, err, "two modules writing the same dotfile target must be a resolve-time error")
	require.Contains(t, err.Error(), "~/.config/systemd", "error should name the conflicting target")
	require.Contains(t, err.Error(), "moda", "error should name every claiming module")
	require.Contains(t, err.Error(), "modb", "error should name every claiming module")
}

func TestResolve_emptyHostnameErrors(t *testing.T) {
	root := t.TempDir()
	writeModule(t, root, "mod", "")
	f := &facts.Facts{Username: "u"}

	_, err := loadAndResolve(t, root, f)
	require.Error(t, err, "empty hostname with selected modules must be an explicit error")
	require.Contains(t, err.Error(), "hostname")
}

func TestResolve_emptyUsernameErrors(t *testing.T) {
	root := t.TempDir()
	writeModule(t, root, "mod", "")
	f := &facts.Facts{Hostname: "h"}

	_, err := loadAndResolve(t, root, f)
	require.Error(t, err, "empty username with selected modules must be an explicit error")
	require.Contains(t, err.Error(), "username")
}

// A module's structured optional hooks resolve into plan.Hooks with the
// Optional flag intact — resolve must carry it, not collapse to bare strings.
func TestResolve_hookOptionalFlowsThrough(t *testing.T) {
	root := t.TempDir()
	writeModule(t, root, "shell", `
[hooks]
[[hooks.pre]]
command = "echo required"
[[hooks.pre]]
command = "echo flaky"
optional = true
`)
	plan, err := loadAndResolve(t, root, &facts.Facts{Hostname: "myhost", Username: "cri"})
	require.NoError(t, err)
	require.Equal(t, []profile.HookCommand{
		{Command: "echo required"},
		{Command: "echo flaky", Optional: true},
	}, plan.Hooks.Pre)
}

// Hooks merge across layers base → user in order, each keeping its own
// Optional flag (an optional base hook stays optional after a user overlay).
func TestResolve_hookOptionalMergesAcrossLayers(t *testing.T) {
	root := t.TempDir()
	writeModule(t, root, "shell", `
[[hooks.pre]]
command = "echo base-opt"
optional = true
`)
	userDir := filepath.Join(root, "users", "cri", "modules", "shell")
	require.NoError(t, os.MkdirAll(userDir, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(userDir, "module.toml"), []byte(`
[[hooks.pre]]
command = "echo user-req"
`), 0o644))

	plan, err := loadAndResolve(t, root, &facts.Facts{Hostname: "myhost", Username: "cri"})
	require.NoError(t, err)
	require.Equal(t, []profile.HookCommand{
		{Command: "echo base-opt", Optional: true},
		{Command: "echo user-req"},
	}, plan.Hooks.Pre)
}

// --- edit entries (line/block/template partial edits) ---

// An inline line edit resolves with no source file on disk — resolveSource is
// skipped for line/block edits (they have no on-disk source).
func TestMergeDotfiles_editEntryLineResolves(t *testing.T) {
	root := t.TempDir()
	writeModule(t, root, "mod", `
[dotfiles]
"~/.zshrc/dev" = { line = "127.0.0.1 dev.local" }
`)
	f := &facts.Facts{Hostname: "h", Username: "u"}

	plan, err := loadAndResolve(t, root, f)
	require.NoError(t, err)
	require.Len(t, plan.Dotfiles.Entries, 1)
	e := plan.Dotfiles.Entries[0]
	require.Equal(t, "127.0.0.1 dev.local", e.Line)
	require.True(t, e.IsEdit())
	require.Empty(t, e.Source, "line edit has no on-disk source")
	require.Empty(t, e.Mode)
}

// mode = "edit" reads the source file's raw contents and uses them as a
// block — no template rendering. The source is resolved across layers; the
// resulting entry is a normal block edit (Mode/Source cleared).
func TestMergeDotfiles_editModeReadsSourceFile(t *testing.T) {
	root := t.TempDir()
	dir := writeModule(t, root, "mod", `
[dotfiles]
"~/.zshrc/aliases" = { source = "aliases.sh", mode = "edit", comment = "#" }
`)
	require.NoError(t, os.WriteFile(filepath.Join(dir, "aliases.sh"), []byte("alias ll='ls -l'\nalias la='ls -la'\n"), 0o644))
	f := &facts.Facts{Hostname: "h", Username: "u"}

	plan, err := loadAndResolve(t, root, f)
	require.NoError(t, err)
	require.Len(t, plan.Dotfiles.Entries, 1)
	e := plan.Dotfiles.Entries[0]
	require.Equal(t, "alias ll='ls -l'\nalias la='ls -la'\n", e.Block, "block should be the source file's raw contents")
	require.Equal(t, "#", e.Comment, "comment should be preserved")
	require.Empty(t, e.Mode, "mode should be translated away")
	require.Empty(t, e.Source, "source should be cleared (block edits have no source)")
	require.True(t, e.IsEdit())
}

// mode = "edit" resolves the source across layers: base declares it, file
// exists only in the user layer → block content comes from the user layer.
func TestMergeDotfiles_editModeResolvesSourceAcrossLayers(t *testing.T) {
	root := t.TempDir()
	writeModule(t, root, "mod", `
[dotfiles]
"~/.zshrc/aliases" = { source = "aliases.sh", mode = "edit" }
`)
	userDir := filepath.Join(root, "users", "u", "modules", "mod")
	require.NoError(t, os.MkdirAll(userDir, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(userDir, "aliases.sh"), []byte("alias fromuser='yes'\n"), 0o644))
	f := &facts.Facts{Hostname: "h", Username: "u"}

	plan, err := loadAndResolve(t, root, f)
	require.NoError(t, err)
	require.Len(t, plan.Dotfiles.Entries, 1)
	require.Equal(t, "alias fromuser='yes'\n", plan.Dotfiles.Entries[0].Block)
}

func TestMergeDotfiles_editModeErrors(t *testing.T) {
	for _, tc := range []struct {
		name   string
		toml   string
		errHas string
	}{
		{
			name:   "mode edit without source",
			toml:   `[dotfiles]` + "\n" + `"~/.zshrc/a" = { mode = "edit" }` + "\n",
			errHas: "mode = \"edit\" requires source",
		},
		{
			name:   "mode edit with line",
			toml:   `[dotfiles]` + "\n" + `"~/.zshrc/a" = { source = "s", mode = "edit", line = "x" }` + "\n",
			errHas: "exclusive with line/block/template",
		},
		{
			name:   "mode edit with block",
			toml:   `[dotfiles]` + "\n" + `"~/.zshrc/a" = { source = "s", mode = "edit", block = "x" }` + "\n",
			errHas: "exclusive with line/block/template",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			root := t.TempDir()
			dir := writeModule(t, root, "mod", tc.toml)
			require.NoError(t, os.WriteFile(filepath.Join(dir, "s"), []byte("x"), 0o644))
			_, err := loadAndResolve(t, root, &facts.Facts{Hostname: "h", Username: "u"})
			require.Error(t, err)
			require.Contains(t, err.Error(), tc.errHas)
		})
	}
}

// A template edit's source resolves across layers: base declares it, the file
// exists only in the user layer → resolved source points at the user layer.
func TestMergeDotfiles_editTemplateResolvesSourceAcrossLayers(t *testing.T) {
	root := t.TempDir()
	writeModule(t, root, "mod", `
[dotfiles]
"~/.gitconfig/id" = { source = "git.tmpl", template = "tera" }
`)
	userDir := filepath.Join(root, "users", "u", "modules", "mod")
	require.NoError(t, os.MkdirAll(userDir, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(userDir, "git.tmpl"), []byte("name = x"), 0o644))
	f := &facts.Facts{Hostname: "h", Username: "u"}

	plan, err := loadAndResolve(t, root, f)
	require.NoError(t, err)
	require.Len(t, plan.Dotfiles.Entries, 1)
	e := plan.Dotfiles.Entries[0]
	require.Equal(t, "tera", e.Template)
	require.True(t, strings.Contains(e.Source, filepath.Join("users", "u", "modules", "mod", "git.tmpl")),
		"template edit source should resolve from the user layer, got %s", e.Source)
}

// Edit entries merge per key: a host overlay overrides only the same key; a
// base-only key survives. (The map key is the full <file>/<id> string, so the
// existing per-key winner loop merges edits for free.)
func TestMergeDotfiles_editLayerWinnerPerKey(t *testing.T) {
	root := t.TempDir()
	writeModule(t, root, "mod", `
[dotfiles]
"~/.zshrc/k1" = { block = "base-A" }
"~/.zshrc/k2" = { block = "base-C" }
`)
	hostDir := filepath.Join(root, "hosts", "h", "modules", "mod")
	require.NoError(t, os.MkdirAll(hostDir, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(hostDir, "module.toml"), []byte(`
[dotfiles]
"~/.zshrc/k1" = { block = "host-B" }
`), 0o644))
	f := &facts.Facts{Hostname: "h", Username: "u"}

	plan, err := loadAndResolve(t, root, f)
	require.NoError(t, err)
	byTarget := make(map[string]resolve.DotfileEntry, len(plan.Dotfiles.Entries))
	for _, e := range plan.Dotfiles.Entries {
		byTarget[e.Target] = e
	}
	require.Equal(t, "host-B", byTarget["~/.zshrc/k1"].Block, "host layer should win for k1")
	require.Equal(t, "host", byTarget["~/.zshrc/k1"].Layer)
	require.Equal(t, "base-C", byTarget["~/.zshrc/k2"].Block, "base-only key k2 should survive")
	require.Equal(t, "base", byTarget["~/.zshrc/k2"].Layer)
}

// Table test over validateEditEntry's seven rules.
func TestMergeDotfiles_editValidationErrors(t *testing.T) {
	cases := []struct {
		name   string
		toml   string
		errHas string
	}{
		{
			name:   "mode set on edit",
			toml:   `[dotfiles]` + "\n" + `"~/.zshrc/a" = { line = "x", mode = "copy" }` + "\n",
			errHas: "must not set mode",
		},
		{
			name:   "both line and block",
			toml:   `[dotfiles]` + "\n" + `"~/.zshrc/a" = { line = "x", block = "y" }` + "\n",
			errHas: "set only one of line or block",
		},
		{
			name:   "template without source",
			toml:   `[dotfiles]` + "\n" + `"~/.zshrc/a" = { template = "tera" }` + "\n",
			errHas: "template edit requires source",
		},
		{
			name:   "template plus line",
			toml:   `[dotfiles]` + "\n" + `"~/.zshrc/a" = { template = "tera", source = "s.tmpl", line = "x" }` + "\n",
			errHas: "template edit must not set line or block",
		},
		{
			name:   "comment without block",
			toml:   `[dotfiles]` + "\n" + `"~/.zshrc/a" = { line = "x", comment = "#" }` + "\n",
			errHas: "comment only applies to block edits",
		},
		{
			name:   "target has no slash",
			toml:   `[dotfiles]` + "\n" + `"noslash" = { line = "x" }` + "\n",
			errHas: "edit target must be <file-path>/<edit-id>",
		},
		{
			name:   "edit id has a space",
			toml:   `[dotfiles]` + "\n" + `"~/.zshrc/bad id" = { line = "x" }` + "\n",
			errHas: "edit id",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			root := t.TempDir()
			writeModule(t, root, "mod", tc.toml)
			_, err := loadAndResolve(t, root, &facts.Facts{Hostname: "h", Username: "u"})
			require.Error(t, err)
			require.Contains(t, err.Error(), tc.errHas)
			require.Contains(t, err.Error(), "mod", "error should name the module")
		})
	}
}

// System-scope edit entries resolve normally — they apply via elevated
// mise dotfiles apply (the only path for in-place system edits).
func TestMergeDotfiles_editSystemScopeResolves(t *testing.T) {
	root := t.TempDir()
	writeModule(t, root, "mod", `
scope = "system"
[dotfiles]
"/etc/hosts/dev" = { line = "127.0.0.1 dev.local" }
`)
	f := &facts.Facts{Hostname: "h", Username: "u"}

	plan, err := loadAndResolve(t, root, f)
	require.NoError(t, err)
	require.Len(t, plan.Dotfiles.Entries, 1)
	e := plan.Dotfiles.Entries[0]
	require.Equal(t, "127.0.0.1 dev.local", e.Line)
	require.Equal(t, profile.ScopeSystem, e.Scope, "system-scope edit entry keeps its scope")
}

// An edit entry on a file another module claims as whole-file is a conflict
// (mise refuses edits through managed symlinks). Cross-module and same-module.
func TestCheckDotfileConflicts_editVsWholeFile(t *testing.T) {
	for _, tc := range []struct {
		name     string
		baseA    string
		baseB    string
		wantSubs []string
	}{
		{
			name:     "cross-module",
			baseA:    `[dotfiles]` + "\n" + `"~/.zshrc" = { source = "zshrc", mode = "symlink" }` + "\n",
			baseB:    `[dotfiles]` + "\n" + `"~/.zshrc/activate" = { block = "eval x" }` + "\n",
			wantSubs: []string{"edit", "~/.zshrc/activate", "whole-file"},
		},
		{
			name: "same-module",
			baseA: `[dotfiles]` + "\n" +
				`"~/.zshrc" = { source = "zshrc", mode = "symlink" }` + "\n" +
				`"~/.zshrc/activate" = { block = "eval x" }` + "\n",
			baseB:    "",
			wantSubs: []string{"edit", "~/.zshrc/activate", "whole-file"},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			root := t.TempDir()
			dirA := writeModule(t, root, "moda", tc.baseA)
			require.NoError(t, os.WriteFile(filepath.Join(dirA, "zshrc"), []byte("x"), 0o644))
			if tc.baseB != "" {
				writeModule(t, root, "modb", tc.baseB)
			}
			f := &facts.Facts{Hostname: "h", Username: "u"}

			_, err := loadAndResolve(t, root, f)
			require.Error(t, err)
			require.Contains(t, err.Error(), "conflict")
			for _, s := range tc.wantSubs {
				require.Contains(t, err.Error(), s)
			}
		})
	}
}
