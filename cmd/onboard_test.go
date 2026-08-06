package dotdrift

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/alecthomas/kong"
	"github.com/stretchr/testify/require"
	"github.com/thedataflows/dotdrift/internal/facts"
	"github.com/thedataflows/dotdrift/internal/mise"
)

// The onboard command maps its flags onto onboard.Options: detect supplies
// the hostname (host overlay), and Mode/Packages/Tools/Yes flow through to
// the module config and mise runner.
func TestOnboard_mapsCommandFieldsToOptions(t *testing.T) {
	t.Setenv("XDG_STATE_HOME", t.TempDir())
	profDir := t.TempDir()
	live := filepath.Join(t.TempDir(), "live.conf")
	require.NoError(t, os.WriteFile(live, []byte("x=1\n"), 0o644))

	orig := detectFacts
	detectFacts = func() (*facts.Facts, error) { return &facts.Facts{Hostname: "testhost"}, nil }
	t.Cleanup(func() { detectFacts = orig })

	fake := &mise.FakeRunner{}
	cmd := &OnboardCmd{
		Paths:    []string{live},
		Profile:  profDir,
		App:      "myapp",
		Mode:     "copy",
		Packages: []string{"ripgrep"},
		Tools:    []string{"node=20"},
		Host:     true,
		Yes:      true,
		Mise:     fake,
	}
	require.NoError(t, cmd.Run())

	require.True(t, fake.InstallCalled, "mise install not called")
	require.True(t, fake.DotfilesCalled, "mise dotfiles apply not called")
	require.True(t, fake.Yes, "--yes must flow to mise dotfiles apply")

	// Hostname from detect selects the host overlay directory.
	moduleDir := filepath.Join(profDir, "hosts", "testhost", "modules", "myapp")
	data, err := os.ReadFile(filepath.Join(moduleDir, "module.toml"))
	require.NoError(t, err)
	cfg := string(data)
	require.Contains(t, cfg, "ripgrep")
	require.Contains(t, cfg, `node = "20"`)
	require.Contains(t, cfg, `mode = "copy"`)

	// The live path was materialized into the module's system tree.
	copied := filepath.Join(moduleDir, "system", strings.TrimPrefix(live, string(filepath.Separator)))
	require.FileExists(t, copied)
}

// --force must parse and flow through to onboard.Options.Force so a
// conflicting path is re-materialized instead of erroring.
func TestOnboard_forceFlagParses(t *testing.T) {
	var cli CLI
	parser, err := kong.New(&cli, kong.Name(appName))
	require.NoError(t, err)
	_, err = parser.Parse([]string{"onboard", "--force", filepath.Join(t.TempDir(), "x")})
	require.NoError(t, err)
	require.True(t, cli.Onboard.Force)
}

func TestOnboard_forceFlowsToOptions(t *testing.T) {
	t.Setenv("XDG_STATE_HOME", t.TempDir())
	profDir := t.TempDir()
	live := filepath.Join(t.TempDir(), "live.conf")
	require.NoError(t, os.WriteFile(live, []byte("v1\n"), 0o644))

	orig := detectFacts
	detectFacts = func() (*facts.Facts, error) { return &facts.Facts{Hostname: "testhost"}, nil }
	t.Cleanup(func() { detectFacts = orig })

	run := func(force bool) error {
		cmd := &OnboardCmd{
			Paths:   []string{live},
			Profile: profDir,
			App:     "myapp",
			Force:   force,
			Mise:    &mise.FakeRunner{},
		}
		return cmd.Run()
	}

	require.NoError(t, run(false))
	require.ErrorContains(t, run(false), "conflict")

	require.NoError(t, os.WriteFile(live, []byte("v2\n"), 0o644))
	require.NoError(t, run(true))

	copied := filepath.Join(profDir, "modules", "myapp", "system", strings.TrimPrefix(live, string(filepath.Separator)))
	data, err := os.ReadFile(copied)
	require.NoError(t, err)
	require.Equal(t, "v2\n", string(data))
}

func TestOnboard_detectErrorPropagates(t *testing.T) {
	orig := detectFacts
	detectFacts = func() (*facts.Facts, error) { return nil, errors.New("no facts") }
	t.Cleanup(func() { detectFacts = orig })

	cmd := &OnboardCmd{Paths: []string{"/x"}, Profile: t.TempDir(), Mise: &mise.FakeRunner{}}
	err := cmd.Run()
	require.ErrorContains(t, err, "detect")
}

// --verbose flows into the real mise construction path: with no injected
// runner, onboard builds its mise through the defaultMise seam and sets
// Verbose on it; without the flag it stays quiet.
func TestOnboard_verbosePropagation(t *testing.T) {
	for _, verbose := range []bool{true, false} {
		t.Run(fmt.Sprintf("verbose=%v", verbose), func(t *testing.T) {
			t.Setenv("XDG_STATE_HOME", t.TempDir())
			profDir := t.TempDir()
			live := filepath.Join(t.TempDir(), "live.conf")
			require.NoError(t, os.WriteFile(live, []byte("x=1\n"), 0o644))

			origDetect := detectFacts
			detectFacts = func() (*facts.Facts, error) { return &facts.Facts{Hostname: "testhost"}, nil }
			origMise := defaultMise
			events := &[]string{}
			var captured *mise.Mise
			defaultMise = func() *mise.Mise { captured = fakeMise(events); return captured }
			t.Cleanup(func() { detectFacts, defaultMise = origDetect, origMise })

			cmd := &OnboardCmd{
				Paths:   []string{live},
				Profile: profDir,
				App:     "myapp",
				Verbose: verbose,
			}
			require.NoError(t, cmd.Run())
			require.NotNil(t, captured, "onboard must build its mise through the defaultMise seam when no runner is injected")
			require.Equal(t, verbose, captured.Verbose, "mise.Verbose must mirror the flag")
		})
	}
}

// Every dotfile mode documented in docs/product/profile-layout.md must parse.
func TestOnboard_modeFlag_acceptsDocumentedModes(t *testing.T) {
	for _, mode := range []string{"symlink", "copy", "template", "symlink-each"} {
		t.Run(mode, func(t *testing.T) {
			var cli CLI
			parser, err := kong.New(&cli, kong.Name(appName))
			require.NoError(t, err)
			_, err = parser.Parse([]string{"onboard", "--mode", mode, filepath.Join(t.TempDir(), "x")})
			require.NoError(t, err, "--mode %s is documented in docs/product/profile-layout.md and must parse", mode)
			require.Equal(t, mode, cli.Onboard.Mode)
		})
	}
}

// No --mode flag: the CLI default must be symlink (contract.md invariant 5),
// not symlink-each — real mise rejects symlink-each for file sources, which
// broke docker-e2e onboarding of a live file.
func TestOnboard_modeFlag_defaultIsSymlink(t *testing.T) {
	var cli CLI
	parser, err := kong.New(&cli, kong.Name(appName))
	require.NoError(t, err)
	_, err = parser.Parse([]string{"onboard", filepath.Join(t.TempDir(), "x")})
	require.NoError(t, err)
	require.Equal(t, "symlink", cli.Onboard.Mode, "default dotfile mode is symlink (contract.md invariant 5)")
}

func TestOnboard_modeFlowsToModuleTOML(t *testing.T) {
	for _, mode := range []string{"template", "symlink-each"} {
		t.Run(mode, func(t *testing.T) {
			t.Setenv("XDG_STATE_HOME", t.TempDir())
			profDir := t.TempDir()
			live := filepath.Join(t.TempDir(), "live.conf")
			require.NoError(t, os.WriteFile(live, []byte("x=1\n"), 0o644))

			orig := detectFacts
			detectFacts = func() (*facts.Facts, error) { return &facts.Facts{Hostname: "testhost"}, nil }
			t.Cleanup(func() { detectFacts = orig })

			cmd := &OnboardCmd{
				Paths:   []string{live},
				Profile: profDir,
				App:     "myapp",
				Mode:    mode,
				Mise:    &mise.FakeRunner{},
			}
			require.NoError(t, cmd.Run())

			data, err := os.ReadFile(filepath.Join(profDir, "modules", "myapp", "module.toml"))
			require.NoError(t, err)
			require.Contains(t, string(data), `mode = "`+mode+`"`, "module.toml must record the requested mode")
		})
	}
}

// --packages values flow through kong (comma split), ParsePackages, and the
// emitter into a module.toml whose present array carries a trailing
// "# description" comment — the style hand-authored modules use.
func TestOnboard_packagesDescriptionEndToEnd(t *testing.T) {
	t.Setenv("XDG_STATE_HOME", t.TempDir())
	profDir := t.TempDir()
	live := filepath.Join(t.TempDir(), "live.conf")
	require.NoError(t, os.WriteFile(live, []byte("x=1\n"), 0o644))

	orig := detectFacts
	detectFacts = func() (*facts.Facts, error) { return &facts.Facts{Hostname: "testhost"}, nil }
	t.Cleanup(func() { detectFacts = orig })

	cmd := &OnboardCmd{
		Paths:   []string{live},
		Profile: profDir,
		App:     "myapp",
		// kong would split 'bat,fd="Find files"' into these two tokens.
		Packages: []string{"bat", `fd="Find files"`},
		Mise:     &mise.FakeRunner{},
	}
	require.NoError(t, cmd.Run())

	data, err := os.ReadFile(filepath.Join(profDir, "modules", "myapp", "module.toml"))
	require.NoError(t, err)
	body := string(data)
	require.Contains(t, body, "  \"bat\",\n")
	require.Contains(t, body, "  \"fd\", # Find files\n")
}
