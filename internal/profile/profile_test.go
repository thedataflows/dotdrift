package profile_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/thedataflows/dotdrift/internal/facts"
	"github.com/thedataflows/dotdrift/internal/profile"
)

func fixture(t *testing.T, name string) string {
	t.Helper()
	return filepath.Join("..", "..", "testdata", "profiles", name)
}

func findModule(t *testing.T, p *profile.Profile, id string) profile.Module {
	t.Helper()
	for _, m := range p.Modules {
		if m.ID == id {
			return m
		}
	}
	t.Fatalf("module %q not found", id)
	return profile.Module{}
}

func TestLoadDotdriftTOML_defaults(t *testing.T) {
	p, err := profile.Load(fixture(t, "minimal"), &facts.Facts{})
	require.NoError(t, err)
	require.Empty(t, p.Config.Modules.Disable)
}

func TestLoadDotdriftTOML_disableList(t *testing.T) {
	p, err := profile.Load(fixture(t, "disabled"), &facts.Facts{})
	require.NoError(t, err)
	require.ElementsMatch(t, []string{"a"}, p.Config.Modules.Disable)
}

func TestLoadModuleTOML_defaults(t *testing.T) {
	p, err := profile.Load(fixture(t, "simple"), &facts.Facts{})
	require.NoError(t, err)
	m := findModule(t, p, "b")
	require.Equal(t, "b", m.ID)
	require.Equal(t, "b", m.App)
}

func TestLoadModuleTOML_when(t *testing.T) {
	p, err := profile.Load(fixture(t, "simple"), &facts.Facts{})
	require.NoError(t, err)
	m := findModule(t, p, "named")
	require.Equal(t, "NamedApp", m.App)
	require.Equal(t, []string{"myhost"}, m.Config.When.Hosts)
	require.Equal(t, []string{"cri"}, m.Config.When.Users)
	require.Equal(t, []string{"cachyos"}, m.Config.When.OS)
	require.Equal(t, "nvidia", m.Config.When.GPU)
}

func TestLoadModuleTOML_packages(t *testing.T) {
	p, err := profile.Load(fixture(t, "simple"), &facts.Facts{})
	require.NoError(t, err)
	m := findModule(t, p, "named")
	require.Equal(t, []string{"neovim", "ripgrep"}, m.Config.Packages.Present)
	require.Equal(t, []string{"nano"}, m.Config.Packages.Absent)
}

func TestLoadModuleTOML_tools(t *testing.T) {
	p, err := profile.Load(fixture(t, "simple"), &facts.Facts{})
	require.NoError(t, err)
	m := findModule(t, p, "named")
	require.Equal(t, map[string]string{"node": "20", "python": "3.12"}, m.Config.Tools)
}

func TestLoadModuleTOML_dotfiles(t *testing.T) {
	p, err := profile.Load(fixture(t, "simple"), &facts.Facts{})
	require.NoError(t, err)
	m := findModule(t, p, "named")
	require.Equal(t, profile.Dotfile{Source: ".bashrc", Mode: "symlink"}, m.Config.Dotfiles["~/.bashrc"])
	require.Equal(t, profile.Dotfile{Source: "nvim", Mode: "symlink-each"}, m.Config.Dotfiles["~/.config/nvim"])
	require.Equal(t, profile.Dotfile{Source: "config.toml", Mode: "copy"}, m.Config.Dotfiles["~/.config/app/config.toml"])
}

// Edit entries (line/block/template partial edits, keyed by "<file>/<id>")
// decode into the new Dotfile fields exactly as mise spells them.
func TestLoadModuleTOML_dotfileEditEntries(t *testing.T) {
	root := t.TempDir()
	modDir := filepath.Join(root, "modules", "edits")
	require.NoError(t, os.MkdirAll(modDir, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(modDir, "module.toml"), []byte(`
[dotfiles]
"~/.zshrc/activate" = { block = 'eval "$(mise activate zsh)"' }
"~/.zshrc/aliases" = { block = "alias ll='ls -l'", comment = "#" }
"/etc/hosts/dev" = { line = "127.0.0.1 dev.local" }
"~/.gitconfig/id" = { source = "snippets/git.tmpl", template = "tera" }
`), 0o644))

	p, err := profile.Load(root, &facts.Facts{})
	require.NoError(t, err)
	m := findModule(t, p, "edits")
	require.Equal(t, profile.Dotfile{Block: `eval "$(mise activate zsh)"`}, m.Config.Dotfiles["~/.zshrc/activate"])
	require.Equal(t, profile.Dotfile{Block: "alias ll='ls -l'", Comment: "#"}, m.Config.Dotfiles["~/.zshrc/aliases"])
	require.Equal(t, profile.Dotfile{Line: "127.0.0.1 dev.local"}, m.Config.Dotfiles["/etc/hosts/dev"])
	require.Equal(t, profile.Dotfile{Source: "snippets/git.tmpl", Template: "tera"}, m.Config.Dotfiles["~/.gitconfig/id"])
}

func TestDotfile_IsEdit(t *testing.T) {
	require.False(t, profile.Dotfile{Mode: "symlink"}.IsEdit(), "whole-file entry is not an edit")
	require.False(t, profile.Dotfile{}.IsEdit(), "empty entry is not an edit")
	require.True(t, profile.Dotfile{Line: "x"}.IsEdit())
	require.True(t, profile.Dotfile{Block: "x"}.IsEdit())
	require.True(t, profile.Dotfile{Template: "tera"}.IsEdit())
	require.True(t, profile.Dotfile{Mode: "edit"}.IsEdit(), "mode=edit with source is an edit")
}

func TestDiscoverModules_empty(t *testing.T) {
	p, err := profile.Load(fixture(t, "minimal"), &facts.Facts{})
	require.NoError(t, err)
	require.Empty(t, p.Modules)
}

func TestDiscoverModules_multiple(t *testing.T) {
	p, err := profile.Load(fixture(t, "simple"), &facts.Facts{})
	require.NoError(t, err)
	require.Len(t, p.Modules, 2)
	require.ElementsMatch(t, []string{"named", "b"}, []string{p.Modules[0].ID, p.Modules[1].ID})
}

func TestDiscoverModules_missingModuleToml(t *testing.T) {
	p, err := profile.Load(fixture(t, "discover"), &facts.Facts{})
	require.NoError(t, err)
	require.Len(t, p.Modules, 1)
	require.Equal(t, "valid", p.Modules[0].ID)
}

func TestDiscoverModules_missingModulesDir(t *testing.T) {
	dir := fixture(t, "nomodules")
	_, err := profile.Load(dir, &facts.Facts{})
	require.Error(t, err)
	require.Contains(t, err.Error(), "not a dotdrift profile")
	require.Contains(t, err.Error(), dir)
	t.Logf("error: %v", err)
}

func TestDiscoverModules_duplicateIDs(t *testing.T) {
	_, err := profile.Load(fixture(t, "dupids"), &facts.Facts{})
	require.Error(t, err)
	require.Contains(t, err.Error(), `"same"`)
	require.Contains(t, err.Error(), filepath.Join("modules", "one"))
	require.Contains(t, err.Error(), filepath.Join("modules", "two"))
	t.Logf("error: %v", err)
}

func TestLoadConfig_emptyHostnameCollapsedOverlay(t *testing.T) {
	_, err := profile.Load(fixture(t, "collapsed"), &facts.Facts{Username: "cri"})
	require.Error(t, err)
	require.Contains(t, err.Error(), "hostname")
	t.Logf("error: %v", err)
}

func TestLoadConfig_emptyUsernameCollapsedOverlay(t *testing.T) {
	_, err := profile.Load(fixture(t, "collapsed"), &facts.Facts{Hostname: "myhost"})
	require.Error(t, err)
	require.Contains(t, err.Error(), "username")
	t.Logf("error: %v", err)
}

func TestLoadConfig_emptyFactsNoOverlayOK(t *testing.T) {
	p, err := profile.Load(fixture(t, "simple"), &facts.Facts{})
	require.NoError(t, err)
	require.NotEmpty(t, p.Modules)
}

func TestSelection_presenceMeansEnabled(t *testing.T) {
	p, err := profile.Load(fixture(t, "simple"), &facts.Facts{
		Hostname: "myhost",
		Username: "cri",
		OS:       "cachyos",
		GPU:      "nvidia",
	})
	require.NoError(t, err)
	require.ElementsMatch(t, []string{"named", "b"}, selectedIDs(p))
	require.Empty(t, p.Skipped)
}

func TestSelection_disableUnion(t *testing.T) {
	p, err := profile.Load(fixture(t, "layers"), &facts.Facts{
		Hostname: "myhost",
		Username: "cri",
	})
	require.NoError(t, err)
	require.ElementsMatch(t, []string{"d"}, selectedIDs(p))
	require.Len(t, p.Skipped, 3)
	require.ElementsMatch(t, []string{"a", "b", "c"}, skippedIDs(p))
}

func TestSelection_whenFilter(t *testing.T) {
	always := &facts.Facts{Hostname: "other", Username: "other", OS: "other", GPU: "other"}

	t.Run("host", func(t *testing.T) {
		p, err := profile.Load(fixture(t, "whenfilter"), &facts.Facts{Hostname: "myhost"})
		require.NoError(t, err)
		require.Contains(t, selectedIDs(p), "hostonly")
		require.Contains(t, skippedIDs(p), "useronly")
		require.Contains(t, skippedIDs(p), "osonly")
		require.Contains(t, skippedIDs(p), "gpuonly")
		require.Contains(t, skippedIDs(p), "combined")
		require.Contains(t, selectedIDs(p), "always")
	})

	t.Run("user", func(t *testing.T) {
		p, err := profile.Load(fixture(t, "whenfilter"), &facts.Facts{Username: "cri"})
		require.NoError(t, err)
		require.Contains(t, selectedIDs(p), "useronly")
		require.Contains(t, skippedIDs(p), "hostonly")
		require.Contains(t, selectedIDs(p), "always")
	})

	t.Run("os", func(t *testing.T) {
		p, err := profile.Load(fixture(t, "whenfilter"), &facts.Facts{OS: "cachyos"})
		require.NoError(t, err)
		require.Contains(t, selectedIDs(p), "osonly")
		require.Contains(t, skippedIDs(p), "hostonly")
		require.Contains(t, selectedIDs(p), "always")
	})

	t.Run("gpu", func(t *testing.T) {
		p, err := profile.Load(fixture(t, "whenfilter"), &facts.Facts{GPU: "nvidia"})
		require.NoError(t, err)
		require.Contains(t, selectedIDs(p), "gpuonly")
		require.Contains(t, skippedIDs(p), "hostonly")
		require.Contains(t, selectedIDs(p), "always")
	})

	t.Run("combined match", func(t *testing.T) {
		p, err := profile.Load(fixture(t, "whenfilter"), &facts.Facts{
			Hostname: "myhost",
			Username: "cri",
			OS:       "cachyos",
			GPU:      "nvidia",
		})
		require.NoError(t, err)
		require.Contains(t, selectedIDs(p), "combined")
	})

	t.Run("combined mismatch gpu", func(t *testing.T) {
		p, err := profile.Load(fixture(t, "whenfilter"), &facts.Facts{
			Hostname: "myhost",
			Username: "cri",
			OS:       "cachyos",
			GPU:      "amd",
		})
		require.NoError(t, err)
		require.Contains(t, skippedIDs(p), "combined")
		require.Contains(t, selectedIDs(p), "always")
	})

	t.Run("always", func(t *testing.T) {
		p, err := profile.Load(fixture(t, "whenfilter"), always)
		require.NoError(t, err)
		require.Contains(t, selectedIDs(p), "always")
	})

	t.Run("kernel match", func(t *testing.T) {
		p, err := profile.Load(fixture(t, "whenfilter"), &facts.Facts{Kernel: "7.2.0-arch1-1"})
		require.NoError(t, err)
		require.Contains(t, selectedIDs(p), "kernelonly")
	})

	t.Run("kernel mismatch", func(t *testing.T) {
		p, err := profile.Load(fixture(t, "whenfilter"), &facts.Facts{Kernel: "6.12.1"})
		require.NoError(t, err)
		require.Contains(t, skippedIDs(p), "kernelonly")
	})

	t.Run("kernel empty fact", func(t *testing.T) {
		p, err := profile.Load(fixture(t, "whenfilter"), &facts.Facts{})
		require.NoError(t, err)
		require.Contains(t, skippedIDs(p), "kernelonly")
	})

	t.Run("kernel unparseable release", func(t *testing.T) {
		p, err := profile.Load(fixture(t, "whenfilter"), &facts.Facts{Kernel: "not-a-version"})
		require.NoError(t, err)
		require.Contains(t, skippedIDs(p), "kernelonly")
	})
}

func TestLoadModuleTOML_whenKernel(t *testing.T) {
	p, err := profile.Load(fixture(t, "whenfilter"), &facts.Facts{Kernel: "7.2.0"})
	require.NoError(t, err)
	m := findModule(t, p, "kernelonly")
	require.Equal(t, ">= 7.1", m.Config.When.Kernel)
}

func TestLoad_malformedWhenKernel(t *testing.T) {
	_, err := profile.Load(fixture(t, "badkernel"), &facts.Facts{})
	require.Error(t, err)
	require.Contains(t, err.Error(), "bad", "error must name the module")
	require.Contains(t, err.Error(), "~ 7.1", "error must carry the expression")
	t.Logf("error: %v", err)
}

func selectedIDs(p *profile.Profile) []string {
	ids := make([]string, len(p.Selected))
	for i, m := range p.Selected {
		ids[i] = m.ID
	}
	return ids
}

func skippedIDs(p *profile.Profile) []string {
	ids := make([]string, len(p.Skipped))
	for i, s := range p.Skipped {
		ids[i] = s.Module.ID
	}
	return ids
}

// writeModule creates a module directory with a module.toml at the given
// relative path under root.
func writeModule(t *testing.T, root, relPath, tomlContent string) {
	t.Helper()
	dir := filepath.Join(root, relPath)
	require.NoError(t, os.MkdirAll(dir, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "module.toml"), []byte(tomlContent), 0o644))
}

// disabled = true in a module's own module.toml skips it with reason "disabled".
func TestModuleDisabledProperty_baseLayer(t *testing.T) {
	root := t.TempDir()
	writeModule(t, root, "modules/vim", "disabled = true\n")
	writeModule(t, root, "modules/git", "")
	p, err := profile.Load(root, &facts.Facts{})
	require.NoError(t, err)
	require.Equal(t, []string{"git"}, selectedIDs(p))
	require.Equal(t, []string{"vim"}, skippedIDs(p))
	require.Equal(t, "disabled", p.Skipped[0].Reason)
}

// disabled = true in a host overlay's module.toml disables the base module
// (union semantics: any layer disabling sticks).
func TestModuleDisabledProperty_hostOverlay(t *testing.T) {
	root := t.TempDir()
	writeModule(t, root, "modules/vim", "")
	writeModule(t, root, "hosts/myhost/modules/vim", "disabled = true\n")
	p, err := profile.Load(root, &facts.Facts{Hostname: "myhost"})
	require.NoError(t, err)
	require.Empty(t, selectedIDs(p))
	require.Equal(t, []string{"vim"}, skippedIDs(p))
	require.Equal(t, "disabled", p.Skipped[0].Reason)
}

// disabled = true in a user overlay's module.toml disables the base module.
func TestModuleDisabledProperty_userOverlay(t *testing.T) {
	root := t.TempDir()
	writeModule(t, root, "modules/vim", "")
	writeModule(t, root, "users/cri/modules/vim", "disabled = true\n")
	p, err := profile.Load(root, &facts.Facts{Username: "cri"})
	require.NoError(t, err)
	require.Empty(t, selectedIDs(p))
	require.Equal(t, []string{"vim"}, skippedIDs(p))
}

// disabled = false in an overlay does NOT un-disable a module that the base
// layer disabled (any disable sticks — union semantics, same as disable list).
func TestModuleDisabledProperty_overlayFalseDoesNotUnDisable(t *testing.T) {
	root := t.TempDir()
	writeModule(t, root, "modules/vim", "disabled = true\n")
	writeModule(t, root, "hosts/myhost/modules/vim", "disabled = false\n")
	p, err := profile.Load(root, &facts.Facts{Hostname: "myhost"})
	require.NoError(t, err)
	require.Empty(t, selectedIDs(p))
	require.Equal(t, []string{"vim"}, skippedIDs(p))
}
