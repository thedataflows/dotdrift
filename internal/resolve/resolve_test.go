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

// mise ignores entries with an empty mode ("unknown mode '', ignoring
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
	for _, mode := range []string{"link", "symlink-each", "copy", "template"} {
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
