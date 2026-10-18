package tui

// T-tui-views slice 1b (issue 0107): the view row under the header —
// Modules · Status · Plan · Profile, one active at a time. Modules is
// the nav+workspace base screen, unchanged; the Status, Plan, and
// Profile bodies land in later slices and render the not-loaded
// placeholder until then. A write or a finished apply marks a read
// view stale (later slices wire the marking); the row title carries
// the stale word until a refresh clears it.

import (
	"strings"

	tea "charm.land/bubbletea/v2"
)

// viewKind names the base views on the view row.
type viewKind int

const (
	viewModules viewKind = iota
	viewStatus
	viewPlan
	viewProfile
)

// viewCount is the number of views on the row: the jump keys are 1-4.
const viewCount = 4

// viewNames are the row titles in row order.
var viewNames = [viewCount]string{"Modules", "Status", "Plan", "Profile"}

// markStale marks a view's data as out of date: the row shows the stale
// word on its title. Later slices call this on writes and finished
// applies, and clear the mark on refresh.
func (m *Compositor) markStale(v viewKind) {
	m.stale[v] = true
}

// viewRow renders the view row: the titles joined by ·, the active view
// accented, a stale view titled with the stale word. The counts slot on
// each title stays empty until slice 5.
func (m *Compositor) viewRow() string {
	parts := make([]string, 0, viewCount)
	for i, name := range viewNames {
		v := viewKind(i)
		if m.stale[v] {
			name += " (stale)"
		}
		if v == m.view {
			parts = append(parts, m.th.viewTitle.Render(name))
		} else {
			parts = append(parts, m.th.headerContext.Render(name))
		}
	}
	return " " + strings.Join(parts, m.th.headerContext.Render(" · "))
}

// viewBody renders the active non-Modules view: a full-width pane with
// the not-loaded placeholder until the view's body lands (later slices).
func (m *Compositor) viewBody(paneH int) string {
	return m.pane(false, m.w-2, paneH, m.th.loading.Render(" not loaded"))
}

// viewHook is the active view's pane-key handler: a non-Modules view
// receives pane keys through this hook, never through the Modules
// bindings. Nil until later slices give the views bodies — pane keys
// are dead on an empty view.
func (m *Compositor) viewHook() func(s string) (tea.Cmd, bool) {
	return nil
}
