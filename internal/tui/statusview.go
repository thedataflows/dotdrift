package tui

// T-tui-views slice 2c (issue 0108): the Status view body on the slice
// 1b frame. Left: the findings list, grouped by section with counts —
// drift/unknown/unauthorized rows only (CLI parity), an all-OK section
// shows one "ok: all N checks passed" row, and a broken module from the
// tolerant load surfaces in a load-problems block (a broken module never
// reads as "no drift"). Right: the selected finding's detail; for a
// copy-mode dotfile finding the live diff (ReadsArea.Diff data). The
// read is async: entry/r runs the probe-free plan check first — a
// system-scope dotfile/mount/smb entry gates the read behind the
// elevation modal (sudo validated before the read starts, so no prompt
// ever reaches the altscreen); declined or failed validation runs the
// read unelevated and permission-denied probes report unauthorized
// (0108). A second r during a probe is a no-op (builder's call, named in
// the spec).

import (
	"context"
	"fmt"
	"strconv"
	"strings"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
	"github.com/thedataflows/dotdrift/internal/drift"
	"github.com/thedataflows/dotdrift/internal/profile"
	"github.com/thedataflows/dotdrift/internal/resolve"
	"github.com/thedataflows/dotdrift/internal/service"
)

// statusPlanMsg carries the probe-free plan read that decides whether
// the status read needs sudo.
type statusPlanMsg struct {
	plan *resolve.Plan
	err  error
}

// statusLoadedMsg carries the landed status read and its copy-mode
// dotfile diffs (the detail pane's data).
type statusLoadedMsg struct {
	read  *service.StatusRead
	diffs []service.DiffEntry
	err   error
}

// statusModel is the Status view's state: the read, the diffs, the
// yank-scope snapshot the read ran under, the section filter, and the
// cursor/scroll pair.
type statusModel struct {
	pending bool
	loadErr string
	read    *service.StatusRead
	diffs   []service.DiffEntry
	scope   []string // the yank snapshot the read ran under (0093); names the title
	filter  string   // a section name; "" = all sections
	cursor  int
	offset  int
}

// statusSectionOrder mirrors drift's report order (its unexported
// sectionOrder); the TUI never re-derives it.
var statusSectionOrder = []string{"packages", "tools", "dotfiles", "mounts", "smb", "orphans"}

// statusRow is one rendered list line: section headers and the summary
// are not selectable; finding rows and the all-OK row are.
type statusRow struct {
	sel     bool
	finding *drift.Finding
	okCount int // >0: the all-OK row of a section with this many checks
	text    string
	style   lipgloss.Style // zero = plain
}

// reloadStatus starts a status read cycle (first entry, r): the
// probe-free plan check first, the gated probing read after it lands.
// A second r during a probe is a no-op.
func (m *Compositor) reloadStatus() tea.Cmd {
	if m.area == nil || m.status.pending {
		return nil
	}
	m.status.pending = true
	m.status.loadErr = ""
	m.status.scope = m.nav.yankedIDs()
	root, scope := m.root, m.status.scope
	return func() tea.Msg {
		pr, err := m.area.Plan(root, scope, nil)
		if err != nil {
			return statusPlanMsg{err: err}
		}
		return statusPlanMsg{plan: pr.Plan}
	}
}

// statusPlanChecked routes the landed plan check: a system-scope entry
// and a wired sudo checker open the elevation gate; anything else starts
// the read unelevated.
func (m *Compositor) statusPlanChecked(msg statusPlanMsg) tea.Cmd {
	if msg.err != nil {
		return m.statusFailed(msg.err)
	}
	if needsStatusSudo(msg.plan) && m.sudoCheck != nil {
		m.modals = append(m.modals, &elevationModel{
			th:         m.th,
			reasons:    []string{"system-scope entries (dotfiles, mounts, smb) probe root-owned targets"},
			check:      m.sudoCheck,
			cancelNote: "cancel (the read runs unelevated)",
			onOK: func() {
				m.afterPop = func() tea.Cmd { return m.startStatusRead(true) }
			},
			onCancel: func(bool) {
				// Declined or aborted: the read runs unelevated; a
				// permission-denied probe reports unauthorized (0108).
				m.afterPop = func() tea.Cmd { return m.startStatusRead(false) }
			},
		})
		return nil
	}
	return m.startStatusRead(false)
}

// startStatusRead runs the probing read as a tea.Cmd — the shell keeps
// rendering while probes run (package queries, mise, system files).
// Elevated says the sudo gate passed: the probes then run under the
// fresh timestamp, as the CLI's per-probe sudo does.
func (m *Compositor) startStatusRead(elevated bool) tea.Cmd {
	root, scope := m.root, m.status.scope
	return func() tea.Msg {
		r, err := m.area.Status(context.Background(), service.StatusOpts{
			ProfilePath: root,
			Modules:     scope,
			Elevated:    elevated,
		})
		if err != nil {
			return statusLoadedMsg{err: err}
		}
		// The copy-mode diffs feed the detail pane; a diff failure never
		// sinks a good read — the pane just has no diff.
		diffs, _ := m.area.Diff(r.Plan, r.ProfileRoot)
		return statusLoadedMsg{read: r, diffs: diffs}
	}
}

// statusLoaded lands the read: fresh data clears the stale mark; a
// failure keeps the old list only with the stale mark (the design's
// anti-goal), or shows the failed state when there is no list.
func (m *Compositor) statusLoaded(msg statusLoadedMsg) tea.Cmd {
	if msg.err != nil {
		return m.statusFailed(msg.err)
	}
	m.status.pending = false
	m.status.loadErr = ""
	m.status.read = msg.read
	m.status.diffs = msg.diffs
	m.stale[viewStatus] = false
	m.status.clamp(m.statusRows())
	return nil
}

// statusFailed records a read failure: the old list survives under the
// stale mark; with no list the view shows the failed state.
func (m *Compositor) statusFailed(err error) tea.Cmd {
	m.status.pending = false
	m.status.loadErr = err.Error()
	m.stale[viewStatus] = m.status.read != nil
	m.message, m.msgErr = firstLineOf(err.Error()), true
	return nil
}

// needsStatusSudo reports whether the scoped plan probes root-owned
// targets: a system-scope dotfile or mount entry, or any smb entry
// (mounts and smb resolve system-scope only).
func needsStatusSudo(plan *resolve.Plan) bool {
	if plan == nil {
		return false
	}
	for _, e := range plan.Dotfiles.Entries {
		if e.Scope == profile.ScopeSystem {
			return true
		}
	}
	for _, e := range plan.Mounts.Entries {
		if e.Scope == profile.ScopeSystem {
			return true
		}
	}
	return len(plan.Smb.Modules) > 0
}

// brokenModules lists the tolerant load's broken modules (the nav greys
// the same rows): the Status view surfaces them so a broken module never
// reads as "no drift".
func brokenModules(p *profile.Profile) []profile.Skip {
	if p == nil {
		return nil
	}
	var out []profile.Skip
	for _, s := range p.Skipped {
		if strings.HasPrefix(s.Reason, "invalid module.toml") {
			out = append(out, s)
		}
	}
	return out
}

// statusCounts renders the named per-section counts ("" when all OK).
func statusCounts(driftN, unkN, unauthN int) string {
	var parts []string
	if driftN > 0 {
		parts = append(parts, fmt.Sprintf("%d drift", driftN))
	}
	if unkN > 0 {
		parts = append(parts, fmt.Sprintf("%d unknown", unkN))
	}
	if unauthN > 0 {
		parts = append(parts, fmt.Sprintf("%d unauthorized", unauthN))
	}
	return strings.Join(parts, ", ")
}

// statusRows builds the flat row list: the load-problems block, then per
// section (fixed order, filtered when set) a header with counts and its
// finding rows or the all-OK row, then the summary.
func (m *Compositor) statusRows() []statusRow {
	r := m.status.read
	if r == nil {
		return nil
	}
	var rows []statusRow
	broken := brokenModules(r.Profile)
	if len(broken) > 0 {
		rows = append(rows, statusRow{text: " load problems:", style: m.th.sectionLabel})
		for _, s := range broken {
			rows = append(rows, statusRow{text: "  " + s.Module.ID + ": " + s.Reason, style: m.th.errorMark})
		}
		rows = append(rows, statusRow{})
	}
	totalDrift, totalUnknown, totalUnauth := 0, 0, 0
	for _, sec := range statusSectionOrder {
		if m.status.filter != "" && m.status.filter != sec {
			continue
		}
		var driftN, unkN, unauthN, n int
		for i := range r.Findings {
			f := &r.Findings[i]
			if f.Section != sec {
				continue
			}
			n++
			switch f.Status {
			case drift.Drift:
				driftN++
			case drift.Unknown:
				unkN++
			case drift.Unauthorized:
				unauthN++
			}
		}
		if n == 0 {
			continue
		}
		totalDrift += driftN
		totalUnknown += unkN
		totalUnauth += unauthN
		header := " " + sec + ":"
		if counts := statusCounts(driftN, unkN, unauthN); counts != "" {
			header += " " + counts
		}
		rows = append(rows, statusRow{text: header, style: m.th.sectionLabel})
		if driftN+unkN+unauthN == 0 {
			rows = append(rows, statusRow{sel: true, okCount: n,
				text: "  ok: all " + strconv.Itoa(n) + " checks passed"})
			continue
		}
		for i := range r.Findings {
			f := &r.Findings[i]
			if f.Section != sec || f.Status == drift.OK {
				continue
			}
			rows = append(rows, statusRow{sel: true, finding: f,
				text: m.statusFindingLine(f), style: statusFindingStyle(m.th, f)})
		}
	}
	// The summary (the CLI's wording): broken modules suppress the
	// "no drift" claim — what resolved may be a subset.
	switch {
	case totalDrift == 0 && totalUnknown == 0 && totalUnauth == 0:
		if len(broken) == 0 {
			rows = append(rows, statusRow{}, statusRow{text: " no drift"})
		}
	default:
		word := "items"
		if totalDrift == 1 {
			word = "item"
		}
		summary := fmt.Sprintf(" drift: %d %s", totalDrift, word)
		if totalUnknown > 0 {
			summary += fmt.Sprintf(", %d unknown", totalUnknown)
		}
		if totalUnauth > 0 {
			summary += fmt.Sprintf(", %d unauthorized", totalUnauth)
		}
		rows = append(rows, statusRow{}, statusRow{text: summary})
	}
	return rows
}

// statusFindingLine renders one finding row with the CLI's wording:
// "module: item - detail", unknown marked (?), unauthorized marked
// (unauthorized).
func (m *Compositor) statusFindingLine(f *drift.Finding) string {
	mod := f.Module
	if mod == "" {
		mod = "?"
	}
	suffix := ""
	switch f.Status {
	case drift.Unknown:
		suffix = " (?)"
	case drift.Unauthorized:
		suffix = " (unauthorized)"
	}
	return fmt.Sprintf("  %s: %s - %s%s", mod, f.Item, f.Detail, suffix)
}

// statusFindingStyle picks the row hue: drift and unauthorized carry the
// error mark, unknown the warn mark.
func statusFindingStyle(th theme, f *drift.Finding) lipgloss.Style {
	if f.Status == drift.Unknown {
		return th.reasonMark
	}
	return th.errorMark
}

// clamp lands the cursor on a selectable row after data or filter
// changes.
func (s *statusModel) clamp(rows []statusRow) {
	if len(rows) == 0 {
		s.cursor, s.offset = 0, 0
		return
	}
	if s.cursor >= len(rows) {
		s.cursor = len(rows) - 1
	}
	if s.cursor < 0 {
		s.cursor = 0
	}
	if rows[s.cursor].sel {
		return
	}
	for i := s.cursor; i < len(rows); i++ {
		if rows[i].sel {
			s.cursor = i
			return
		}
	}
	for i := s.cursor; i >= 0; i-- {
		if rows[i].sel {
			s.cursor = i
			return
		}
	}
}

// statusMove steps the cursor d selectable rows (headers and blank rows
// are not selectable).
func (m *Compositor) statusMove(d int) {
	rows := m.statusRows()
	s := &m.status
	step := 1
	if d < 0 {
		step = -1
	}
	for n := 0; n < d*step; n++ {
		i := s.cursor + step
		for i >= 0 && i < len(rows) && !rows[i].sel {
			i += step
		}
		if i < 0 || i >= len(rows) {
			return
		}
		s.cursor = i
	}
}

// statusHome/statusEnd jump to the first/last selectable row.
func (m *Compositor) statusHome() {
	rows := m.statusRows()
	for i := range rows {
		if rows[i].sel {
			m.status.cursor = i
			return
		}
	}
}

func (m *Compositor) statusEnd() {
	rows := m.statusRows()
	for i := len(rows) - 1; i >= 0; i-- {
		if rows[i].sel {
			m.status.cursor = i
			return
		}
	}
}

// statusKey is the Status view's viewHook: the pane movement keys (they
// live in the one binding table) drive the findings cursor. enter/P on a
// finding land in Task 7.
func (m *Compositor) statusKey(s string) (tea.Cmd, bool) {
	page := m.h - 8 // pane content height (layout: 6 chrome lines + 2 border)
	switch s {
	case "j", "down":
		m.statusMove(1)
	case "k", "up":
		m.statusMove(-1)
	case "pgdown":
		m.statusMove(page)
	case "pgup":
		m.statusMove(-page)
	case "home":
		m.statusHome()
	case "end":
		m.statusEnd()
	default:
		return nil, false
	}
	return nil, true
}

// cycleStatusFilter is f (the one section-filter mechanism): all → each
// present section in report order → all. Inert off the Status view.
func (m *Compositor) cycleStatusFilter() tea.Cmd {
	if m.view != viewStatus || m.status.read == nil {
		return nil
	}
	var secs []string
	for _, sec := range statusSectionOrder {
		for i := range m.status.read.Findings {
			if m.status.read.Findings[i].Section == sec {
				secs = append(secs, sec)
				break
			}
		}
	}
	idx := -1
	for i, sec := range secs {
		if sec == m.status.filter {
			idx = i
			break
		}
	}
	if idx+1 < len(secs) {
		m.status.filter = secs[idx+1]
	} else {
		m.status.filter = ""
	}
	m.status.cursor, m.status.offset = 0, 0
	m.status.clamp(m.statusRows())
	return nil
}

// statusList renders the left pane: the findings list in its current
// state (not loaded / probing / loaded / failed).
func (m *Compositor) statusList(w, h int) string {
	s := &m.status
	switch {
	case s.read == nil && s.pending:
		return m.th.loading.Render(" probing…")
	case s.read == nil && s.loadErr != "":
		return m.th.errorMark.Render(" status read failed: " + firstLineOf(s.loadErr))
	case s.read == nil:
		return m.th.loading.Render(" not loaded")
	}
	var head []string
	if len(s.scope) > 0 {
		head = append(head, m.th.meta.Render(" scope: "+strings.Join(s.scope, ", ")))
	}
	if s.filter != "" {
		head = append(head, m.th.meta.Render(" section: "+s.filter+" (f cycles)"))
	}
	rows := m.statusRows()
	s.clamp(rows)
	avail := h - len(head)
	if avail < 1 {
		avail = 1
	}
	if s.cursor < s.offset {
		s.offset = s.cursor
	}
	if s.cursor >= s.offset+avail {
		s.offset = s.cursor - avail + 1
	}
	lines := head
	for i := s.offset; i < len(rows) && len(lines) < h; i++ {
		var line string
		if i == s.cursor && rows[i].sel {
			// The cursor bar replaces the row's leading space (the nav
			// idiom); the row's own hue survives inside it.
			line = m.th.cursorRow.MaxWidth(w).Render(rows[i].style.Render(strings.TrimPrefix(rows[i].text, " ")))
		} else {
			line = rows[i].style.Render(rows[i].text)
		}
		lines = append(lines, ansi.Truncate(line, w, ""))
	}
	return strings.Join(lines, "\n")
}

// statusDetail renders the right pane: the selected finding's module,
// item, state, and detail; for a copy-mode dotfile finding the live
// diff. The all-OK row shows its count.
func (m *Compositor) statusDetail(w int) string {
	s := &m.status
	if s.read == nil {
		return ""
	}
	rows := m.statusRows()
	if s.cursor >= len(rows) {
		return ""
	}
	row := rows[s.cursor]
	var b strings.Builder
	kv := func(k, v string) {
		if v == "" {
			return
		}
		b.WriteString(ansi.Truncate(m.th.rowKey.Render(fmt.Sprintf(" %-8s", k+":"))+" "+m.th.rowText.Render(v), w, "") + "\n")
	}
	if row.okCount > 0 {
		kv("state", "ok")
		kv("detail", "all "+strconv.Itoa(row.okCount)+" checks passed")
		return strings.TrimRight(b.String(), "\n")
	}
	f := row.finding
	if f == nil {
		return ""
	}
	kv("module", f.Module)
	kv("item", f.Item)
	kv("state", f.Status.String())
	kv("detail", f.Detail)
	if f.Section == "dotfiles" {
		for _, e := range s.diffs {
			if e.Target != f.Item {
				continue
			}
			var db strings.Builder
			if err := service.RenderDiff(&db, []service.DiffEntry{e}, "internal", false); err == nil {
				for _, line := range strings.Split(strings.TrimRight(db.String(), "\n"), "\n") {
					b.WriteString(ansi.Truncate(line, w, "") + "\n")
				}
			}
			break
		}
	}
	return strings.TrimRight(b.String(), "\n")
}
