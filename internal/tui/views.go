package tui

// T-tui-views slice 1b (issue 0107): the view row under the header —
// Modules · Status · Plan · Profile, one active at a time. Modules is
// the nav+workspace base screen, unchanged; the Profile body landed in
// slice 1c (profileview.go), the Status and Plan bodies land in later
// slices and render the not-loaded placeholder until then. A write or
// a finished apply marks a read view stale (markReadViewsStale); the
// row title carries the stale word until a refresh clears it.

import (
	"strings"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
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
// word on its title. A refresh clears the mark when the fresh read
// lands. The seq bump orders the mark against in-flight reads: a read
// that started before this mark landed carries older data and must not
// clear it (0108 fix round 1).
func (m *Compositor) markStale(v viewKind) {
	m.stale[v] = true
	m.staleSeq[v]++
}

// markReadViewsStale marks every read view stale: a write (save, manage,
// onboard, generate, restore) or a finished apply outlived their data
// (0107). Modules re-reads itself on writes, so it is never marked.
func (m *Compositor) markReadViewsStale() {
	m.markStale(viewStatus)
	m.markStale(viewPlan)
	m.markStale(viewProfile)
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
		// The Status title names the yank scope its data was read under
		// (0108): a scoped read is visible, and a yank change marks the
		// view stale until a refresh re-reads under the new scope.
		if v == viewStatus && len(m.status.scope) > 0 {
			name += " [" + strings.Join(m.status.scope, ", ") + "]"
		}
		if v == m.view {
			parts = append(parts, m.th.viewTitle.Render(name))
		} else {
			parts = append(parts, m.th.headerContext.Render(name))
		}
	}
	return " " + strings.Join(parts, m.th.headerContext.Render(" · "))
}

// viewBody renders the active non-Modules view: the Profile view's list
// (slice 1c), the Status view's findings list + detail panes (0108
// slice 2c), or a full-width pane with the not-loaded placeholder for
// the views whose bodies land later. The pane content gets the outer
// dimensions minus the border, like the base panes.
func (m *Compositor) viewBody(paneH int) string {
	switch m.view {
	case viewProfile:
		return m.pane(false, m.w-2, paneH, m.profileBody(m.w-4))
	case viewStatus:
		// Findings rows run long (paths + details): an even split, not
		// the Modules view's narrow nav.
		listW := m.w / 2
		left := m.pane(m.focus == focusNav, listW-2, paneH, m.statusList(listW-4, paneH-2))
		right := m.pane(m.focus == focusWork, m.w-listW-2, paneH, m.statusDetail(m.w-listW-4))
		return lipgloss.JoinHorizontal(lipgloss.Top, left, right)
	}
	return m.pane(false, m.w-2, paneH, m.th.loading.Render(" not loaded"))
}

// viewHook is the active view's pane-key handler: a non-Modules view
// receives pane keys through this hook, never through the Modules
// bindings. Nil for views without a body — pane keys are dead on an
// empty view.
func (m *Compositor) viewHook() func(s string) (tea.Cmd, bool) {
	if m.view == viewStatus {
		return m.statusKey
	}
	return nil
}
