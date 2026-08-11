package mise_test

import (
	"context"
	"errors"
	"testing"

	"github.com/BurntSushi/toml"
	"github.com/stretchr/testify/require"
	"github.com/thedataflows/dotdrift/internal/mise"
	"github.com/thedataflows/dotdrift/internal/resolve"
)

func TestGenerateTools(t *testing.T) {
	out := mise.GenerateTools(map[string]string{
		"node":   "20",
		"python": "3.12",
		"rust":   "stable",
	})
	require.Contains(t, out, "[tools]")
	require.Contains(t, out, `node = "20"`)
	require.Contains(t, out, `python = "3.12"`)
	require.Contains(t, out, `rust = "stable"`)
}

// dotdrift's mode vocabulary is exactly mise's (symlink, symlink-each, copy,
// template), so GenerateDotfiles emits modes unchanged.
func TestGenerateDotfiles(t *testing.T) {
	out := mise.GenerateDotfiles([]resolve.DotfileEntry{
		{Target: "~/.bashrc", Source: ".bashrc", Mode: "symlink"},
		{Target: "~/.config/nvim", Source: "nvim", Mode: "symlink-each"},
	})
	require.Contains(t, out, "[dotfiles]")
	require.Contains(t, out, `"~/.bashrc" = { source = ".bashrc", mode = "symlink" }`)
	require.Contains(t, out, `"~/.config/nvim" = { source = "nvim", mode = "symlink-each" }`)
}

// Every documented mode is valid mise vocabulary (verified against mise
// 2026.7.10) and must pass through unchanged.
func TestGenerateDotfiles_passthroughModes(t *testing.T) {
	for _, mode := range []string{"symlink", "copy", "template", "symlink-each"} {
		t.Run(mode, func(t *testing.T) {
			out := mise.GenerateDotfiles([]resolve.DotfileEntry{
				{Target: "~/target", Source: "src", Mode: mode},
			})

			var decoded struct {
				Dotfiles map[string]struct {
					Mode string `toml:"mode"`
				} `toml:"dotfiles"`
			}
			_, err := toml.Decode(out, &decoded)
			require.NoError(t, err, "generated TOML must be parseable: %q", out)
			require.Equal(t, mode, decoded.Dotfiles["~/target"].Mode)
		})
	}
}

func TestGenerateConfig(t *testing.T) {
	plan := &resolve.Plan{
		Tools: resolve.ToolsStep{Versions: map[string]string{"node": "20"}},
		Dotfiles: resolve.DotfilesStep{Entries: []resolve.DotfileEntry{
			{Target: "~/.bashrc", Source: ".bashrc", Mode: "symlink"},
		}},
	}
	out := mise.GenerateConfig(plan)
	require.Contains(t, out, "[tools]")
	require.Contains(t, out, "[dotfiles]")
	require.Contains(t, out, `node = "20"`)
	require.Contains(t, out, `"~/.bashrc" = { source = ".bashrc", mode = "symlink" }`)
}

func TestToolsStep_callsInstall(t *testing.T) {
	fr := &mise.FakeRunner{}
	plan := &resolve.Plan{Tools: resolve.ToolsStep{Versions: map[string]string{"node": "20"}}}
	step := &mise.ToolsStep{Runner: fr, Plan: plan, ConfigPath: "/tmp/mise-tools.toml"}
	require.Equal(t, "tools", step.Name())
	require.NoError(t, step.Run(context.Background()))
	require.True(t, fr.InstallCalled)
	require.False(t, fr.DotfilesCalled)
}

func TestToolsStep_failurePersistsError(t *testing.T) {
	boom := errors.New("boom")
	fr := &mise.FakeRunner{Err: boom}
	plan := &resolve.Plan{Tools: resolve.ToolsStep{Versions: map[string]string{"node": "20"}}}
	step := &mise.ToolsStep{Runner: fr, Plan: plan, ConfigPath: "/tmp/mise-tools.toml"}
	err := step.Run(context.Background())
	require.ErrorIs(t, err, boom)
}

func TestDotfilesStep_callsApply(t *testing.T) {
	fr := &mise.FakeRunner{}
	plan := &resolve.Plan{Dotfiles: resolve.DotfilesStep{Entries: []resolve.DotfileEntry{
		{Target: "~/.bashrc", Source: ".bashrc", Mode: "symlink"},
	}}}
	step := &mise.DotfilesStep{Runner: fr, Plan: plan, ConfigPath: "/tmp/mise-dotfiles.toml", Yes: true}
	require.Equal(t, "dotfiles", step.Name())
	require.NoError(t, step.Run(context.Background()))
	require.True(t, fr.DotfilesCalled)
	require.True(t, fr.Yes)
	require.False(t, fr.InstallCalled)
}

func TestDotfilesStep_conflictStops(t *testing.T) {
	boom := errors.New("conflict")
	fr := &mise.FakeRunner{Err: boom}
	plan := &resolve.Plan{Dotfiles: resolve.DotfilesStep{Entries: []resolve.DotfileEntry{
		{Target: "~/.bashrc", Source: ".bashrc", Mode: "symlink"},
	}}}
	step := &mise.DotfilesStep{Runner: fr, Plan: plan, ConfigPath: "/tmp/mise-dotfiles.toml", Yes: false}
	err := step.Run(context.Background())
	require.ErrorIs(t, err, boom)
}

func TestGenerateTools_escapesQuotesAndBackslashes(t *testing.T) {
	versions := map[string]string{
		"node": `20 "lts"`,
		"java": `C:\Program Files\Java\21`,
	}
	out := mise.GenerateTools(versions)

	var decoded struct {
		Tools map[string]string `toml:"tools"`
	}
	_, err := toml.Decode(out, &decoded)
	require.NoError(t, err, "generated TOML must be parseable: %q", out)
	require.Equal(t, versions, decoded.Tools)
}

func TestGenerateDotfiles_escapesQuotesAndBackslashes(t *testing.T) {
	entries := []resolve.DotfileEntry{
		{Target: `~/weird"dir\file`, Source: `mod\src"x`, Mode: "symlink"},
		{Target: "~/.bashrc", Source: ".bashrc", Mode: "symlink"},
	}
	out := mise.GenerateDotfiles(entries)

	var decoded struct {
		Dotfiles map[string]struct {
			Source string `toml:"source"`
			Mode   string `toml:"mode"`
		} `toml:"dotfiles"`
	}
	_, err := toml.Decode(out, &decoded)
	require.NoError(t, err, "generated TOML must be parseable: %q", out)
	require.Len(t, decoded.Dotfiles, 2)
	require.Equal(t, `mod\src"x`, decoded.Dotfiles[`~/weird"dir\file`].Source)
	require.Equal(t, "symlink", decoded.Dotfiles[`~/weird"dir\file`].Mode)
	require.Equal(t, ".bashrc", decoded.Dotfiles["~/.bashrc"].Source)
}

// Edit entries emit the three mise forms: { line }, { block[, comment] },
// { source, template }. Whole-file entries still emit { source, mode }.
func TestGenerateDotfiles_editEntries(t *testing.T) {
	entries := []resolve.DotfileEntry{
		{Target: "~/.zshrc/dev", Line: "127.0.0.1 dev.local"},
		{Target: "~/.zshrc/aliases", Block: "alias ll='ls -l'", Comment: "#"},
		{Target: "~/.zshrc/activate", Block: `eval "$(mise activate zsh)"`},
		{Target: "~/.gitconfig/id", Source: "snippets/git.tmpl", Template: "tera"},
		{Target: "~/.bashrc", Source: ".bashrc", Mode: "symlink"},
	}
	out := mise.GenerateDotfiles(entries)

	require.Contains(t, out, `"~/.zshrc/dev" = { line = "127.0.0.1 dev.local" }`)
	require.Contains(t, out, `"~/.zshrc/aliases" = { block = "alias ll='ls -l'", comment = "#" }`)
	require.Contains(t, out, `"~/.zshrc/activate" = { block = "eval \"$(mise activate zsh)\"" }`)
	require.Contains(t, out, `"~/.gitconfig/id" = { source = "snippets/git.tmpl", template = "tera" }`)
	require.Contains(t, out, `"~/.bashrc" = { source = ".bashrc", mode = "symlink" }`)

	// The whole section must stay parseable TOML.
	var decoded struct {
		Dotfiles map[string]struct {
			Source   string `toml:"source"`
			Mode     string `toml:"mode"`
			Line     string `toml:"line"`
			Block    string `toml:"block"`
			Comment  string `toml:"comment"`
			Template string `toml:"template"`
		} `toml:"dotfiles"`
	}
	_, err := toml.Decode(out, &decoded)
	require.NoError(t, err, "generated TOML must be parseable: %q", out)
	require.Equal(t, "127.0.0.1 dev.local", decoded.Dotfiles["~/.zshrc/dev"].Line)
	require.Equal(t, "alias ll='ls -l'", decoded.Dotfiles["~/.zshrc/aliases"].Block)
	require.Equal(t, "#", decoded.Dotfiles["~/.zshrc/aliases"].Comment)
}

// A multi-line block (newlines, quotes, backslashes) round-trips byte-identical
// through BurntSushi — the same decode mise performs.
func TestGenerateDotfiles_editBlockMultilineRoundTrip(t *testing.T) {
	block := "alias ll='ls -l'\nalias la='ls -la'\nexport PATH=\"$HOME/bin:$PATH\""
	entries := []resolve.DotfileEntry{
		{Target: "~/.zshrc/aliases", Block: block, Comment: "#"},
	}
	out := mise.GenerateDotfiles(entries)

	var decoded struct {
		Dotfiles map[string]struct {
			Block   string `toml:"block"`
			Comment string `toml:"comment"`
		} `toml:"dotfiles"`
	}
	_, err := toml.Decode(out, &decoded)
	require.NoError(t, err, "generated TOML must be parseable: %q", out)
	require.Equal(t, block, decoded.Dotfiles["~/.zshrc/aliases"].Block,
		"multi-line block must round-trip byte-identical")
	require.Equal(t, "#", decoded.Dotfiles["~/.zshrc/aliases"].Comment)
}
