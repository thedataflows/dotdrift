package mise_test

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/thedataflows/dotdrift/internal/mise"
	"github.com/thedataflows/dotdrift/internal/resolve"
)

// Issue 0158: a whole-file entry whose source is another entry target in
// the same config declares depends on that target, so mise writes the
// target before it reads the source. One apply pass converges a chain.
func TestGenerateDotfiles_dependsChainedPair(t *testing.T) {
	out, err := mise.GenerateDotfiles([]resolve.DotfileEntry{
		{Target: "/out/.profile", Source: "src/profile", Mode: "copy"},
		{Target: "/out/.bashrc", Source: "/out/.profile", Mode: "symlink"},
	})
	require.NoError(t, err)
	require.Contains(t, out, "\"/out/.bashrc\" = { source = \"/out/.profile\", mode = \"symlink\", depends = [\"/out/.profile\"] }")
	require.Contains(t, out, "\"/out/.profile\" = { source = \"src/profile\", mode = \"copy\" }")
}

// Declaration order does not matter: the dependency is computed from
// source and target paths, not from position in the list.
func TestGenerateDotfiles_dependsReverseOrder(t *testing.T) {
	out, err := mise.GenerateDotfiles([]resolve.DotfileEntry{
		{Target: "/out/.bashrc", Source: "/out/.profile", Mode: "symlink"},
		{Target: "/out/.profile", Source: "src/profile", Mode: "copy"},
	})
	require.NoError(t, err)
	require.Contains(t, out, "depends = [\"/out/.profile\"]")
}

// A source under a symlink-each target directory depends on that target.
func TestGenerateDotfiles_dependsUnderTargetDir(t *testing.T) {
	out, err := mise.GenerateDotfiles([]resolve.DotfileEntry{
		{Target: "/out/config", Source: "cfgdir", Mode: "symlink-each"},
		{Target: "/out/toolrc", Source: "/out/config/tool/rc", Mode: "copy"},
	})
	require.NoError(t, err)
	require.Contains(t, out, "\"/out/toolrc\" = { source = \"/out/config/tool/rc\", mode = \"copy\", depends = [\"/out/config\"] }")
}

// Edit entries never declare depends and never satisfy one: mise does
// not accept the key for edits (dotfiles reference, 2026.11.9).
func TestGenerateDotfiles_noDependsForEdits(t *testing.T) {
	out, err := mise.GenerateDotfiles([]resolve.DotfileEntry{
		{Target: "/out/.zshrc/aliases", Block: "alias x=y"},
		{Target: "/out/copy", Source: "/out/.zshrc", Mode: "copy"},
	})
	require.NoError(t, err)
	require.NotContains(t, out, "depends")
}

// A source dependency cycle, including an entry that sources itself, is
// an error that names the targets. mise would fail the whole apply.
func TestGenerateDotfiles_dependsCycleErrors(t *testing.T) {
	_, err := mise.GenerateDotfiles([]resolve.DotfileEntry{
		{Target: "/out/a", Source: "/out/b", Mode: "copy"},
		{Target: "/out/b", Source: "/out/a", Mode: "copy"},
	})
	require.Error(t, err)
	require.Contains(t, err.Error(), "/out/a")
	require.Contains(t, err.Error(), "/out/b")

	_, err = mise.GenerateDotfiles([]resolve.DotfileEntry{
		{Target: "/out/a", Source: "/out/a", Mode: "copy"},
	})
	require.Error(t, err)
	require.Contains(t, err.Error(), "/out/a")
}

// Every complete generated config states the mise floor itself, so an
// old mise refuses it with a clear message even when dotdrift version
// check does not run again (elevated child, hand run).
func TestGenerateConfig_minVersion(t *testing.T) {
	plan := &resolve.Plan{
		Tools: resolve.ToolsStep{Versions: map[string]string{"node": "20"}},
	}
	out, err := mise.GenerateConfig(plan)
	require.NoError(t, err)
	require.True(t, len(out) > 0)
	require.Equal(t, "min_version = \"2026.11.9\"", firstLine(out))
}

func TestMinMiseVersion_current(t *testing.T) {
	require.Equal(t, "2026.11.9", mise.MinMiseVersion)
}

// A chained user-scope pair converges through the ordinary DotfilesStep:
// the written config carries depends, and FakeRunner records one apply.
func TestDotfilesStep_chainedPairWritesDepends(t *testing.T) {
	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "mise.toml")
	profileTarget := filepath.Join(dir, "home", ".profile")
	plan := &resolve.Plan{Dotfiles: resolve.DotfilesStep{Entries: []resolve.DotfileEntry{
		{Target: filepath.Join(dir, "home", ".bashrc"), Source: profileTarget, Mode: "symlink"},
		{Target: profileTarget, Source: "src/profile", Mode: "copy"},
	}}}
	fr := &mise.FakeRunner{}
	step := &mise.DotfilesStep{Runner: fr, Plan: plan, ConfigPath: cfgPath, Yes: true}
	require.NoError(t, step.Run(context.Background()))

	cfg, err := os.ReadFile(cfgPath)
	require.NoError(t, err)
	require.Contains(t, string(cfg), "min_version = \"2026.11.9\"")
	require.Contains(t, string(cfg), "depends = [\""+profileTarget+"\"]")
}

func firstLine(s string) string {
	for i := 0; i < len(s); i++ {
		if s[i] == '\n' {
			return s[:i]
		}
	}
	return s
}
