package editor

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/thedataflows/dotdrift/internal/profile"
)

// The four custom models (0065-D9): the sections whose schema is not
// form-shaped — dotfiles variants, when as validated text, hooks ordering
// and per-row spelling, systemd passthrough.

func dotfilesFixture() *dotfilesAdapter {
	return newDotfilesAdapter(&profile.ModuleConfig{Dotfiles: map[string]profile.Dotfile{
		"~/.zshrc":         {Source: "zshrc", Mode: "symlink"},
		"~/.config/rofi":   {Source: "rofi", Mode: "symlink-each"},
		"~/.zshrc/line":    {Line: "set -x"},
		"~/.zshrc/block":   {Block: "managed", Comment: "#"},
		"~/.bashrc/tpl":    {Source: "banner.tpl", Template: "tera"},
		"/etc/sysctl/edit": {Source: "99-local.conf", Mode: "edit"},
	}})
}

func TestDotfilesEditor_kindBadges(t *testing.T) {
	a := dotfilesFixture()
	require.Equal(t, "whole", kindBadge(a.draft["~/.zshrc"]))
	require.Equal(t, "edit·line", kindBadge(a.draft["~/.zshrc/line"]))
	require.Equal(t, "edit·block", kindBadge(a.draft["~/.zshrc/block"]))
	require.Equal(t, "edit·tpl", kindBadge(a.draft["~/.bashrc/tpl"]))
	require.Equal(t, "edit·src", kindBadge(a.draft["/etc/sysctl/edit"]))

	// The badges surface in the list view too.
	view := a.View(plainPalette{})
	require.Contains(t, view, "[edit·line] ~/.zshrc/line")
	require.Contains(t, view, "[whole] ~/.zshrc")
}

func TestDotfilesEditor_variantSwitch(t *testing.T) {
	a := newDotfilesAdapter(&profile.ModuleConfig{Dotfiles: map[string]profile.Dotfile{
		"~/.zshrc": {Source: "zshrc", Mode: "symlink"},
	}})
	a.cur = 0

	// whole → line
	a.variant()
	require.Equal(t, "edit·line", kindBadge(a.draft["~/.zshrc"]))
	require.Equal(t, "set -x", a.draft["~/.zshrc"].Line)
	require.Empty(t, a.draft["~/.zshrc"].Mode, "the variant switch drops forbidden fields")
	require.Equal(t, "zshrc", a.draft["~/.zshrc"].Source, "the source survives the edit variants")

	// line → block
	a.variant()
	require.Equal(t, "edit·block", kindBadge(a.draft["~/.zshrc"]))

	// block → template
	a.variant()
	require.Equal(t, "edit·tpl", kindBadge(a.draft["~/.zshrc"]))
	require.Equal(t, "tera", a.draft["~/.zshrc"].Template)

	// template → whole: the source survives, the edit fields go.
	a.variant()
	require.Equal(t, "whole", kindBadge(a.draft["~/.zshrc"]))
	require.Empty(t, a.draft["~/.zshrc"].Template)
	require.Equal(t, "zshrc", a.draft["~/.zshrc"].Source,
		"the variant switch keeps the source it can keep")

	// Each switch dirties; the block re-encodes canonically.
	require.True(t, a.Dirty())
	require.Equal(t, "[dotfiles]\n\"~/.zshrc\" = { source = \"zshrc\" }\n", a.Block())
}

func TestDotfilesEditor_contract18Exclusivity(t *testing.T) {
	a := dotfilesFixture()
	require.Empty(t, a.Errors(), "the fixture entries are all legal")

	// line + block are exclusive.
	a.draft["~/.zshrc/line"] = profile.Dotfile{Line: "set -x", Block: "managed"}
	require.Contains(t, a.Errors(), "\"~/.zshrc/line\": line and block are exclusive")

	// mode = "edit" is exclusive with line/block/template and needs a source.
	a.draft["~/.zshrc/line"] = profile.Dotfile{Source: "s", Mode: "edit", Line: "set -x"}
	require.Contains(t, a.Errors(), "\"~/.zshrc/line\": mode = \"edit\" is exclusive with line/block/template")
	a.draft["~/.zshrc/line"] = profile.Dotfile{Mode: "edit"}
	require.Contains(t, a.Errors(), "\"~/.zshrc/line\": mode = \"edit\" requires source")

	// mode set on an edit entry.
	a.draft["~/.zshrc/line"] = profile.Dotfile{Line: "set -x", Mode: "copy"}
	require.Contains(t, a.Errors(), "\"~/.zshrc/line\": edit entry must not set mode")

	// An edit variant is keyed <file-path>/<edit-id>.
	a.draft["plainline"] = profile.Dotfile{Line: "set -x"}
	require.Contains(t, a.Errors(), "\"plainline\": edit entries are keyed <file-path>/<edit-id>")
}

func TestWhenEditor_grammarValidationPositions(t *testing.T) {
	a := newWhenAdapter(&profile.ModuleConfig{})
	require.False(t, a.Dirty())
	require.Empty(t, a.Errors(), "an empty when is always selected, never broken")

	// A good expression validates and re-encodes through the section encoder.
	a.body = "hosts = [\"box\"]\n"
	require.Empty(t, a.Errors())
	require.True(t, a.Dirty())
	require.Equal(t, "[when]\nhosts = [\"box\"]\n", a.Block())

	// An unknown key on the SECOND body line is reported positioned —
	// shifted one line below the synthesized [when] header.
	a.body = "gpu = \"nvidia\"\nbogus = 1\n"
	errs := a.Errors()
	require.Len(t, errs, 1)
	require.Contains(t, errs[0], "line 2", "the position points into the body, not the header: %q", errs[0])
	require.Contains(t, errs[0], "bogus")

	// A malformed kernel constraint fails profile's own grammar (no
	// source position exists for grammar errors — the message names it).
	a.body = "kernel = \"wat\"\n"
	require.Contains(t, stringsJoin(a.Errors()), "invalid kernel constraint")

	// An empty or-list fails the same grammar.
	a.body = "or = []\n"
	require.Contains(t, stringsJoin(a.Errors()), "when.or")

	// Editing flows through the tiny line editor.
	b := newWhenAdapter(&profile.ModuleConfig{})
	b.HandleKey("enter") // begin edit
	for _, k := range []string{"g", "p", "u", " ", "=", " ", "\"", "n", "\""} {
		b.HandleKey(k)
	}
	require.Equal(t, "gpu = \"n\"", b.body)
	b.HandleKey("esc")
	require.False(t, b.edit)
}

func stringsJoin(errs []string) string {
	out := ""
	for _, e := range errs {
		out += e
	}
	return out
}

func TestHooksEditor_orderedRows(t *testing.T) {
	a := newHooksAdapter(&profile.ModuleConfig{})
	a.HandleKey("n") // prompt
	for _, k := range []string{"e", "c", "h", "o", " ", "a"} {
		a.HandleKey(k)
	}
	a.HandleKey("enter")
	a.HandleKey("n")
	for _, k := range []string{"e", "c", "h", "o", " ", "b"} {
		a.HandleKey(k)
	}
	a.HandleKey("enter")
	require.Equal(t, "[hooks]\npre = [\"echo a\", \"echo b\"]\n", a.Block(),
		"rows keep their order")

	// Move the second row up: the order is the data.
	a.HandleKey(">")
	a.HandleKey("<")
	a.HandleKey("<")
	require.Equal(t, "[hooks]\npre = [\"echo b\", \"echo a\"]\n", a.Block())
	require.True(t, a.Dirty())

	// Deletion.
	a.HandleKey("x")
	require.Equal(t, "[hooks]\npre = [\"echo a\"]\n", a.Block())
}

func TestHooksEditor_spellingPerRow(t *testing.T) {
	a := newHooksAdapter(&profile.ModuleConfig{Hooks: profile.Hooks{
		Pre: []profile.HookCommand{{Command: "echo plain"}, {Command: "echo opt", Optional: true}},
	}})
	// The decoded optional row keeps the structured spelling.
	require.Equal(t,
		"[hooks]\npre = [\"echo plain\", { command = \"echo opt\", optional = true }]\n",
		a.Block(), "plain string and inline table spellings coexist per row")

	// Toggling the spelling of the plain row makes it structured, command-only.
	a.cur = 0
	a.HandleKey("v")
	require.Equal(t,
		"[hooks]\npre = [{ command = \"echo plain\" }, { command = \"echo opt\", optional = true }]\n",
		a.Block())

	// The optional checkbox rides the structured row.
	a.HandleKey(" ")
	require.True(t, a.pre[0].optional)
	require.Contains(t, a.Block(), "{ command = \"echo plain\", optional = true }")

	// Untoggling optional on a structured row keeps it structured (the
	// user chose the spelling; the checkbox is orthogonal).
	a.HandleKey(" ")
	require.True(t, a.pre[0].structured)
	require.Contains(t, a.Block(), "{ command = \"echo plain\" }")
}

func TestSystemdEditor_directivePassthrough(t *testing.T) {
	a := newSystemdAdapter(&profile.ModuleConfig{Systemd: profile.SystemdSpec{Units: map[string]profile.SystemdUnit{
		"app.service": {"Environment": "KEY=1", "WantedBy": "default.target", "RemainAfterExit": true},
	}}})
	require.False(t, a.Dirty())
	require.Equal(t,
		"[systemd.units.\"app.service\"]\nEnvironment = \"KEY=1\"\nRemainAfterExit = true\nWantedBy = \"default.target\"\n",
		a.Block(), "directives pass through with their types, keys sorted")

	// Editing a directive's raw TOML text keeps types: the block re-parses
	// the raw values into the typed draft.
	a.HandleKey("enter") // into the unit
	a.curDir = 0         // Environment (sorted)
	a.HandleKey("enter")
	a.editVal.set("\"KEY=2\"")
	a.raw["app.service"]["Environment"] = a.editVal.String()
	block := a.Block()
	require.Contains(t, block, "Environment = \"KEY=2\"")
	require.Equal(t, "KEY=2", a.draft["app.service"]["Environment"],
		"the parsed string survives the raw round trip")
	require.True(t, a.Dirty())
}

func TestSystemdEditor_kindBadge(t *testing.T) {
	require.Equal(t, "service", unitKindBadge("app.service"))
	require.Equal(t, "timer", unitKindBadge("app.timer"))
	require.Equal(t, "unit", unitKindBadge("mounts.path"))
	require.Contains(t, newSystemdAdapter(&profile.ModuleConfig{Systemd: profile.SystemdSpec{
		Units: map[string]profile.SystemdUnit{"app.timer": {}, "app.service": {}},
	}}).View(plainPalette{}), "[timer] app.timer")
}

func TestSystemdEditor_badValueIsLiveError(t *testing.T) {
	a := newSystemdAdapter(&profile.ModuleConfig{Systemd: profile.SystemdSpec{
		Units: map[string]profile.SystemdUnit{"x.service": {"Environment": "KEY=1"}},
	}})
	a.raw["x.service"]["Environment"] = "not toml ]"
	require.NotEmpty(t, a.Errors(), "a directive value that is not TOML is a live error")
}
