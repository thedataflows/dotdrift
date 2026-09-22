package tui

import (
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/stretchr/testify/require"
)

// T-tui-navfilter (0081): `/` on the nav pane filters the modules list
// in place — no modal. The typed query shows at the pane bottom; esc
// removes the filter. The workspace pane's `/` still opens the palette.

func filterShell(t *testing.T) *Compositor {
	t.Helper()
	_, c := wsShell(t, map[string]string{
		"modules/demo/module.toml":  "id = \"demo\"\n",
		"modules/other/module.toml": "id = \"other\"\n",
		"modules/plain/module.toml": "id = \"plain\"\n",
	})
	return c
}

func moduleLabels(c *Compositor) []string {
	var out []string
	for _, r := range c.nav.rows() {
		if r.layer == "" {
			out = append(out, r.label)
		}
	}
	return out
}

func plainFrame(c *Compositor) string { return ansiRe.ReplaceAllString(c.View().Content, "") }

func TestFilter_slashFiltersListInPlace(t *testing.T) {
	c := filterShell(t)
	c = cpress(c, "/")
	require.Empty(t, c.modals, "no modal: the filter is in place")
	require.True(t, c.nav.filtering, "the nav pane is filtering")

	c = typeText(c, "oth")
	require.Equal(t, []string{"other"}, moduleLabels(c), "typing narrows the list live")
	require.Equal(t, "other", c.nav.selected().moduleID, "the cursor lands on the match")
}

func TestFilter_queryShownAtPaneBottom(t *testing.T) {
	c := filterShell(t)
	c = cpress(c, "/")
	c = typeText(c, "de")
	require.Contains(t, plainFrame(c), "/ de▏", "the query shows while typing")

	c = cpress(c, "enter")
	require.False(t, c.nav.filtering, "enter ends the typing mode")
	require.Contains(t, plainFrame(c), "/ de · esc clears", "the applied filter stays named")
	require.Equal(t, []string{"demo"}, moduleLabels(c), "the filter still applies")
}

func TestFilter_escRemovesFilter(t *testing.T) {
	c := filterShell(t)
	c = cpress(c, "/")
	c = typeText(c, "de")
	c = cpress(c, "enter") // applied, mode off
	c = cpress(c, "esc")   // base esc clears the applied filter
	require.Equal(t, []string{"demo", "other", "plain"}, moduleLabels(c), "esc restores the full list")
	require.Empty(t, c.nav.query)
	require.NotContains(t, plainFrame(c), "/ de", "the query line is gone")

	// esc while typing clears too.
	c = cpress(c, "/")
	c = typeText(c, "ot")
	c = cpress(c, "esc")
	require.False(t, c.nav.filtering)
	require.Equal(t, []string{"demo", "other", "plain"}, moduleLabels(c))
}

func TestFilter_noMatchesNamesTheQuery(t *testing.T) {
	c := filterShell(t)
	c = cpress(c, "/")
	c = typeText(c, "zzz")
	require.Empty(t, moduleLabels(c))
	require.Contains(t, plainFrame(c), "no modules match «zzz»")
	c = cpress(c, "j") // must not panic on the empty list
	require.Equal(t, 0, c.nav.cursor)
}

func TestFilter_arrowsMoveWhileTyping(t *testing.T) {
	c := filterShell(t)
	c = cpress(c, "/")
	c = typeText(c, "e") // demo + other match
	c = cpress(c, "j")
	require.Equal(t, "other", c.nav.selected().moduleID, "j moves within the matches")
	c = cpress(c, "enter")
	require.Equal(t, focusNav, c.focus)
	c = wsPress(t, c, "enter")
	require.Equal(t, focusWork, c.focus, "enter opens the selection as usual")
	require.Equal(t, "other", c.ws.moduleID)
}

func TestFilter_cursorStaysOnMatch(t *testing.T) {
	c := filterShell(t)
	c = cpress(c, "j") // cursor on "other"
	require.Equal(t, "other", c.nav.selected().moduleID)
	c = cpress(c, "/")
	c = typeText(c, "e") // demo and other both match
	require.Equal(t, "other", c.nav.selected().moduleID, "the cursor keeps the module it was on")
}

func TestFilter_workSlashStillOpensPalette(t *testing.T) {
	c := filterShell(t)
	c = cpress(c, "tab")
	c = cpress(c, "/")
	require.Len(t, c.modals, 1, "the workspace pane's / is still the palette")
	require.IsType(t, &paletteModel{}, c.modals[0])
}

func TestFilter_footerHintFollowsPane(t *testing.T) {
	// The footer renders ShortHelp; "/" must name what the focused
	// pane's / actually does.
	slashHelp := func(c *Compositor) string {
		for _, kb := range c.ShortHelp() {
			for _, k := range kb.Keys() {
				if k == "/" {
					return kb.Help().Desc
				}
			}
		}
		return ""
	}
	c := filterShell(t)
	require.Equal(t, "filter modules", slashHelp(c), "nav names the filter")
	c = cpress(c, "tab")
	require.Equal(t, "palette", slashHelp(c), "work keeps the palette")
}

func TestFilter_clickSelectsAndExitsMode(t *testing.T) {
	c := filterShell(t)
	c = cpress(c, "/")
	c = typeText(c, "e")
	c = cstepMouse(t, c, tea.MouseClickMsg{X: 5, Y: 3, Button: tea.MouseLeft})
	require.False(t, c.nav.filtering, "a click commits the typing mode")
	require.Contains(t, plainFrame(c), "/ e · esc clears", "the filter stays applied")
}

// T-tui-navfilter (0101): a query change does not sync the workspace per
// keystroke (that would read a layer file per character); a debounce
// syncs it to the selection once typing settles.

// settle runs the tick cmd a query change returned and feeds the
// resulting message back in, then settles any scheduled layer read.
func settle(t *testing.T, c *Compositor, cmd tea.Cmd) *Compositor {
	t.Helper()
	msg := mustMsg(cmd)
	require.IsType(t, filterSettleMsg{}, msg, "a query change schedules the settle tick")
	c, next := cstep(c, msg)
	return wsSettle(t, c, next)
}

func TestFilter_typingSettlesDetails(t *testing.T) {
	c := filterShell(t)
	require.Equal(t, "demo", c.ws.moduleID)

	c = cpress(c, "/")
	var cmd tea.Cmd
	c, _ = cstep(c, tea.KeyPressMsg{Code: 'o', Text: "o"})
	c, cmd = cstep(c, tea.KeyPressMsg{Code: 't', Text: "t"})
	require.Equal(t, "other", c.nav.selected().moduleID, "the cursor lands on the match")
	require.Equal(t, "demo", c.ws.moduleID, "typing does not sync the workspace per keystroke")

	c = settle(t, c, cmd)
	require.Equal(t, "other", c.ws.moduleID, "the settled filter syncs the workspace to the selection")
}

func TestFilter_backspaceSettlesDetails(t *testing.T) {
	c := filterShell(t)
	c = wsPress(t, c, "j")
	require.Equal(t, "other", c.ws.moduleID)

	c = cpress(c, "/")
	c, _ = cstep(c, tea.KeyPressMsg{Code: 'd', Text: "d"})
	require.Equal(t, "demo", c.nav.selected().moduleID, "the query moves the cursor to demo")
	require.Equal(t, "other", c.ws.moduleID, "the workspace still shows other")
	c, cmd := cstep(c, keyPress("backspace"))
	require.Equal(t, "demo", c.nav.selected().moduleID, "backspace re-broadens, the cursor keeps demo")
	require.Equal(t, "other", c.ws.moduleID, "backspace does not sync per keystroke either")

	c = settle(t, c, cmd)
	require.Equal(t, "demo", c.ws.moduleID, "the settled backspace syncs the workspace")
}

func TestFilter_pasteSettlesDetails(t *testing.T) {
	c := filterShell(t)
	c = cpress(c, "/")
	c, cmd := cstep(c, tea.PasteMsg{Content: "ot"})
	require.Equal(t, "other", c.nav.selected().moduleID)
	require.Equal(t, "demo", c.ws.moduleID, "pasting does not sync per message")

	c = settle(t, c, cmd)
	require.Equal(t, "other", c.ws.moduleID, "the settled paste syncs the workspace")
}

func TestFilter_noMatchSettlesToEmptyWorkspace(t *testing.T) {
	c := filterShell(t)
	c = cpress(c, "/")
	var cmd tea.Cmd
	for _, r := range "zzz" {
		c, cmd = cstep(c, tea.KeyPressMsg{Code: r, Text: string(r)})
	}
	require.Empty(t, moduleLabels(c))

	c = settle(t, c, cmd)
	require.Empty(t, c.ws.moduleID, "an empty match set settles to an empty workspace")
}

func TestFilter_staleSettleTickIsIgnored(t *testing.T) {
	c := filterShell(t)
	c = cpress(c, "/")
	c = typeText(c, "ot")
	require.Equal(t, "demo", c.ws.moduleID)

	c, cmd := cstep(c, filterSettleMsg{seq: c.filterSeq - 1})
	require.Nil(t, cmd, "a superseded tick schedules nothing")
	require.Equal(t, "demo", c.ws.moduleID, "a superseded tick does not sync")
}
