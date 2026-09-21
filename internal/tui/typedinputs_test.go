package tui

// T-tui-typed-inputs (0094): the compositor wiring for typed inputs.
// A row's edit kind decides what enter opens: the inline single-line
// input (default), the multi-line editor (writes blocks, hooks), or
// the file picker (path fields). Form fields marked browseable open
// the picker with ctrl+o — enter already means "commit the form"
// there. Every commit rides the existing seams (commitFieldAt →
// validateField → applyEdit → splice/ledger/undo), so a picker or
// editor commit is exactly the pipeline a typed commit runs.

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/stretchr/testify/require"
)

const typedFixture = `id = "demo"
description = "the demo module"

[dotfiles]
"~/.bashrc" = { line = "export EDITOR=nvim" }
"~/.gitconfig" = { source = "gitconfig", mode = "symlink" }
"~/.profile" = { block = "export PATH=$PATH:~/bin" }

[hooks]
pre = ["echo pre"]

[mounts.data]
source = "/dev/sda1"
destination = "/mnt/data"
type = "ext4"
`

// typedShell loads the typed-inputs fixture and focuses the workspace.
func typedShell(t *testing.T) *Compositor {
	t.Helper()
	_, c := wsShell(t, map[string]string{"modules/demo/module.toml": typedFixture})
	return cpress(c, "tab")
}

// topModal asserts the modal stack's top type and returns it.
func topModal[T modal](t *testing.T, c *Compositor) T {
	t.Helper()
	require.NotEmpty(t, c.modals)
	m, ok := c.modals[len(c.modals)-1].(T)
	require.True(t, ok, "the top modal is %T", *new(T))
	return m
}

// --- multi-line rows ---

func TestTyped_blockRowOpensMultilineSeeded(t *testing.T) {
	c := typedShell(t)
	c = cursorTo(t, c, "edit: block")
	c = cpress(c, "enter")
	ml := topModal[*multiline](t, c)
	require.Equal(t, "export PATH=$PATH:~/bin", ml.value(), "the editor seeds from the block's content")
	require.Nil(t, c.ws.editing, "a block never opens the inline single-line input")
}

func TestTyped_blockCommitSplicesDraft(t *testing.T) {
	c := typedShell(t)
	dir := c.ws.activeDir()
	c = cursorTo(t, c, "edit: block")
	c = cpress(c, "enter")
	c = cpress(c, "enter") // a newline at the content's end
	c = typeText(c, "# tail")
	c = cpress(c, "ctrl+enter")
	require.Empty(t, c.modals, "a successful commit closes the editor")
	d := c.wsDraftFor(dir)
	require.NotNil(t, d, "the commit forks the draft")
	require.Equal(t, "export PATH=$PATH:~/bin\n# tail", d.cfg.Dotfiles["~/.profile"].Block)
}

func TestTyped_blockEmptyCommitRefused(t *testing.T) {
	c := typedShell(t)
	dir := c.ws.activeDir()
	c = cursorTo(t, c, "edit: block")
	c = cpress(c, "enter")
	for range len("export PATH=$PATH:~/bin") {
		c = cpress(c, "backspace")
	}
	c = cpress(c, "ctrl+enter")
	ml := topModal[*multiline](t, c)
	require.NotEmpty(t, ml.err, "emptying a block names the refusal inside the editor")
	require.Nil(t, c.wsDraftFor(dir), "a refused commit stages nothing (d removes the entry instead)")
}

func TestTyped_hookRowOpensMultiline(t *testing.T) {
	c := typedShell(t)
	dir := c.ws.activeDir()
	c = cursorTo(t, c, "pre: echo pre")
	c = cpress(c, "enter")
	ml := topModal[*multiline](t, c)
	require.Equal(t, "echo pre", ml.value(), "the hook edits its full command")
	c = cpress(c, "enter")
	c = typeText(c, "second")
	c = cpress(c, "ctrl+enter")
	require.Empty(t, c.modals)
	d := c.wsDraftFor(dir)
	require.NotNil(t, d)
	require.Equal(t, "echo pre\nsecond", d.cfg.Hooks.Pre[0].Command, "a hook commits many lines")
}

func TestTyped_writesLineRowKeepsInlineEdit(t *testing.T) {
	c := typedShell(t)
	c = cursorTo(t, c, "edit: line")
	c = cpress(c, "enter")
	require.NotNil(t, c.ws.editing, "a line write keeps the inline single-line input")
	require.Empty(t, c.modals)
}

func TestTyped_multilineQuestionMarkTypes(t *testing.T) {
	c := typedShell(t)
	c = cursorTo(t, c, "edit: block")
	c = cpress(c, "enter")
	c, _ = cstep(c, tea.KeyPressMsg{Code: '?', Text: "?"})
	ml := topModal[*multiline](t, c)
	require.Contains(t, ml.value(), "?", "? is editor content, not the help gesture")
}

// --- path rows ---

func TestTyped_smbPathRowOpensPickerAndCommits(t *testing.T) {
	share := filepath.Join(t.TempDir(), "media")
	require.NoError(t, os.MkdirAll(filepath.Join(share, "sub"), 0o755))
	fixture := fmt.Sprintf("id = \"demo\"\n\n[smb.shares.media]\npath = %q\n", share)
	_, c := wsShell(t, map[string]string{"modules/demo/module.toml": fixture})
	c = cpress(c, "tab")
	dir := c.ws.activeDir()

	c = cursorTo(t, c, "path ")
	c = cpress(c, "enter")
	p := topModal[*filePicker](t, c)
	require.Equal(t, kindDirs, p.mode, "a share path picks directories")
	require.Equal(t, filepath.Dir(share), p.cwd, "the picker opens on the field's current value (0096)")
	require.Equal(t, filepath.Base(share), p.visible()[p.sel].name, "with the value's own entry selected")

	c = cpress(c, "right") // into media
	c = cpress(c, "right") // into sub
	c = cpress(c, "ctrl+enter")
	require.Empty(t, c.modals, "a pick closes the picker")
	d := c.wsDraftFor(dir)
	require.NotNil(t, d)
	require.Equal(t, filepath.Join(share, "sub"), d.cfg.Smb.Shares["media"].Path,
		"the pick commits through the field pipeline (validate, splice, ledger)")
}

func TestTyped_mountsPathRowsOpenPicker(t *testing.T) {
	c := typedShell(t)
	c = cursorTo(t, c, "destination")
	c = cpress(c, "enter")
	require.Equal(t, kindDirs, topModal[*filePicker](t, c).mode, "a mount destination picks directories")
	c = cpress(c, "esc")
	c = cursorTo(t, c, "source")
	c = cpress(c, "enter")
	require.Equal(t, kindEither, topModal[*filePicker](t, c).mode, "a mount source picks files or directories")
}

func TestTyped_pickerEscCommitsNothing(t *testing.T) {
	c := typedShell(t)
	dir := c.ws.activeDir()
	c = cursorTo(t, c, "destination")
	c = cpress(c, "enter")
	c = cpress(c, "esc")
	require.Empty(t, c.modals, "esc closes the picker")
	require.Nil(t, c.wsDraftFor(dir), "esc commits nothing")
}

func TestTyped_pickerFilterEscClearsNotPops(t *testing.T) {
	c := typedShell(t)
	c = cursorTo(t, c, "destination")
	c = cpress(c, "enter")
	c = cpress(c, "/")
	c = typeText(c, "x")
	c = cpress(c, "esc")
	p := topModal[*filePicker](t, c)
	require.Empty(t, string(p.filter), "esc inside the filter clears it…")
	c = cpress(c, "esc")
	require.Empty(t, c.modals, "…and only the next esc closes the picker")
}

func TestTyped_pickerHelpRows(t *testing.T) {
	c := typedShell(t)
	c = cursorTo(t, c, "destination")
	c = cpress(c, "enter")
	c = cpress(c, "?") // browsing owns no text input: ? opens help
	h := topModal[*helpModel](t, c)
	var keys []string
	for _, r := range h.rows {
		keys = append(keys, r.keys)
	}
	require.Contains(t, keys, "/", "the picker's help names its filter")
}

// --- form fields: ctrl+o browses ---

func TestTyped_addFormBrowseCommitsIntoField(t *testing.T) {
	picked := filepath.Join(t.TempDir(), "picked.conf")
	require.NoError(t, os.WriteFile(picked, []byte("x"), 0o644))
	c := typedShell(t)
	c = cursorTo(t, c, "data") // the mounts container
	c = cpress(c, "a")
	f := topModal[*addForm](t, c)
	c = cpress(c, "down") // the value row
	c = cpress(c, "ctrl+o")
	require.Equal(t, kindEither, topModal[*filePicker](t, c).mode, "ctrl+o on a path field browses (source: files or dirs)")
	c = cpress(c, "ctrl+l")
	c = cpress(c, "ctrl+u")
	c = typeText(c, picked)
	c = cpress(c, "enter")
	f = topModal[*addForm](t, c)
	require.Equal(t, picked, f.rows[1].field.String(), "the pick fills the form field; the form stays open")
}

func TestTyped_addFormBrowseFollowsFieldChoice(t *testing.T) {
	c := typedShell(t)
	c = cursorTo(t, c, "data")
	c = cpress(c, "a")
	c = cpress(c, "right") // field choice: source → destination
	c = cpress(c, "down")
	c = cpress(c, "ctrl+o")
	require.Equal(t, kindDirs, topModal[*filePicker](t, c).mode, "the browse mode follows the chosen field (destination: dirs)")
}

func TestTyped_linkFormsSourceBrowse(t *testing.T) {
	c := typedShell(t)
	c = cursorTo(t, c, "←")
	c = cpress(c, "a") // add link form
	c = cpress(c, "down")
	c = cpress(c, "ctrl+o")
	require.Equal(t, kindEither, topModal[*filePicker](t, c).mode, "the add form's source browses")
	c = cpress(c, "esc")
	c = cpress(c, "esc")

	c = cursorTo(t, c, "←")
	c = cpress(c, "enter") // the 0095 edit-link modal
	topModal[*addForm](t, c)
	c = cpress(c, "down")
	c = cpress(c, "ctrl+o")
	require.Equal(t, kindEither, topModal[*filePicker](t, c).mode, "the edit modal's source browses too")
}

func TestTyped_writesAddTargetBrowse(t *testing.T) {
	c := typedShell(t)
	c = cursorTo(t, c, "edit: line")
	c = cpress(c, "a")
	c = cpress(c, "down") // the target row
	c = cpress(c, "ctrl+o")
	require.Equal(t, kindFiles, topModal[*filePicker](t, c).mode, "a writes target browses files")
}

func TestTyped_onboardPathsCtrlO(t *testing.T) {
	adopt := filepath.Join(t.TempDir(), "adopt")
	require.NoError(t, os.MkdirAll(adopt, 0o755))
	_, c := wsShell(t, map[string]string{"modules/demo/module.toml": "id = \"demo\"\n"})
	withWrites(c)
	c.openOnboardHere()
	d := onboardModalOf(t, c)
	d.HandleKey("down") // the paths row
	c = cpress(c, "ctrl+o")
	require.Equal(t, kindEither, topModal[*filePicker](t, c).mode, "the onboard paths field browses")
	c = cpress(c, "ctrl+l")
	c = cpress(c, "ctrl+u")
	c = typeText(c, adopt)
	c = cpress(c, "enter")      // a directory navigates…
	c = cpress(c, "ctrl+enter") // …and ctrl+enter picks it
	onboardModalOf(t, c)
	require.Equal(t, adopt, d.rows[1].field.String(), "the pick lands in the paths field")
}
