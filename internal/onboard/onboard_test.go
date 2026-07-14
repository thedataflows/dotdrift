package onboard_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/thedataflows/dotdrift/internal/mise"
	"github.com/thedataflows/dotdrift/internal/onboard"
)

func TestPathMap_homeAndSystem(t *testing.T) {
	// Exposed through Run behavior; mapPath is internal, so we test via the
	// resulting module.toml source entries after a successful onboard.
	home := t.TempDir()
	profile := t.TempDir()

	src := filepath.Join(home, ".bashrc")
	require.NoError(t, writeFile(src, "bashrc"))
	sys := filepath.Join("/etc", "pacman.conf")

	fr := &mise.FakeRunner{}
	o := &onboard.Onboard{Mise: fr}
	err := o.Run(onboard.Options{
		ProfileRoot: profile,
		Paths:       []string{src, sys},
		App:         "bash",
		Mode:        "link",
		Home:        home,
	})
	require.NoError(t, err)

	modToml := filepath.Join(profile, "modules", "bash", "module.toml")
	require.FileExists(t, modToml)
	content, err := readFile(modToml)
	require.NoError(t, err)
	require.Contains(t, content, `"~/.bashrc"`)
	require.Contains(t, content, `"home/.bashrc"`)
	require.Contains(t, content, `"/etc/pacman.conf"`)
	require.Contains(t, content, `"system/etc/pacman.conf"`)
}

func TestInferApp_configDir(t *testing.T) {
	home := t.TempDir()
	profile := t.TempDir()

	src := filepath.Join(home, ".config", "nvim", "init.lua")
	require.NoError(t, writeFile(src, "init"))

	fr := &mise.FakeRunner{}
	o := &onboard.Onboard{Mise: fr}
	err := o.Run(onboard.Options{
		ProfileRoot: profile,
		Paths:       []string{src},
		Home:        home,
	})
	require.NoError(t, err)

	require.DirExists(t, filepath.Join(profile, "modules", "nvim"))
}

func TestOnboard_copiesTree(t *testing.T) {
	home := t.TempDir()
	profile := t.TempDir()

	src := filepath.Join(home, ".config", "app", "config.toml")
	require.NoError(t, writeFile(src, "config"))

	fr := &mise.FakeRunner{}
	o := &onboard.Onboard{Mise: fr}
	err := o.Run(onboard.Options{
		ProfileRoot: profile,
		Paths:       []string{src},
		App:         "app",
		Home:        home,
	})
	require.NoError(t, err)

	require.FileExists(t, filepath.Join(profile, "modules", "app", "home", ".config", "app", "config.toml"))
}

func TestOnboard_writesToml(t *testing.T) {
	home := t.TempDir()
	profile := t.TempDir()

	src := filepath.Join(home, ".bashrc")
	require.NoError(t, writeFile(src, "bashrc"))

	fr := &mise.FakeRunner{}
	o := &onboard.Onboard{Mise: fr}
	err := o.Run(onboard.Options{
		ProfileRoot: profile,
		Paths:       []string{src},
		App:         "bash",
		Mode:        "copy",
		Packages:    []string{"bash"},
		Tools:       []string{"node=20"},
		Home:        home,
	})
	require.NoError(t, err)

	modToml := filepath.Join(profile, "modules", "bash", "module.toml")
	content, err := readFile(modToml)
	require.NoError(t, err)
	require.Contains(t, content, `id = "bash"`)
	require.Contains(t, content, `app = "bash"`)
	require.Contains(t, content, `mode = "copy"`)
	require.Contains(t, content, `"bash"`)
	require.Contains(t, content, `node = "20"`)
}

func TestOnboard_noEnableFlagNeeded(t *testing.T) {
	home := t.TempDir()
	profile := t.TempDir()

	src := filepath.Join(home, ".bashrc")
	require.NoError(t, writeFile(src, "bashrc"))

	fr := &mise.FakeRunner{}
	o := &onboard.Onboard{Mise: fr}
	err := o.Run(onboard.Options{
		ProfileRoot: profile,
		Paths:       []string{src},
		App:         "bash",
		Home:        home,
	})
	require.NoError(t, err)

	require.DirExists(t, filepath.Join(profile, "modules", "bash"))
	require.FileExists(t, filepath.Join(profile, "modules", "bash", "module.toml"))
	// No --enable flag was used; presence selects the module.
}

func TestOnboard_orderCopyThenEnsureThenMiseApply(t *testing.T) {
	home := t.TempDir()
	profile := t.TempDir()

	src := filepath.Join(home, ".bashrc")
	require.NoError(t, writeFile(src, "bashrc"))

	fr := &mise.FakeRunner{}
	o := &onboard.Onboard{Mise: fr}
	err := o.Run(onboard.Options{
		ProfileRoot: profile,
		Paths:       []string{src},
		App:         "bash",
		Home:        home,
	})
	require.NoError(t, err)

	require.True(t, fr.InstallCalled)
	require.True(t, fr.DotfilesCalled)
	require.FileExists(t, filepath.Join(profile, "modules", "bash", "home", ".bashrc"))
}

func TestOnboard_defaultModeLink(t *testing.T) {
	home := t.TempDir()
	profile := t.TempDir()

	src := filepath.Join(home, ".bashrc")
	require.NoError(t, writeFile(src, "bashrc"))

	fr := &mise.FakeRunner{}
	o := &onboard.Onboard{Mise: fr}
	err := o.Run(onboard.Options{
		ProfileRoot: profile,
		Paths:       []string{src},
		App:         "bash",
		Home:        home,
	})
	require.NoError(t, err)

	modToml := filepath.Join(profile, "modules", "bash", "module.toml")
	content, err := readFile(modToml)
	require.NoError(t, err)
	require.Contains(t, content, `mode = "link"`)
}

func TestOnboard_conflictKeepsModule(t *testing.T) {
	home := t.TempDir()
	profile := t.TempDir()

	src := filepath.Join(home, ".bashrc")
	require.NoError(t, writeFile(src, "bashrc"))

	fr := &mise.FakeRunner{}
	o := &onboard.Onboard{Mise: fr}
	err := o.Run(onboard.Options{
		ProfileRoot: profile,
		Paths:       []string{src},
		App:         "bash",
		Home:        home,
	})
	require.NoError(t, err)

	// Onboard the same path again should fail with conflict, but leave module.
	err = o.Run(onboard.Options{
		ProfileRoot: profile,
		Paths:       []string{src},
		App:         "bash",
		Home:        home,
	})
	require.Error(t, err)
	require.Contains(t, err.Error(), "conflict")
	require.FileExists(t, filepath.Join(profile, "modules", "bash", "module.toml"))
}

func TestOnboard_dryRun_noSideEffects(t *testing.T) {
	home := t.TempDir()
	profile := t.TempDir()

	src := filepath.Join(home, ".bashrc")
	require.NoError(t, writeFile(src, "bashrc"))

	fr := &mise.FakeRunner{}
	o := &onboard.Onboard{Mise: fr}
	err := o.Run(onboard.Options{
		ProfileRoot: profile,
		Paths:       []string{src},
		App:         "bash",
		Home:        home,
		DryRun:      true,
	})
	require.NoError(t, err)

	require.NoDirExists(t, filepath.Join(profile, "modules", "bash"))
	require.False(t, fr.InstallCalled)
	require.False(t, fr.DotfilesCalled)
}

func writeFile(path, content string) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	return os.WriteFile(path, []byte(content), 0o644)
}

func readFile(path string) (string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}
	return string(data), nil
}
