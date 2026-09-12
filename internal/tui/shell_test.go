package tui

import (
	"context"
	"errors"
	"testing"

	"charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/stretchr/testify/require"
	"github.com/thedataflows/dotdrift/internal/drift"
	"github.com/thedataflows/dotdrift/internal/facts"
	"github.com/thedataflows/dotdrift/internal/profile"
	"github.com/thedataflows/dotdrift/internal/resolve"
	"github.com/thedataflows/dotdrift/internal/service"
)

// The shell state machine (T-tui-shell): exactly one focused pane, the
// vim-ish key baseline, chrome (header + status-bar hints), quit
// dirty-confirm, selection-driven views with esc popping back, and the
// async reads loads — all as pure model transitions with golden-free
// content assertions (the read views' rendering has its own goldens).

// fakeReads is the narrow interface, faked. It records the module filter
// the resolved module view asks for.
type fakeReads struct {
	read      *service.ModulesRead
	plan      *service.PlanRead
	planErr   error
	status    *service.StatusRead
	diff      []service.DiffEntry
	planCalls []string
}

func (f *fakeReads) Modules(string, []string) (*service.ModulesRead, error) {
	return f.read, nil
}

func (f *fakeReads) Plan(_ string, modules []string, _ *facts.Facts) (*service.PlanRead, error) {
	f.planCalls = append(f.planCalls, joinModules(modules))
	return f.plan, f.planErr
}

func (f *fakeReads) Status(context.Context, service.StatusOpts) (*service.StatusRead, error) {
	return f.status, nil
}

func (f *fakeReads) Diff(*resolve.Plan, string) ([]service.DiffEntry, error) { return f.diff, nil }

func (f *fakeReads) ModuleConfigAt(string) (*profile.ModuleConfig, error) {
	return &profile.ModuleConfig{}, nil
}

func joinModules(modules []string) string {
	out := ""
	for i, m := range modules {
		if i > 0 {
			out += ","
		}
		out += m
	}
	return out
}

func newShellForTest(t *testing.T) (*Shell, tea.Cmd) {
	t.Helper()
	area := &fakeReads{read: treeFixture(t)}
	m := New(area, Options{
		ProfilePath: "/home/cri/profiles/main",
		ProbesFor:   func(*facts.Facts) drift.Probes { return drift.Probes{} },
	})
	return m, m.Init()
}

// loadShell runs the startup load and sizes the window — the ready shell
// with the tree built and the first module selected.
// loadTree runs the startup load only — the tree is built but the initial
// view's async fill has not run yet.
func loadTree(t *testing.T) *Shell {
	t.Helper()
	m, init := newShellForTest(t)
	require.NotNil(t, init, "Init schedules the startup modules load")
	m, _ = step(m, init())
	m, _ = step(m, tea.WindowSizeMsg{Width: 100, Height: 30})
	return m
}

func loadShell(t *testing.T) *Shell {
	t.Helper()
	m, init := newShellForTest(t)
	require.NotNil(t, init, "Init schedules the startup modules load")
	m, loadCmd := step(m, init())
	if loadCmd != nil {
		m, _ = step(m, loadCmd())
	}
	m, _ = step(m, tea.WindowSizeMsg{Width: 100, Height: 30})
	return m
}

func step(m *Shell, msg tea.Msg) (*Shell, tea.Cmd) {
	next, cmd := m.Update(msg)
	return next.(*Shell), cmd
}

func press(m *Shell, s string) *Shell {
	next, _ := step(m, keyPress(s))
	return next
}

func keyPress(s string) tea.KeyPressMsg {
	switch s {
	case "tab":
		return tea.KeyPressMsg{Code: tea.KeyTab}
	case "shift+tab":
		return tea.KeyPressMsg{Code: tea.KeyTab, Mod: tea.ModShift}
	case "esc":
		return tea.KeyPressMsg{Code: tea.KeyEscape}
	case "enter":
		return tea.KeyPressMsg{Code: tea.KeyEnter}
	case "up":
		return tea.KeyPressMsg{Code: tea.KeyUp}
	case "down":
		return tea.KeyPressMsg{Code: tea.KeyDown}
	case "left":
		return tea.KeyPressMsg{Code: tea.KeyLeft}
	case "right":
		return tea.KeyPressMsg{Code: tea.KeyRight}
	case "ctrl+c":
		return tea.KeyPressMsg{Code: 'c', Mod: tea.ModCtrl}
	default:
		return tea.KeyPressMsg{Code: rune(s[0])}
	}
}

func selectedItemLabel(m *Shell) string {
	it := m.selectedItem()
	return it.label
}

func TestShell_focusMovesWithTab(t *testing.T) {
	m := loadShell(t)
	require.Equal(t, focusTree, m.focus, "the tree starts focused")
	m = press(m, "tab")
	require.Equal(t, focusMain, m.focus)
	m = press(m, "shift+tab")
	require.Equal(t, focusTree, m.focus)
}

func TestShell_oneFocusedPane(t *testing.T) {
	m := loadShell(t)
	m = press(m, "tab")
	// j scrolls the main pane; it must not move the tree selection.
	before := selectedItemLabel(m)
	m = press(m, "j")
	require.Equal(t, before, selectedItemLabel(m), "j scrolls when the main pane holds focus")
	m = press(m, "shift+tab")
	m = press(m, "j")
	require.NotEqual(t, before, selectedItemLabel(m), "j moves the tree when it holds focus")
}

// advanceTo presses j until the named node is selected (bounded),
// returning the async load its selection scheduled, if any.
func advanceTo(t *testing.T, m *Shell, label string) (*Shell, tea.Cmd) {
	t.Helper()
	for i := 0; i < 20; i++ {
		if selectedItemLabel(m) == label {
			return m, nil
		}
		m, cmd := step(m, keyPress("j"))
		if selectedItemLabel(m) == label {
			return m, cmd
		}
	}
	t.Fatalf("never reached %q", label)
	return m, nil
}

func TestShell_selectionOpensViews(t *testing.T) {
	m, init := newShellForTest(t)
	m, loadCmd := step(m, init())
	// The startup selection is the first module (sorted: editor) — its
	// resolved view is active, loading its plan through the reads area.
	require.Equal(t, kindModule, m.selectedItem().kind)
	require.Equal(t, viewModule, m.stack.top().id.kind)
	require.True(t, m.stack.top().loading, "the resolved view starts as a placeholder")
	require.NotNil(t, loadCmd, "a load was scheduled for the module's plan")

	m, _ = step(m, loadCmd())
	require.False(t, m.stack.top().loading, "the load fills the view")
	require.Equal(t, "editor", m.area.(*fakeReads).planCalls[0], "the module view resolves by module filter")

	// Moving the tree selection onto shell opens its own resolved view.
	var shellCmd tea.Cmd
	m, shellCmd = advanceTo(t, m, "shell ·3")
	require.NotNil(t, shellCmd)
	require.Equal(t, viewModule, m.stack.top().id.kind)
	require.Equal(t, "shell", m.stack.top().id.key)
	require.True(t, m.stack.top().loading, "shell's resolved view loads on selection")
	m, _ = step(m, shellCmd())
	require.False(t, m.stack.top().loading)

	// Onto the profile group's generate action.
	m = press(m, "g")
	m = press(m, "G")
	require.Equal(t, kindAction, m.selectedItem().kind)
	require.Equal(t, "GENERATE", m.stack.top().title)
}

func TestShell_escPopsBack(t *testing.T) {
	m := loadShell(t)
	m = press(m, "g")
	m = press(m, "G") // to the profile group's generate action
	require.Equal(t, "GENERATE", m.stack.top().title)

	m = press(m, "esc")
	require.Equal(t, viewModule, m.stack.top().id.kind, "esc pops the view stack")
	require.Equal(t, "editor", selectedItemLabel(m), "esc walks the tree cursor back to the restored view")
}

func TestShell_rawToggle(t *testing.T) {
	m := loadShell(t)
	m, shellCmd := advanceTo(t, m, "shell ·3")
	m, _ = step(m, shellCmd())
	require.False(t, m.stack.top().raw)
	m = press(m, "r")
	require.True(t, m.stack.top().raw, "r shows the raw layer declarations")
	require.Contains(t, m.stack.top().content, "raw declarations")
	require.Equal(t, "shell", m.area.(*fakeReads).planCalls[len(m.area.(*fakeReads).planCalls)-1])
	m = press(m, "esc")
	require.False(t, m.stack.top().raw, "esc returns to the resolved view before popping")
}

func TestKeys_baseline(t *testing.T) {
	m := loadShell(t)

	// j/k and arrows move the tree.
	first := selectedItemLabel(m)
	m = press(m, "j")
	require.NotEqual(t, first, selectedItemLabel(m))
	m = press(m, "k")
	require.Equal(t, first, selectedItemLabel(m))
	m = press(m, "down")
	require.NotEqual(t, first, selectedItemLabel(m))
	m = press(m, "up")
	require.Equal(t, first, selectedItemLabel(m))

	// g/G jump.
	m = press(m, "G")
	require.Equal(t, "generate", selectedItemLabel(m), "G lands on the last tree node")
	m = press(m, "g")
	require.Equal(t, "main", selectedItemLabel(m), "g lands on the tree root (the profile dir)")

	// enter toggles group expansion: closing MODULES hides its modules.
	m = press(m, "down") // onto MODULES
	require.Contains(t, m.View().Content, "editor")
	m = press(m, "enter")
	require.NotContains(t, m.View().Content, "editor", "enter closes the group")
	m = press(m, "enter")
	require.Contains(t, m.View().Content, "editor", "enter reopens the group")

	// ? toggles the full help view.
	require.False(t, m.help.ShowAll)
	m = press(m, "?")
	require.True(t, m.help.ShowAll)
	m = press(m, "?")
	require.False(t, m.help.ShowAll)

	// ctrl+c quits without a dirty prompt.
	_, cmd := step(m, keyPress("ctrl+c"))
	require.NotNil(t, cmd, "ctrl+c quits")
}

func TestShell_quitDirtyConfirm(t *testing.T) {
	m := loadShell(t)

	// A clean shell quits immediately.
	_, cmd := step(m, keyPress("q"))
	require.NotNil(t, cmd, "clean q quits")

	// A dirty shell asks first.
	m2 := loadShell(t)
	top := m2.stack.top()
	top.dirty = true
	m2.stack.setTop(top)
	m2 = press(m2, "q")
	require.True(t, m2.confirm, "dirty q asks")
	require.Contains(t, m2.View().Content, "Really quit", "the confirmation renders")
	m2 = press(m2, "n")
	require.False(t, m2.confirm, "n cancels")
	require.NotNil(t, m2, "n does not quit")
	m2 = press(m2, "q")
	require.True(t, m2.confirm, "q asks again while dirty")
	_, cmd = step(m2, keyPress("y"))
	require.NotNil(t, cmd, "y quits")
}

func TestChrome_headerShowsRootContextDirty(t *testing.T) {
	m := loadShell(t)
	view := m.View().Content
	require.Contains(t, view, "dotdrift tui")
	require.Contains(t, view, "/home/cri/profiles/main", "the header names the profile root")
	require.Contains(t, view, "host myhost", "the header names the host context")
	require.Contains(t, view, "user cri", "the header names the user context")
	require.NotContains(t, view, "●", "a clean shell shows no dirty indicator")

	top := m.stack.top()
	top.dirty = true
	m.stack.setTop(top)
	require.Contains(t, m.View().Content, "●", "the dirty indicator appears")
}

func TestChrome_statusBarHintsFollowFocus(t *testing.T) {
	m := loadShell(t)

	treeHelp := shortHelpKeys(m)
	m = press(m, "tab")
	mainHelp := shortHelpKeys(m)
	require.NotEqual(t, treeHelp, mainHelp, "each focused pane shows its own hints")
	require.Contains(t, mainHelp, "esc", "the main pane hints back/pop")
	require.NotContains(t, treeHelp, "esc", "the tree has no pop")
}

func shortHelpKeys(m *Shell) []string {
	var keys []string
	for _, b := range m.ShortHelp() {
		if !b.Enabled() {
			continue
		}
		keys = append(keys, b.Help().Key)
	}
	return keys
}

func TestShell_backgroundColorSwitch(t *testing.T) {
	m := loadShell(t)
	require.True(t, m.isDark, "dark is the default until bubbletea reports")
	m, _ = step(m, tea.BackgroundColorMsg{Color: lipgloss.Color("#FFFFFF")})
	require.False(t, m.isDark, "the reported background flips the palette")
}

func TestShell_tooSmall(t *testing.T) {
	m := loadShell(t)
	m, _ = step(m, tea.WindowSizeMsg{Width: 40, Height: 10})
	require.Contains(t, m.View().Content, "too small")
}

func TestShell_loadingScreen(t *testing.T) {
	m, _ := newShellForTest(t)
	m, _ = step(m, tea.WindowSizeMsg{Width: 100, Height: 30})
	require.Contains(t, m.View().Content, "loading", "the shell shows a loading screen until the reads land")
}

func TestShell_loadErrorShowsInView(t *testing.T) {
	area := &fakeReads{read: treeFixture(t), planErr: errors.New("resolve exploded")}
	m := New(area, Options{ProfilePath: "/home/cri/profiles/main"})
	m, _ = step(m, tea.WindowSizeMsg{Width: 100, Height: 30})
	m, loadCmd := step(m, mustMsg(m.Init()))
	m, _ = step(m, loadCmd())
	require.False(t, m.stack.top().loading, "an error ends the loading state")
	require.Contains(t, m.stack.top().content, "resolve exploded", "the error surfaces in the view")
}

// mustMsg runs a command that must return a message.
func mustMsg(cmd tea.Cmd) tea.Msg {
	if cmd == nil {
		return nil
	}
	return cmd()
}
