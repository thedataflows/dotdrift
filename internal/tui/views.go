package tui

// T-tui-views slice 1b (issue 0107): the view row under the header —
// Modules · Status · Plan · Profile, one active at a time. Modules is
// the nav+workspace base screen, unchanged; the Profile body landed in
// slice 1c (profileview.go), the Status body in slice 2c
// (statusview.go), the Plan body in slice 3 (planview.go). A write or
// a finished apply marks a read view stale (markReadViewsStale); the
// row title carries the stale word until a refresh clears it.

import (
	"strconv"
	"strings"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
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
var viewNames = [viewCount]string{"1. Modules", "2. Status", "3. Plan", "4. Profile"}

// markStale marks a view's data as out of date: the row shows the stale
// dot badge on its tab. A refresh clears the mark when the fresh read
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

// viewRow renders the tab bar (0113): each view is a padded block, the
// active tab filled, an inactive tab flat. A stale view carries an
// amber ● badge after its tab, not the stale word. The counts slot
// (0111): a loaded read numbers its rows last on the tab (after the
// scope, so those stay contiguous) — Status counts its findings, Plan
// counts the plan's entries. The Status and Plan tabs name the yank
// scope their data was read under (0108, 0109).
func (m *Compositor) viewRow() string {
	build := func(compact bool) string {
		parts := make([]string, 0, viewCount)
		for i, name := range viewNames {
			v := viewKind(i)
			label := name
			var scope []string
			if v == viewStatus {
				scope = m.status.scope
			}
			if v == viewPlan {
				scope = m.planv.scope
			}
			if len(scope) > 0 {
				if compact {
					label += "[" + strconv.Itoa(len(scope)) + "]"
				} else {
					label += " [" + strings.Join(scope, ", ") + "]"
				}
			}
			if !compact {
				if v == viewStatus && m.status.read != nil {
					label += " (" + strconv.Itoa(len(m.status.read.Findings)) + ")"
				}
				if v == viewPlan && m.planv.read != nil {
					label += " (" + strconv.Itoa(planEntries(m.planv.read)) + ")"
				}
			}
			style := m.th.tabInactive
			if v == m.view {
				style = m.th.tabActive
			}
			part := style.Render(label)
			if m.stale[v] {
				part += m.th.staleBadge.Render("●")
			}
			parts = append(parts, part)
		}
		return " " + strings.Join(parts, " ")
	}
	row := build(false)
	if ansi.StringWidth(row) > m.w {
		row = build(true)
	}
	// The tab bar carries a bottom rule across the frame (0114), the
	// gh-dash tab treatment; layout() reserves the extra line.
	return m.th.tabBar.Width(m.w).Render(ansi.Truncate(row, m.w, "…"))
}

// viewBody renders the active non-Modules view: the Profile view's list
// (slice 1c), the Status view's findings list + detail panes (0108
// slice 2c), the Plan view's section list + detail panes (0109 slice
// 3), or a full-width pane with the not-loaded placeholder for the
// views whose bodies land later. The pane content gets the outer
// dimensions minus the border, like the base panes.
func (m *Compositor) viewBody(paneH int) string {
	switch m.view {
	case viewProfile:
		// No outer pane (issue 0144): the group cards are the frame,
		// at the full frame width (0145). The lines clip to the pane
		// height and pad back to it, so the footer stays at the
		// bottom of the frame.
		lines := strings.Split(m.profileBody(m.w), "\n")
		if len(lines) > paneH {
			lines = lines[:max(paneH, 0)]
		}
		for len(lines) < paneH {
			lines = append(lines, "")
		}
		return strings.Join(lines, "\n")
	case viewStatus:
		// Findings rows run long (paths + details): an even split, not
		// the Modules view's narrow nav. Each pane fills its slot,
		// content at the full inner width (0145).
		listW := m.w / 2
		left := m.pane(m.focus == focusNav, listW, paneH, m.statusList(listW-2, paneH-2))
		right := m.pane(m.focus == focusWork, m.w-listW, paneH, m.statusDetail(m.w-listW-2, paneH-2))
		return lipgloss.JoinHorizontal(lipgloss.Top, left, right)
	case viewPlan:
		// The report's entry lines run long: the same even split as
		// the Status view (0109).
		listW := m.w / 2
		left := m.pane(m.focus == focusNav, listW, paneH, m.planList(listW-2, paneH-2))
		right := m.pane(m.focus == focusWork, m.w-listW, paneH, m.planDetail(m.w-listW-2, paneH-2))
		return lipgloss.JoinHorizontal(lipgloss.Top, left, right)
	}
	return m.pane(false, m.w, paneH, m.th.loading.Render(" not loaded"))
}

// detailWindow cuts the visible window out of a detail pane's lines
// (0111): offset scrolls, and a dim overflow row replaces the
// first/last visible line when content is clipped on that side — a
// long section never clips silently, and the mark names the way to the
// tail.
func detailWindow(th theme, lines []string, offset, h int) []string {
	if h < 1 {
		h = 1
	}
	if len(lines) <= h {
		return lines
	}
	if offset > len(lines)-h {
		offset = len(lines) - h
	}
	if offset < 0 {
		offset = 0
	}
	out := make([]string, h)
	copy(out, lines[offset:offset+h])
	if offset > 0 {
		out[0] = th.meta.Render("  … more above")
	}
	if offset+h < len(lines) {
		out[h-1] = th.meta.Render("  … more below (right arrow to the pane, j/k scroll)")
	}
	return out
}

// viewHook is the active view's pane-key handler: a non-Modules view
// receives pane keys through this hook, never through the Modules
// bindings. Nil for views without a body — pane keys are dead on an
// empty view.
func (m *Compositor) viewHook() func(s string) (tea.Cmd, bool) {
	if m.view == viewStatus {
		return m.statusKey
	}
	if m.view == viewPlan {
		return m.planKey
	}
	return nil
}
