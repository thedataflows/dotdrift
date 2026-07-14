package profile_test

import (
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
	require.Equal(t, profile.Dotfile{Source: ".bashrc", Mode: "link"}, m.Config.Dotfiles["~/.bashrc"])
	require.Equal(t, profile.Dotfile{Source: "nvim", Mode: "symlink-each"}, m.Config.Dotfiles["~/.config/nvim"])
	require.Equal(t, profile.Dotfile{Source: "config.toml", Mode: "copy"}, m.Config.Dotfiles["~/.config/app/config.toml"])
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
