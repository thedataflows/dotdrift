package tui

// T-tui-addform: `a` opens a labeled form modal instead of the grammar
// input. The old inline add input rendered nowhere (the row renderer
// gated it behind !editing.add), so users typed blind. The form names
// the entry and its destination, reuses the dialog row primitives, and
// commits through the same applyEdit pipeline; a refused commit keeps
// the form open with the error inside it.

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/thedataflows/dotdrift/internal/profile"
)

// addFormTop returns the top modal as the add form.
func addFormTop(t *testing.T, c *Compositor) *addForm {
	t.Helper()
	require.NotEmpty(t, c.modals, "a form modal is open")
	f, ok := c.modals[len(c.modals)-1].(*addForm)
	require.True(t, ok, "the top modal is the add form, not %T", c.modals[len(c.modals)-1])
	return f
}

func TestAddForm_opensWithLabeledRows(t *testing.T) {
	_, _, c := editShell(t)
	wsToKey(t, c, "present:neovim")
	c = cpress(c, "a")
	f := addFormTop(t, c)
	require.Nil(t, c.ws.editing, "no invisible inline input behind the form")
	view := f.view(100, 30)
	require.Contains(t, view, "add package · demo")
	require.Contains(t, view, "name")
	require.Contains(t, view, "state")
}

func TestAddForm_packagesCommit(t *testing.T) {
	_, dir, c := editShell(t)
	wsToKey(t, c, "present:neovim")
	c = cpress(c, "a")
	c = typeText(c, "fd") // typing lands in the focused name row
	c = wsPress(t, c, "enter")
	require.Empty(t, c.modals, "a successful commit closes the form")
	require.Nil(t, c.ws.editing)
	require.Contains(t, c.ws.placeholderOrBody(), "+ fd", "the committed row joins the table")
	require.Equal(t, []string{"neovim", "ripgrep", "fd"}, c.wsDraftFor(dir).cfg.Packages.Present)
}

func TestAddForm_packagesAbsent(t *testing.T) {
	_, dir, c := editShell(t)
	wsToKey(t, c, "present:neovim")
	c = cpress(c, "a")
	c = typeText(c, "fd")
	c = cpress(c, "down") // state row
	c = cpress(c, "right")
	c = wsPress(t, c, "enter")
	require.Equal(t, []string{"fd"}, c.wsDraftFor(dir).cfg.Packages.Absent)
	require.Contains(t, c.ws.placeholderOrBody(), "− fd")
}

func TestAddForm_emptyNameRefusesInPlace(t *testing.T) {
	_, dir, c := editShell(t)
	wsToKey(t, c, "present:neovim")
	c = cpress(c, "a")
	c = wsPress(t, c, "enter") // no name typed
	f := addFormTop(t, c)
	require.False(t, f.finished(), "a refused add keeps the form open")
	require.Contains(t, f.view(100, 30), "must not be empty")
	require.Nil(t, c.wsDraftFor(dir), "a refused add stages nothing")
}

func TestAddForm_escCancels(t *testing.T) {
	_, dir, c := editShell(t)
	wsToKey(t, c, "present:neovim")
	c = cpress(c, "a")
	c = typeText(c, "fd")
	c = cpress(c, "esc")
	require.Empty(t, c.modals, "esc pops the form")
	require.Nil(t, c.wsDraftFor(dir), "esc stages nothing")
}

func TestAddForm_toolsCommit(t *testing.T) {
	_, dir, c := editShell(t)
	wsToKey(t, c, "node")
	c = cpress(c, "a")
	c = typeText(c, "go")
	c = cpress(c, "down")
	c = typeText(c, "1.22")
	c = wsPress(t, c, "enter")
	require.Equal(t, "1.22", c.wsDraftFor(dir).cfg.Tools["go"])
	require.Contains(t, c.ws.placeholderOrBody(), "go = 1.22")
}

func TestAddForm_hooksPhasePost(t *testing.T) {
	_, dir, c := editShell(t)
	wsToHeader(t, c, "hooks") // an empty section's header is the way in
	c = cpress(c, "a")
	f := addFormTop(t, c)
	require.Contains(t, f.view(100, 30), "add hook · demo")
	c = typeText(c, "echo hi")
	c = cpress(c, "down") // phase row
	c = cpress(c, "right")
	c = wsPress(t, c, "enter")
	require.Equal(t, []profile.HookCommand{{Command: "echo hi"}}, c.wsDraftFor(dir).cfg.Hooks.Post)
	require.Contains(t, c.ws.placeholderOrBody(), "post: echo hi")
}

func TestAddForm_linksCommit(t *testing.T) {
	_, dir, c := editShell(t)
	wsToHeader(t, c, "links")
	c = cpress(c, "a")
	c = typeText(c, "~/.fd")
	c = cpress(c, "down")
	c = typeText(c, "fd")
	c = wsPress(t, c, "enter")
	d := c.wsDraftFor(dir).cfg.Dotfiles["~/.fd"]
	require.Equal(t, "fd", d.Source)
	require.Equal(t, "symlink", d.Mode, "an add creates a symlink entry")
}

func TestAddForm_smbWhatRelabelsValue(t *testing.T) {
	_, c := entriesShell(t)
	wsToHeader(t, c, "smb")
	c = cpress(c, "a")
	f := addFormTop(t, c)
	view := f.view(100, 30)
	require.Contains(t, view, "what")
	require.Contains(t, view, "share name", "the default what is a share")
	cpress(c, "right") // group
	cpress(c, "right") // users
	view = f.view(100, 30)
	require.Contains(t, view, "value", "a scalar what takes a plain value")
}

func TestAddForm_writesLineCommit(t *testing.T) {
	_, dir, c := editShell(t)
	wsToHeader(t, c, "writes")
	c = cpress(c, "a")
	f := addFormTop(t, c)
	view := f.view(100, 30)
	require.Contains(t, view, "add write · demo")
	require.Contains(t, view, "kind")
	require.Contains(t, view, "line text", "the default kind is a line write")
	c = cpress(c, "down")
	c = typeText(c, "~/.zshrc")
	c = cpress(c, "down")
	c = typeText(c, "export PATH=added")
	c = wsPress(t, c, "enter")
	require.Empty(t, c.modals)
	d := c.wsDraftFor(dir).cfg.Dotfiles["~/.zshrc"]
	require.Equal(t, "export PATH=added", d.Line, "the line lands — spaces and all")
	require.Equal(t, "", d.Mode, "a line entry carries no mode")
	require.Contains(t, c.ws.placeholderOrBody(), "~/.zshrc (edit: line)", "the writes row renders")
}

func TestAddForm_writesLinkCommit(t *testing.T) {
	_, dir, c := editShell(t)
	wsToHeader(t, c, "writes")
	c = cpress(c, "a")
	c = cpress(c, "right") // kind: line → link
	f := addFormTop(t, c)
	require.Contains(t, f.view(100, 30), "source", "a link kind takes a source")
	c = cpress(c, "down")
	c = typeText(c, "~/.b")
	c = cpress(c, "down")
	c = typeText(c, "b")
	c = wsPress(t, c, "enter")
	d := c.wsDraftFor(dir).cfg.Dotfiles["~/.b"]
	require.Equal(t, "b", d.Source)
	require.Equal(t, "symlink", d.Mode, "the link kind creates a symlink entry")
}

func TestAddForm_writesEmptyRefuses(t *testing.T) {
	_, dir, c := editShell(t)
	wsToHeader(t, c, "writes")
	c = cpress(c, "a")
	c = wsPress(t, c, "enter")
	f := addFormTop(t, c)
	require.False(t, f.finished(), "a refused add keeps the form open")
	require.Contains(t, f.view(100, 30), "target")
	require.Nil(t, c.wsDraftFor(dir), "a refused add stages nothing")
}
