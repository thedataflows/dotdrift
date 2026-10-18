package tui

// T-tui-views slice 3 (issue 0109): the Plan view body on the slice
// 1b frame. Left: the merged plan's sections with counts (packages
// install/remove, tools, dotfiles, hooks, systemd, mounts, smb), in
// the report's order. Right: the selected section's entries exactly as
// service.RenderPlanReport renders them — the full report renders once
// into a buffer at landing and the detail extracts the section's
// block, never re-rendered by hand. d toggles the copy-mode diff list
// (Reads.Diff data, RenderDiff internal) in the detail pane. The read
// is the probe-free Reads.Plan, async on the Status pattern: entry/r
// schedules it, planLoadedMsg lands it, a second r in flight is a
// no-op. Yank-scoped: the title names the scope, a yank change marks
// the view stale, and the 0108 stale-seq landing rule holds — a mark
// landing after the read started survives the landing.

import (
	"bytes"
	"strconv"
	"strings"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"
	"github.com/thedataflows/dotdrift/internal/service"
)

// planLoadedMsg carries the landed plan read, its rendered report, and
// the copy-mode diffs (the diff toggle's data).
type planLoadedMsg struct {
	read   *service.PlanRead
	report string
	diffs  []service.DiffEntry
	err    error
}

// planViewModel is the Plan view's state: the read, the rendered
// report, the diffs, the yank-scope snapshot the read ran under, the
// diff toggle, and the cursor/scroll pair.
type planViewModel struct {
	pending  bool
	loadErr  string
	read     *service.PlanRead
	report   string
	diffs    []service.DiffEntry
	scope    []string // the yank snapshot the read ran under (0093); names the title
	seq      int      // staleSeq[viewPlan] when the read started (0108 fix round 1)
	showDiff bool
	cursor   int
	offset   int
}

// planSection is one left-pane row: the report section name and its
// rendered count text ("" marks an empty section — never listed).
type planSection struct {
	name  string
	count string
}

// planSections lists the sections with entries, in the report's order.
// packages merges the install and remove counts, hooks the pre and
// post counts; the other sections count their entries.
func planSections(r *service.PlanRead) []planSection {
	if r == nil || r.Plan == nil {
		return nil
	}
	p := r.Plan
	num := func(n int) string {
		if n == 0 {
			return ""
		}
		return strconv.Itoa(n)
	}
	var out []planSection
	add := func(name, count string) {
		if count != "" {
			out = append(out, planSection{name, count})
		}
	}
	var pkgs, hooks []string
	if s := num(len(p.Packages.Install)); s != "" {
		pkgs = append(pkgs, s+" install")
	}
	if s := num(len(p.Packages.Remove)); s != "" {
		pkgs = append(pkgs, s+" remove")
	}
	add("packages", strings.Join(pkgs, ", "))
	add("tools", num(len(p.Tools.Versions)))
	add("dotfiles", num(len(p.Dotfiles.Entries)))
	if s := num(len(p.Hooks.Pre)); s != "" {
		hooks = append(hooks, s+" pre")
	}
	if s := num(len(p.Hooks.Post)); s != "" {
		hooks = append(hooks, s+" post")
	}
	add("hooks", strings.Join(hooks, ", "))
	add("systemd", num(len(p.Systemd.Units)))
	add("mounts", num(len(p.Mounts.Entries)))
	add("smb", num(len(p.Smb.Modules)))
	return out
}

// reloadPlan starts a plan read cycle (first entry, r): the probe-free
// resolve, the report render, and the copy-mode diffs in one cmd — the
// shell keeps rendering while it runs. A second r in flight is a
// no-op.
func (m *Compositor) reloadPlan() tea.Cmd {
	if m.area == nil || m.planv.pending {
		return nil
	}
	m.planv.pending = true
	m.planv.loadErr = ""
	m.planv.scope = m.nav.yankedIDs()
	m.planv.seq = m.staleSeq[viewPlan] // a mark landing after this point survives the read
	root, scope := m.root, m.planv.scope
	return func() tea.Msg {
		r, err := m.area.Plan(root, scope, nil)
		if err != nil {
			return planLoadedMsg{err: err}
		}
		// The detail pane's data: the full report, rendered once; the
		// section blocks are cut from it at render time.
		var buf bytes.Buffer
		_ = service.RenderPlanReport(&buf, r, nil) // a buffer never fails
		// A diff failure never sinks a good read — the pane just has no
		// diff (the Status view's rule).
		diffs, _ := m.area.Diff(r.Plan, r.Profile.Root)
		return planLoadedMsg{read: r, report: buf.String(), diffs: diffs}
	}
}

// planLoaded lands the read: fresh data clears the stale mark, unless a
// mark landed after the read started (a yank change, a write, a
// finished apply) — the landed data is the old scope's and stays marked
// (0108 fix round 1). A failure keeps the old list only with the stale
// mark, or shows the failed state when there is no list.
func (m *Compositor) planLoaded(msg planLoadedMsg) tea.Cmd {
	if msg.err != nil {
		return m.planFailed(msg.err)
	}
	m.planv.pending = false
	m.planv.loadErr = ""
	m.planv.read = msg.read
	m.planv.report = msg.report
	m.planv.diffs = msg.diffs
	m.stale[viewPlan] = m.staleSeq[viewPlan] != m.planv.seq
	m.planv.clamp(len(planSections(msg.read)))
	return nil
}

// planFailed records a read failure: the old list survives under the
// stale mark; with no list the view shows the failed state.
func (m *Compositor) planFailed(err error) tea.Cmd {
	m.planv.pending = false
	m.planv.loadErr = err.Error()
	m.stale[viewPlan] = m.planv.read != nil
	m.message, m.msgErr = firstLineOf(err.Error()), true
	return nil
}

// clamp lands the cursor on a row after data changes.
func (p *planViewModel) clamp(n int) {
	if n == 0 {
		p.cursor, p.offset = 0, 0
		return
	}
	if p.cursor >= n {
		p.cursor = n - 1
	}
	if p.cursor < 0 {
		p.cursor = 0
	}
}

// planMove steps the cursor d section rows.
func (m *Compositor) planMove(d int) {
	m.planv.cursor += d
	m.planv.clamp(len(planSections(m.planv.read)))
}

// planKey is the Plan view's viewHook: the pane movement keys (they
// live in the one binding table) drive the section cursor, and d
// toggles the copy-mode diff list in the detail pane.
func (m *Compositor) planKey(s string) (tea.Cmd, bool) {
	page := m.h - 8 // pane content height (layout: 6 chrome lines + 2 border)
	switch s {
	case "j", "down":
		m.planMove(1)
	case "k", "up":
		m.planMove(-1)
	case "pgdown":
		m.planMove(page)
	case "pgup":
		m.planMove(-page)
	case "home":
		m.planv.cursor = 0
	case "end":
		m.planv.cursor = len(planSections(m.planv.read)) - 1
		m.planv.clamp(len(planSections(m.planv.read)))
	case "d":
		m.planv.showDiff = !m.planv.showDiff
	default:
		return nil, false
	}
	return nil, true
}

// planList renders the left pane: the section list in its current
// state (not loaded / resolving / loaded / failed).
func (m *Compositor) planList(w, h int) string {
	s := &m.planv
	switch {
	case s.read == nil && s.pending:
		return m.th.loading.Render(" resolving…")
	case s.read == nil && s.loadErr != "":
		return m.th.errorMark.Render(" plan read failed: " + firstLineOf(s.loadErr))
	case s.read == nil:
		return m.th.loading.Render(" not loaded")
	}
	var head []string
	if len(s.scope) > 0 {
		head = append(head, m.th.meta.Render(" scope: "+strings.Join(s.scope, ", ")))
	}
	if s.showDiff {
		head = append(head, m.th.meta.Render(" diff: on (d toggles)"))
	}
	secs := planSections(s.read)
	s.clamp(len(secs))
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
	if len(secs) == 0 {
		lines = append(lines, " plan is empty")
	}
	for i := s.offset; i < len(secs) && len(lines) < h; i++ {
		text := " " + secs[i].name + ": " + secs[i].count
		if i == s.cursor {
			// The cursor bar replaces the row's leading space (the nav
			// idiom).
			text = m.th.cursorRow.MaxWidth(w).Render(strings.TrimPrefix(text, " "))
		}
		lines = append(lines, ansi.Truncate(text, w, ""))
	}
	return strings.Join(lines, "\n")
}

// planDetail renders the right pane: the copy-mode diff list when the
// toggle is on, otherwise the selected section's entries as
// RenderPlanReport renders them.
func (m *Compositor) planDetail(w int) string {
	s := &m.planv
	if s.read == nil {
		return ""
	}
	if s.showDiff {
		if len(s.diffs) == 0 {
			return m.th.meta.Render(" no copy-mode diffs")
		}
		var b strings.Builder
		if err := service.RenderDiff(&b, s.diffs, "internal", false); err == nil {
			var lines []string
			for _, line := range strings.Split(strings.TrimSpace(b.String()), "\n") {
				lines = append(lines, ansi.Truncate(line, w, ""))
			}
			return strings.Join(lines, "\n")
		}
		return ""
	}
	secs := planSections(s.read)
	if s.cursor >= len(secs) {
		return ""
	}
	return m.planSectionDetail(secs[s.cursor].name, w)
}

// planReportBlocks maps a left-pane section to the report blocks that
// render it: the packages section reads the install and the remove
// block, every other section its own block.
var planReportBlocks = map[string][]string{
	"packages": {"packages", "remove"},
}

// planSectionDetail cuts the section's lines out of the rendered
// report, clamped to the pane width.
func (m *Compositor) planSectionDetail(sec string, w int) string {
	names := planReportBlocks[sec]
	if names == nil {
		names = []string{sec}
	}
	var lines []string
	for _, name := range names {
		lines = append(lines, reportBlock(m.planv.report, name)...)
	}
	for i, l := range lines {
		lines[i] = ansi.Truncate(l, w, "")
	}
	return strings.TrimRight(strings.Join(lines, "\n"), "\n")
}

// reportBlock extracts one section's lines from RenderPlanReport
// output: a block runs from its "<name>:" header line to the next
// unindented header line (entry lines are always indented).
func reportBlock(report, name string) []string {
	var out []string
	in := false
	for _, line := range strings.Split(report, "\n") {
		if !strings.HasPrefix(line, " ") && strings.HasSuffix(line, ":") {
			in = line == name+":"
		}
		if in {
			out = append(out, line)
		}
	}
	return out
}
