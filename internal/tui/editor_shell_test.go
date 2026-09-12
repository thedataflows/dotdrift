package tui

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/thedataflows/dotdrift/internal/facts"
	"github.com/thedataflows/dotdrift/internal/profile"
	"github.com/thedataflows/dotdrift/internal/service"
	"github.com/thedataflows/dotdrift/internal/tui/editor"
)

// Editors open only from the raw overlay-stack node (0065-D2): `e` on an
// origin view opens the editor frame bound to that exact module.toml. The
// shell routes keys to the frame, mirrors the frame's dirty state onto the
// view stack (the quit-confirm input), and pops when the frame asks.

// editorFixture is a real temp profile with two distinct modules — the
// save pipeline's resolve checks reject treeFixture's duplicate ids.
func editorFixture(t *testing.T) *service.ModulesRead {
	t.Helper()
	dir := t.TempDir()
	write := func(rel, id string) {
		modDir := filepath.Join(dir, rel)
		require.NoError(t, os.MkdirAll(modDir, 0o755))
		require.NoError(t, os.WriteFile(filepath.Join(modDir, "module.toml"), []byte(
			"id = \""+id+"\"\napp = \""+id+"\"\n"), 0o644))
	}
	write(filepath.Join("modules", "shell"), "shell")
	write(filepath.Join("modules", "editor"), "editor")
	return &service.ModulesRead{
		Facts: &facts.Facts{Hostname: "myhost", Username: "cri", OS: "linux"},
		Profile: &profile.Profile{
			Root: dir,
			Modules: []profile.Module{
				{ID: "editor", App: "editor", Path: filepath.Join(dir, "modules", "editor")},
				{ID: "shell", App: "shell", Path: filepath.Join(dir, "modules", "shell")},
			},
		},
	}
}

func editorShell(t *testing.T) *Shell {
	t.Helper()
	area := &fakeReads{read: editorFixture(t)}
	root := area.read.Profile.Root
	m := New(area, Options{
		ProfilePath: root,
		ConfigFor: func(*facts.Facts) service.ConfigEditor {
			return service.NewConfigArea(root, service.ConfigDeps{
				Facts: area.read.Facts,
			})
		},
	})
	cmd := m.Init()
	if cmd != nil {
		cmd()
	}
	m.setSize(120, 40)
	r, err := area.Modules(m.opts.ProfilePath, nil)
	require.NoError(t, err)
	m.Update(modulesLoadedMsg{read: r})
	return m
}

// selectOrigin walks the cursor to shell's base origin and opens its view.
func selectOrigin(t *testing.T, m *Shell) treeItem {
	t.Helper()
	origins := m.moduleOriginItems("shell")
	require.NotEmpty(t, origins)
	m.walkTo(origins[0])
	m.syncSelection()
	return m.stack.top().item
}

// editorFrame is the open editor frame of the top view.
func (m *Shell) editorFrame() *editor.Frame {
	return m.frames[m.stack.top().id.key]
}

func TestShell_editorOpensFromRawOrigin(t *testing.T) {
	m := editorShell(t)
	selectOrigin(t, m)
	top := m.stack.top()
	require.Equal(t, viewOrigin, top.id.kind)
	originDir := top.item.dir

	m = press(m, "e")
	top = m.stack.top()
	require.Equal(t, viewEditor, top.id.kind, "e on a raw origin opens the editor")
	require.Equal(t, originDir, top.id.key, "the view key is the module dir; the chrome names the file")
	require.Contains(t, top.content, "EDIT")
	require.Contains(t, top.content, "module.toml", "the chrome names the exact file")

	// The editor is a stack view like any other: esc pops (clean draft).
	m = press(m, "esc")
	require.Equal(t, viewOrigin, m.stack.top().id.kind)
}

func TestShell_editorKeysRouteToFrame(t *testing.T) {
	m := editorShell(t)
	selectOrigin(t, m)
	m = press(m, "e")
	m.focus = focusMain

	// Section keys reach the frame.
	m = press(m, "]")
	require.Equal(t, "packages", m.editorFrame().Current())
	m = press(m, "[")

	// The shell's globals stay the shell's.
	m = press(m, "tab")
	require.Equal(t, focusTree, m.focus, "tab stays the pane switch")
	m = press(m, "tab")
	require.Equal(t, focusMain, m.focus)
	m = press(m, "?")
	require.True(t, m.help.ShowAll, "? stays the help toggle")
}

func TestShell_editorDirtyMirrorsToStack(t *testing.T) {
	m := editorShell(t)
	selectOrigin(t, m)
	m = press(m, "e")
	m.focus = focusMain
	frame := m.editorFrame()

	// Add a package row through raw keys: ] to packages, n, name, enter.
	m = press(m, "]")
	m = press(m, "n")
	for _, k := range []string{"b", "a", "t"} {
		m = press(m, k)
	}
	m = press(m, "enter")

	require.True(t, frame.Dirty())
	require.True(t, m.stack.top().dirty, "the view stack mirrors the draft's dirtiness")
	require.True(t, m.stack.dirtyAnywhere(), "the quit-confirm input sees it")
	require.Contains(t, m.View().Content, "unsaved", "the header carries the dirty mark")

	// esc shows the frame's dirty-confirm, not a pop.
	m = press(m, "esc")
	require.Equal(t, viewEditor, m.stack.top().id.kind)
	require.True(t, frame.Confirming())

	// d discards and pops; the draft is gone.
	m = press(m, "d")
	require.Equal(t, viewOrigin, m.stack.top().id.kind)
}

func TestShell_editorSavePersists(t *testing.T) {
	m := editorShell(t)
	root := m.read.Profile.Root
	selectOrigin(t, m)
	m = press(m, "e")
	m.focus = focusMain

	// Edit the description: keys section, key 3, type, enter, save.
	m = press(m, "3")
	for _, k := range []string{"m", "y"} {
		m = press(m, k)
	}
	m = press(m, "enter")
	m = press(m, "s")

	frame := m.editorFrame()
	require.Equal(t, "saved", frame.Status())
	raw, err := os.ReadFile(filepath.Join(root, "modules", "shell", "module.toml"))
	require.NoError(t, err)
	require.Contains(t, string(raw), "description = \"my\"", "the save pipeline wrote the file")

	// The draft rebased: esc leaves without a confirm.
	m = press(m, "esc")
	require.Equal(t, viewOrigin, m.stack.top().id.kind)
}
