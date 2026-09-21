package tui

// T-tui-keymap: one binding table as data — the dispatcher, the footer
// hints, and the contextual ? help all render from it, so docs cannot
// drift from behavior. shift is the dangerous version (p/P, d/D); esc
// has one meaning; no undo (drafts + confirms are the safety net).

import (
	"strings"
	"testing"

	"charm.land/bubbles/v2/key"
	tea "charm.land/bubbletea/v2"
	"github.com/stretchr/testify/require"
	"github.com/thedataflows/dotdrift/internal/facts"
	"github.com/thedataflows/dotdrift/internal/service"
)

func TestKeys_table(t *testing.T) {
	_, c := wsShell(t, map[string]string{
		"modules/demo/module.toml":           editFixture,
		"users/cri/modules/demo/module.toml": "id = \"demo\"\n",
		"modules/other/module.toml":          "id = \"other\"\n",
	})
	c.applyFor = func(*facts.Facts) ApplyLauncher {
		return &fakeLauncher{previews: nil, run: closedRun(&service.SessionResult{Outcome: service.OutcomeCompleted})}
	}
	c.writesFor = func(*facts.Facts) Writes { return newFakeWrites() }
	c.pumpNoBlock = true

	// nav movement
	require.Equal(t, 0, c.nav.cursor)
	c = cpress(c, "j")
	require.Equal(t, 1, c.nav.cursor, "j moves down")
	c = cpress(c, "k")
	require.Equal(t, 0, c.nav.cursor, "k moves up")
	c = cpress(c, "down")
	require.Equal(t, 1, c.nav.cursor)
	c = cpress(c, "up")
	require.Equal(t, 0, c.nav.cursor)

	// collapse/expand
	c = cpress(c, "l")
	require.True(t, c.nav.expanded["demo"], "l expands")
	c = cpress(c, "h")
	require.False(t, c.nav.expanded["demo"], "h collapses")

	// enter on a module row jumps focus to the workspace
	c = wsPress(t, c, "enter")
	require.Equal(t, focusWork, c.focus, "enter opens the module in the workspace")

	// workspace rows
	start := c.ws.cursor
	c = cpress(c, "j")
	require.Greater(t, c.ws.cursor, start, "j walks workspace rows")
	c = wsPress(t, c, "L")
	require.Equal(t, "user", c.ws.tabs[c.ws.active].layer, "L cycles layers")
	c = wsPress(t, c, "L")
	require.Equal(t, "base", c.ws.tabs[c.ws.active].layer, "L wraps")

	// editing
	c = cpress(c, "j") // to app row
	require.Equal(t, "app", c.ws.rows[c.ws.cursor].key)
	c = cpress(c, "e")
	require.NotNil(t, c.ws.editing, "e edits")
	c = cpress(c, "esc")
	require.Nil(t, c.ws.editing, "esc exits the edit")

	// a opens the add form on a table section
	for !c.ws.atSection("packages") {
		c = cpress(c, "j")
	}
	c = cpress(c, "a")
	addFormTop(t, c)
	c = cpress(c, "esc")
	require.Empty(t, c.modals, "esc pops the form")

	// d / D: remove-row and discard-draft confirms
	c = cpress(c, "enter")
	c = typeText(c, "-zz")
	c = wsPress(t, c, "enter") // a committed edit → draft exists
	c = cpress(c, "d")
	require.IsType(t, &confirmModel{}, c.modals[0], "d asks before removing")
	c = cpress(c, "n")
	c = cpress(c, "D")
	require.IsType(t, &confirmModel{}, c.modals[0], "D asks before discarding")
	require.Contains(t, c.View().Content, "discard draft")
	c = cpress(c, "y") // discard for real
	require.Empty(t, c.store)

	// ctrl+s with no draft: a note, not a crash
	c, cmd := cstep(c, keyPress("ctrl+s"))
	require.Nil(t, cmd)
	require.Equal(t, "no changes", c.message)

	// palette, help, manage, writes, plan
	c = cpress(c, "/")
	require.IsType(t, &paletteModel{}, c.modals[0])
	c = cpress(c, "esc")
	c = cpress(c, "?")
	require.IsType(t, &helpModel{}, c.modals[0], "? opens contextual help")
	c = cpress(c, "esc")
	c = cpress(c, "tab") // nav focus
	c = cpress(c, "m")
	require.IsType(t, &dialogModal{}, c.modals[0], "m opens manage as a modal")
	c = cpress(c, "esc")
	c = cpress(c, "n")
	require.IsType(t, &dialogModal{}, c.modals[0], "n opens module creation")
	c = cpress(c, "esc")
	c = cpress(c, "tab") // w lives on the workspace pane
	c = cpress(c, "w")
	require.IsType(t, &writesMenuModel{}, c.modals[0], "w opens the writes menu")
	c = cpress(c, "esc")
	c = wsPress(t, c, "p")
	require.IsType(t, &planModel{}, c.modals[0], "p opens the plan surface")
	require.Empty(t, c.store, "plan never prompts for credentials")
	c = cpress(c, "esc")

	// P starts the apply gate (fake launcher, no privileged steps)
	c = wsPress(t, c, "P")
	require.NotNil(t, c.apply, "P started the run")

	// q quits clean shells
	c.applying, c.apply = false, nil
	_, cmd = cstep(c, keyPress("q"))
	require.NotNil(t, cmd)
	require.IsType(t, tea.QuitMsg{}, mustMsg(cmd))
}

func TestKeys_shiftIsTheDangerousVersion(t *testing.T) {
	_, c := wsShell(t, map[string]string{"modules/demo/module.toml": editFixture})
	c.applyFor = func(*facts.Facts) ApplyLauncher {
		return &fakeLauncher{run: closedRun(&service.SessionResult{Outcome: service.OutcomeCompleted})}
	}
	c.pumpNoBlock = true
	c = cpress(c, "tab")

	// p plans (read-only), P applies (gates).
	c = wsPress(t, c, "p")
	require.IsType(t, &planModel{}, c.modals[0])
	c = cpress(c, "esc")
	c = wsPress(t, c, "P")
	require.NotNil(t, c.apply, "P is the dangerous counterpart")
	c.applying, c.apply = false, nil

	// d removes a row (confirm), D discards the whole draft (confirm).
	for !c.ws.atSection("packages") {
		c = cpress(c, "j")
	}
	c = cpress(c, "d")
	cm := c.modals[0].(*confirmModel)
	require.Contains(t, cm.title, "remove")
	c = cpress(c, "n")
	c = cpress(c, "D")
	require.Empty(t, c.modals, "D without a draft does nothing") // no draft yet
	c = cpress(c, "enter")
	c = typeText(c, "x")
	c = wsPress(t, c, "enter")
	c = cpress(c, "D")
	require.IsType(t, &confirmModel{}, c.modals[0], "D with a draft confirms")
}

func TestHelp_contextual(t *testing.T) {
	_, c := wsShell(t, map[string]string{"modules/demo/module.toml": editFixture})

	// nav focus: nav keys, no edit keys
	c = cpress(c, "?")
	frame := c.View().Content
	require.Contains(t, frame, "expand")
	require.Contains(t, frame, "filter modules")
	require.NotContains(t, frame, "commit", "nav help hides edit-mode keys")
	c = cpress(c, "esc")

	// editing: edit keys, no nav keys
	c = cpress(c, "tab")
	c = cpress(c, "j")
	c = cpress(c, "enter")
	c = cpress(c, "?") // swallowed by edit mode? no — help must still work
	require.IsType(t, &helpModel{}, c.modals[0])
	frame = c.View().Content
	require.Contains(t, frame, "commit")
	require.NotContains(t, frame, "expand")
	c = cpress(c, "esc")
	c = cpress(c, "esc") // exit edit

	// modal open: the modal's keys
	c = cpress(c, "/")
	c = cpress(c, "?")
	frame = c.View().Content
	require.Contains(t, frame, "choose")
	require.Contains(t, frame, "ctrl+n")
}

func TestKeys_modalOverrides(t *testing.T) {
	_, c := wsShell(t, map[string]string{"modules/demo/module.toml": editFixture})
	c = cpress(c, "tab") // the palette lives on the workspace pane (nav / filters)
	c = cpress(c, "/")
	c = cpress(c, "m") // captured by the palette, not manage
	require.Len(t, c.modals, 1)
	require.IsType(t, &paletteModel{}, c.modals[0])
	c = cpress(c, "esc")

	// Elevation captures everything but esc.
	c.sudoCheck = func([]byte) error { return nil }
	c.pumpNoBlock = true
	c.applyFor = func(*facts.Facts) ApplyLauncher {
		return &fakeLauncher{
			previews: []service.StepPreview{{Name: "sys", NeedsTTY: true, Reason: "elevated (sudo)"}},
			run:      closedRun(&service.SessionResult{Outcome: service.OutcomeCompleted}),
		}
	}
	c = wsPress(t, c, "P")
	require.IsType(t, &elevationModel{}, c.modals[0])
	c = cpress(c, "P") // captured — no second apply
	require.Len(t, c.modals, 1)
	require.Nil(t, c.apply, "the elevation swallowed P")
	cpress(c, "esc")
}

func TestMouse_parity(t *testing.T) {
	_, c := wsShell(t, map[string]string{
		"modules/demo/module.toml":  editFixture,
		"modules/plain/module.toml": "id = \"plain\"\napp = \"plain\"\n",
	})

	// Wheel over the nav moves its cursor.
	c = cstepMouse(t, c, tea.MouseWheelMsg{Button: tea.MouseWheelDown, X: 5, Y: 5})
	require.Equal(t, 1, c.nav.cursor, "wheel scrolls the hovered pane")
	c = cstepMouse(t, c, tea.MouseWheelMsg{Button: tea.MouseWheelUp, X: 5, Y: 5})
	require.Equal(t, 0, c.nav.cursor)

	// Double-click a workspace row edits the field (demo is loaded).
	c = clickWorkRow(t, c, 2) // the app row
	c = clickWorkRow(t, c, 2)
	require.NotNil(t, c.ws.editing, "double-click edits the field under the cursor")
	c = cpress(c, "esc")

	// Click a nav row: selects it and focuses the nav (the workspace
	// follows on the identity placeholder — the click's load cmd is
	// dropped by the harness, which is fine here).
	c = cstepMouse(t, c, tea.MouseClickMsg{Button: tea.MouseLeft, X: 5, Y: 4})
	require.Equal(t, focusNav, c.focus, "click focuses the pane")
	require.Equal(t, 1, c.nav.cursor, "click selects the row")
	require.Contains(t, c.ws.placeholderOrBody(), "plain", "the workspace follows the click")
}

func TestKeys_noOrphanedBindings(t *testing.T) {
	// The table is the dispatcher: every entry names a handler, and the
	// handler switch covers every entry (both fail to compile otherwise —
	// this test pins the expected key set as data).
	var keys []string
	for _, b := range keyTable() {
		require.NotEmpty(t, b.help, "every binding has help text")
		keys = append(keys, b.key+":"+b.pane)
	}
	require.ElementsMatch(t, keys, []string{
		"j:nav", "k:nav", "down:nav", "up:nav", "h:nav", "l:nav", "left:nav", "right:nav",
		"enter:nav", "/:nav", "n:nav", "m:nav", "P:both", "p:both", "?:both", "q:both", "tab:both", "o:both",
		"j:work", "k:work", "down:work", "up:work", "enter:work", "e:work", "left:work", "right:work", "a:both",
		"d:work", "D:work", "ctrl+s:work", "ctrl+z:work", "ctrl+shift+z:work", "L:work", "w:work", "/:work",
		"pgup:nav", "pgdown:nav", "home:nav", "end:nav",
		"pgup:work", "pgdown:work", "home:work", "end:work",
	})
}

// cstepMouse feeds a mouse message through Update and settles the load
// the selection change schedules.
func cstepMouse(t *testing.T, c *Compositor, msg tea.Msg) *Compositor {
	t.Helper()
	next, cmd := cstep(c, msg)
	return wsSettle(t, next, cmd)
}

// clickWorkRow clicks the workspace entry row index i (title + border
// offsets accounted for) and returns the stepped compositor.
func clickWorkRow(t *testing.T, c *Compositor, i int) *Compositor {
	t.Helper()
	navW, _, _ := c.layout()
	return cstepMouse(t, c, tea.MouseClickMsg{Button: tea.MouseLeft, X: navW + 3, Y: workRowY(i)})
}

// workRowY is the screen row of workspace entry i (header + border +
// title offsets).
func workRowY(i int) int { return 4 + i }

func TestWritesMenu_cursorRowBar(t *testing.T) {
	// 0075 T-tui-selection: modal menus use the same cursor treatment.
	w := &writesMenuModel{th: newTheme(true)}
	frame := ansiRe.ReplaceAllString(w.view(40, 10), "")
	require.Contains(t, frame, "│ onboard", "the writes menu's cursor row renders the bar")
	require.Contains(t, frame, "  restore", "plain rows keep the two-space lead")
}

// 0078: the footer hints name the pane's primary verbs. enter and a
// were never rendered (the short help took the first three table rows,
// all scrolling), so the two most important keys were undiscoverable.
func TestKeys_footerHintsShowPrimaryActions(t *testing.T) {
	_, _, c := editShell(t)
	joined := func(bindings []key.Binding) string {
		var parts []string
		for _, b := range bindings {
			if h := b.Help(); h.Key != "" {
				parts = append(parts, h.Key+" "+h.Desc)
			}
		}
		return strings.Join(parts, " · ")
	}

	c.focus = focusWork
	work := joined(c.ShortHelp())
	require.Contains(t, work, "enter edit field", "enter is hinted in the workspace")
	require.Contains(t, work, "a add entry / apply detail", "a is hinted in the workspace")
	require.Contains(t, work, "ctrl+s save draft", "save is hinted in the workspace")
	require.NotContains(t, work, "page up", "scroll hints yield the footer to the verbs")

	c.focus = focusNav
	nav := joined(c.ShortHelp())
	require.Contains(t, nav, "enter open in workspace", "enter is hinted in the nav")
	require.NotContains(t, nav, "add entry", "a does nothing from the nav pane, so it is not hinted")
}
