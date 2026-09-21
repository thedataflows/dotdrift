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
