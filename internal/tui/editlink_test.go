package tui

// T-tui-editlink (0095): enter/e on a links row opens the link modal —
// the same form `a` opens, prefilled with the entry's target and source
// — instead of a bare text input on the source alone. The commit rides
// the same applyEdit seam as every field edit (validation, splice,
// undo, the cursor following a rename); a refused commit keeps the
// modal open with the error inside it.

import (
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/stretchr/testify/require"
)

const linkFixture = `id = "demo"
description = "the demo module"

[dotfiles]
"~/.bashrc" = { source = ".bashrc", mode = "symlink" }
"~/.vimrc" = { source = ".vimrc", mode = "copy" }
"~/.zshrc" = { line = "export Z=1" }
`

// linkShell loads the links fixture and focuses the workspace.
func linkShell(t *testing.T) (map[string]string, string, *Compositor) {
	t.Helper()
	subs, c := wsShell(t, map[string]string{"modules/demo/module.toml": linkFixture})
	dir := c.ws.activeDir()
	return subs, dir, cpress(c, "tab")
}

// backspaces sends n backspace keys — clearing a prefilled field from
// its end, the way a terminal user does. (keyPress has no "backspace"
// case; a real terminal delivers KeyBackspace.)
func backspaces(c *Compositor, n int) *Compositor {
	for i := 0; i < n; i++ {
		c, _ = cstep(c, tea.KeyPressMsg{Code: tea.KeyBackspace})
	}
	return c
}

func TestEditLink_enterOpensTheLinkModal(t *testing.T) {
	_, _, c := linkShell(t)
	wsToKey(t, c, "~/.bashrc", "links")
	c = cpress(c, "enter")
	f := addFormTop(t, c)
	require.Nil(t, c.ws.editing, "no inline text input behind the modal")
	view := f.view(100, 30)
	require.Contains(t, view, "edit link · demo")
	require.Contains(t, view, "target")
	require.Contains(t, view, "source")
	require.Contains(t, view, "~/.bashrc", "the target prefills")
	require.Contains(t, view, ".bashrc", "the source prefills")
	require.Contains(t, view, "enter commits", "the footer's verb is an edit's, not an add's")
}

func TestEditLink_commitsSourceKeepsTargetAndMode(t *testing.T) {
	_, dir, c := linkShell(t)
	wsToKey(t, c, "~/.bashrc", "links")
	c = cpress(c, "enter")
	c = cpress(c, "down") // the source row
	c = backspaces(c, 7)  // ".bashrc"
	c = typeText(c, "bashrc")
	c = wsPress(t, c, "enter")
	require.Empty(t, c.modals, "a successful commit closes the modal")
	require.Nil(t, c.ws.editing)
	d := c.wsDraftFor(dir).cfg.Dotfiles["~/.bashrc"]
	require.Equal(t, "bashrc", d.Source)
	require.Equal(t, "symlink", d.Mode, "the mode survives an edit")
	require.Contains(t, c.ws.placeholderOrBody(), "~/.bashrc ← bashrc (symlink)")
}

func TestEditLink_renamesTarget(t *testing.T) {
	_, dir, c := linkShell(t)
	wsToKey(t, c, "~/.bashrc", "links")
	c = cpress(c, "enter")
	c = backspaces(c, 9) // "~/.bashrc"
	c = typeText(c, "~/.bash_profile")
	c = wsPress(t, c, "enter")
	require.Empty(t, c.modals)
	cfg := c.wsDraftFor(dir).cfg
	_, old := cfg.Dotfiles["~/.bashrc"]
	require.False(t, old, "the old target is gone")
	d := cfg.Dotfiles["~/.bash_profile"]
	require.Equal(t, ".bashrc", d.Source)
	require.Equal(t, "symlink", d.Mode, "the mode rides the rename")
	require.Contains(t, c.ws.placeholderOrBody(), "~/.bash_profile ← .bashrc (symlink)")
	require.True(t, wsAtKey(c, "~/.bash_profile", []string{"links"}), "the cursor follows the renamed row")
}

func TestEditLink_renameToExistingRefuses(t *testing.T) {
	_, dir, c := linkShell(t)
	wsToKey(t, c, "~/.bashrc", "links")
	c = cpress(c, "enter")
	c = backspaces(c, 9) // "~/.bashrc"
	c = typeText(c, "~/.vimrc")
	c = wsPress(t, c, "enter")
	f := addFormTop(t, c)
	require.False(t, f.finished(), "a refused commit keeps the modal open")
	require.Contains(t, f.view(100, 30), "already exists")
	require.Nil(t, c.wsDraftFor(dir), "a refused commit stages nothing")
}

func TestEditLink_emptyFieldRefuses(t *testing.T) {
	_, dir, c := linkShell(t)
	wsToKey(t, c, "~/.bashrc", "links")
	c = cpress(c, "enter")
	c = cpress(c, "down")
	c = backspaces(c, 7) // the source clears
	c = wsPress(t, c, "enter")
	f := addFormTop(t, c)
	require.False(t, f.finished(), "an empty source refuses")
	require.Contains(t, f.view(100, 30), "target source")
	require.Nil(t, c.wsDraftFor(dir))
}

func TestEditLink_escStagesNothing(t *testing.T) {
	_, dir, c := linkShell(t)
	wsToKey(t, c, "~/.bashrc", "links")
	c = cpress(c, "e") // e is enter's twin binding
	require.NotEmpty(t, c.modals)
	c = cpress(c, "down")
	c = typeText(c, "x")
	c = cpress(c, "esc")
	require.Empty(t, c.modals, "esc pops the modal")
	require.Nil(t, c.wsDraftFor(dir), "esc stages nothing")
}

func TestEditLink_writesLineStillEditsInline(t *testing.T) {
	_, _, c := linkShell(t)
	wsToKey(t, c, "~/.zshrc", "writes")
	c = cpress(c, "enter")
	require.Empty(t, c.modals, "no modal for a line write")
	require.NotNil(t, c.ws.editing, "a line write keeps the inline edit")
	require.Equal(t, "export Z=1", c.ws.editing.inputString())
}
