package tui

// Issue 0148: the quickfilter for the read views (Status, Plan,
// Profile) — the nav filter's rules (0081) on whatever the focused
// panel shows. Each panel owns one paneFilter: f or / opens it, typing
// narrows the panel live, j/k and the arrows still move in a list
// panel, enter keeps the filter and ends the typing mode, esc removes
// it, ctrl+c quits. Matching is a case-insensitive substring test on
// the plain text of a row or line.

import (
	"strings"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"
)

// paneFilter is one panel's quickfilter state: the typing mode and
// the shared input (0135), like the nav filter's pair.
type paneFilter struct {
	filtering bool
	query     textInput
}

// active reports whether the filter takes part in the render: the
// typing mode, or an applied query after enter.
func (f *paneFilter) active() bool { return f.filtering || len(f.query.buf) > 0 }

func (f *paneFilter) clear() {
	f.filtering = false
	f.query = textInput{}
}

// view is the panel-bottom query line (the nav filter's line): the
// input with its caret while typing, a named filter after enter.
func (f *paneFilter) view(th theme, w int) string {
	if f.filtering {
		f.query.Cells = max(w-4, 1)
		return th.inputText.Render(" / ") + f.query.render(th, th.inputText, true)
	}
	f.query.Cells = max(w-16, 1)
	return th.meta.Render(" / ") + f.query.render(th, th.meta, false) + th.meta.Render(" · esc clears")
}

// matchText reports whether text contains the query, case-insensitive
// on the plain text (style codes stripped). An empty query matches.
func matchText(query, text string) bool {
	if query == "" {
		return true
	}
	return strings.Contains(strings.ToLower(ansi.Strip(text)), strings.ToLower(query))
}

// filterLines keeps the lines that match the query.
func filterLines(lines []string, query string) []string {
	if query == "" {
		return lines
	}
	out := lines[:0:0]
	for _, l := range lines {
		if matchText(query, l) {
			out = append(out, l)
		}
	}
	return out
}

// paneFilterFor returns the focused panel's filter in the active read
// view, nil on the Modules view (its panes keep their own / rules).
func (m *Compositor) paneFilterFor(focus paneFocus) *paneFilter {
	switch m.view {
	case viewStatus:
		if focus == focusWork {
			return &m.status.detailFilter
		}
		return &m.status.listFilter
	case viewPlan:
		if focus == focusWork {
			return &m.planv.detailFilter
		}
		return &m.planv.listFilter
	case viewProfile:
		return &m.profile.filter
	}
	return nil
}

// typingFilter returns the filter whose typing mode owns the keys in
// the active view, nil when none is typing.
func (m *Compositor) typingFilter() *paneFilter {
	for _, f := range []*paneFilter{&m.status.listFilter, &m.status.detailFilter,
		&m.planv.listFilter, &m.planv.detailFilter, &m.profile.filter} {
		if f.filtering {
			return f
		}
	}
	return nil
}

// openQuickFilter opens the focused panel's quickfilter (f, and /
// through the view hooks).
func (m *Compositor) openQuickFilter() tea.Cmd {
	if f := m.paneFilterFor(m.focus); f != nil {
		f.filtering = true
	}
	return nil
}

// afterFilterChange re-clamps the filtered panel after its query
// changed: the cursor lands on a visible row, the detail scroll
// resets to the top.
func (m *Compositor) afterFilterChange(f *paneFilter) {
	switch f {
	case &m.status.listFilter, &m.status.detailFilter:
		m.status.clamp(m.statusRows())
		m.status.dOffset = 0
	case &m.planv.listFilter, &m.planv.detailFilter:
		m.planv.clamp(len(m.planSections()))
		m.planv.dOffset = 0
	}
}

// quickFilterKey runs a typing filter's keys: esc removes the filter,
// enter keeps it, the movement keys still move in the panel (the view
// hook, with the focus on the filtered panel), and the shared input's
// keys edit the query.
func (m *Compositor) quickFilterKey(k tea.KeyPressMsg) tea.Cmd {
	f := m.typingFilter()
	if f == nil {
		return nil
	}
	switch k.String() {
	case "esc":
		f.clear()
		m.afterFilterChange(f)
		return nil
	case "enter":
		f.filtering = false
		return nil
	case "up", "k", "down", "j", "pgup", "pgdown", "home", "end":
		if h := m.viewHook(); h != nil {
			if cmd, ok := h(k.String()); ok {
				return cmd
			}
		}
		return nil
	}
	before := f.query.String()
	if f.query.handleKey(k) && f.query.String() != before {
		m.afterFilterChange(f)
	}
	return nil
}
