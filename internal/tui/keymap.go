package tui

// T-tui-keymap: the binding table as data. The base dispatcher consults
// it, the footer hints and the contextual ? help render from it — one
// source of truth, so docs cannot drift from behavior. esc stays
// compositor-owned (one meaning: pop the top layer); shift is the
// dangerous version (p/P, d/D); there is no undo — drafts and confirms
// are the safety net.

import (
	"errors"
	"strconv"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/thedataflows/dotdrift/internal/service"
)

// errNoProfile is the restore index's pre-load refusal.
var errNoProfile = errors.New("restore: the profile is not loaded yet")

// keyTableEntry is one binding: the key, the pane it lives in ("nav",
// "work", "both"), its help text, and its handler.
type keyTableEntry struct {
	key  string
	pane string
	help string
	run  func(m *Compositor) tea.Cmd
}

// keyTable is the whole base keymap (a func: a package var's initializer
// would cycle through the help handler back to the table).
// TestKeys_noOrphanedBindings pins the set; TestKeys_table pins behavior.
func keyTable() []keyTableEntry {
	return []keyTableEntry{
		// nav pane
		{"j", "nav", "down", func(m *Compositor) tea.Cmd { m.nav.move(1); return m.syncWorkspace() }},
		{"k", "nav", "up", func(m *Compositor) tea.Cmd { m.nav.move(-1); return m.syncWorkspace() }},
		{"down", "nav", "down", func(m *Compositor) tea.Cmd { m.nav.move(1); return m.syncWorkspace() }},
		{"up", "nav", "up", func(m *Compositor) tea.Cmd { m.nav.move(-1); return m.syncWorkspace() }},
		{"h", "nav", "collapse / parent", func(m *Compositor) tea.Cmd { m.nav.collapse(); return m.syncWorkspace() }},
		{"l", "nav", "expand", func(m *Compositor) tea.Cmd { m.nav.expand(); return m.syncWorkspace() }},
		{"left", "nav", "collapse / parent", func(m *Compositor) tea.Cmd { m.nav.collapse(); return m.syncWorkspace() }},
		{"right", "nav", "expand", func(m *Compositor) tea.Cmd { m.nav.expand(); return m.syncWorkspace() }},
		{"pgup", "nav", "page up", func(m *Compositor) tea.Cmd { m.nav.page(-1); return m.syncWorkspace() }},
		{"pgdown", "nav", "page down", func(m *Compositor) tea.Cmd { m.nav.page(1); return m.syncWorkspace() }},
		{"home", "nav", "first row", func(m *Compositor) tea.Cmd { m.nav.home(); return m.syncWorkspace() }},
		{"end", "nav", "last row", func(m *Compositor) tea.Cmd { m.nav.end(); return m.syncWorkspace() }},
		{"enter", "nav", "open in workspace", func(m *Compositor) tea.Cmd { m.focus = focusWork; return m.syncWorkspace() }},
		{"n", "nav", "new module", func(m *Compositor) tea.Cmd { m.openManageCreate(""); return nil }},
		{"m", "nav", "manage modules", func(m *Compositor) tea.Cmd { m.openManage(); return nil }},
		// shared
		{"/", "both", "palette", func(m *Compositor) tea.Cmd { m.openPalette(); return nil }},
		{"?", "both", "help", func(m *Compositor) tea.Cmd { m.openHelp(); return nil }},
		{"q", "both", "quit", func(m *Compositor) tea.Cmd { return m.quit() }},
		{"p", "both", "plan", func(m *Compositor) tea.Cmd { return m.openPlan() }},
		{"P", "both", "apply", func(m *Compositor) tea.Cmd { return m.startApply() }},
		{"tab", "both", "switch pane", func(m *Compositor) tea.Cmd { m.focus = (m.focus + 1) % 2; return nil }},
		// workspace pane
		{"j", "work", "scroll down", func(m *Compositor) tea.Cmd { m.ws.move(1); return nil }},
		{"k", "work", "scroll up", func(m *Compositor) tea.Cmd { m.ws.move(-1); return nil }},
		{"down", "work", "scroll down", func(m *Compositor) tea.Cmd { m.ws.move(1); return nil }},
		{"up", "work", "scroll up", func(m *Compositor) tea.Cmd { m.ws.move(-1); return nil }},
		{"pgup", "work", "page up", func(m *Compositor) tea.Cmd { m.ws.page(-1); return nil }},
		{"pgdown", "work", "page down", func(m *Compositor) tea.Cmd { m.ws.page(1); return nil }},
		{"home", "work", "first row", func(m *Compositor) tea.Cmd { m.ws.home(); return nil }},
		{"end", "work", "last row", func(m *Compositor) tea.Cmd { m.ws.end(); return nil }},
		{"enter", "work", "edit field", func(m *Compositor) tea.Cmd { m.startEdit(); return nil }},
		{"e", "work", "edit field", func(m *Compositor) tea.Cmd { m.startEdit(); return nil }},
		{"a", "both", "add row / apply detail", func(m *Compositor) tea.Cmd {
			// A live or ended run takes precedence from either pane;
			// otherwise a is the workspace's add-row.
			if m.apply != nil {
				m.openApplyDetail()
				return nil
			}
			if m.focus == focusWork {
				m.startAdd()
			}
			return nil
		}},
		{"d", "work", "remove row", func(m *Compositor) tea.Cmd { m.confirmRemoveRow(); return nil }},
		{"D", "work", "discard draft", func(m *Compositor) tea.Cmd { m.confirmDiscard(); return nil }},
		{"ctrl+s", "work", "save draft", func(m *Compositor) tea.Cmd { return m.saveDraft() }},
		{"L", "work", "cycle layer", func(m *Compositor) tea.Cmd { return m.cycleLayer() }},
		{"w", "work", "writes actions", func(m *Compositor) tea.Cmd { m.openWritesMenu(); return nil }},
	}
}

// dispatchKey runs the binding for the pressed key and focused pane.
// shift+tab aliases tab (two panes: reverse is forward).
func (m *Compositor) dispatchKey(s string) (tea.Cmd, bool) {
	if s == "shift+tab" {
		s = "tab"
	}
	pane := "nav"
	if m.focus == focusWork {
		pane = "work"
	}
	for _, b := range keyTable() {
		if b.key == s && (b.pane == pane || b.pane == "both") {
			return b.run(m), true
		}
	}
	return nil, false
}

// --- contextual help ---

// helpRow is one "keys → what" line.
type helpRow struct {
	keys string
	what string
}

// helpModel is the contextual ? modal: only bindings valid right now.
type helpModel struct {
	th   theme
	rows []helpRow
	done bool
}

func (h *helpModel) finished() bool { return h.done }

func (h *helpModel) update(msg tea.Msg) tea.Cmd {
	if _, ok := msg.(tea.KeyPressMsg); ok {
		h.done = true // any key closes
	}
	return nil
}

func (h *helpModel) view(w, _ int) string {
	var b strings.Builder
	b.WriteString(h.th.modalTitle.Render("keys") + "\n\n")
	for _, r := range h.rows {
		b.WriteString("  " + h.th.sectionLabel.Render(r.keys) + "  " + r.what + "\n")
	}
	return h.th.modalBorder.Padding(1, 2).Render(strings.TrimRight(b.String(), "\n"))
}

// openHelp pushes the help modal built for the current state.
func (m *Compositor) openHelp() {
	m.modals = append(m.modals, &helpModel{th: m.th, rows: m.helpRows()})
}

// helpRows: modal open → that modal's keys; editing → the edit keys;
// otherwise the focused pane's table rows.
func (m *Compositor) helpRows() []helpRow {
	if len(m.modals) > 0 {
		switch m.modals[len(m.modals)-1].(type) {
		case *paletteModel:
			return []helpRow{
				{"type", "filter"}, {"enter", "choose"}, {"up/down", "move"},
				{"ctrl+n", "create module (no matches)"}, {"esc", "close"},
			}
		case *elevationModel:
			return []helpRow{{"type", "password"}, {"enter", "submit"}, {"esc", "cancel (aborts the operation)"}}
		case *confirmModel:
			return []helpRow{{"y", "confirm"}, {"any other key", "cancel"}, {"esc", "cancel"}}
		case *choiceModel:
			return []helpRow{{"up/down", "pick"}, {"enter", "choose"}, {"esc", "cancel"}}
		case *applyDetailModel:
			return []helpRow{{"j/k", "scroll output"}, {"ctrl+c", "cancel the run"}, {"esc", "close (the run lives on)"}}
		case *planModel:
			return []helpRow{{"esc", "close"}}
		default:
			return []helpRow{{"esc", "close"}, {"", "the dialog's own hints apply"}}
		}
	}
	if m.ws.editing != nil {
		return []helpRow{{"type", "edit the field"}, {"enter", "commit"}, {"esc", "cancel"}, {"?", "this help"}}
	}
	pane := "nav"
	if m.focus == focusWork {
		pane = "work"
	}
	var rows []helpRow
	seen := map[string]bool{}
	for _, b := range keyTable() {
		if (b.pane == pane || b.pane == "both") && !seen[b.key+b.help] {
			seen[b.key+b.help] = true
			rows = append(rows, helpRow{b.key, b.help})
		}
	}
	return rows
}

// --- plan surface ---

// planPreviewMsg carries the read-only plan classification (p) — never
// gated, never credentialed.
type planPreviewMsg struct {
	previews []service.StepPreview
	err      error
}

// planModel is the read-only plan modal: the steps, the privilege
// markers, the overwrite counts — what P would run, without running it.
type planModel struct {
	th       theme
	previews []service.StepPreview
	err      error
}

func (p *planModel) update(msg tea.Msg) tea.Cmd { return nil }

func (p *planModel) view(w, _ int) string {
	var b strings.Builder
	if p.err != nil {
		b.WriteString(p.th.errorMark.Render("plan failed: "+firstLineOf(p.err.Error())) + "\n")
	} else {
		p.th.modalTitle.Render("plan")
		b.WriteString(p.th.modalTitle.Render("plan — "+strconv.Itoa(len(p.previews))+" step(s)") + "\n\n")
		for _, s := range p.previews {
			line := "  " + s.Name
			if s.NeedsTTY {
				line += "  [tty]"
			}
			if len(s.Overwrites) > 0 {
				line += "  overwrites " + strconv.Itoa(len(s.Overwrites))
			}
			b.WriteString(line + "\n")
			if s.Reason != "" {
				b.WriteString(p.th.reasonMark.Render("      "+s.Reason) + "\n")
			}
		}
		if len(p.previews) == 0 {
			b.WriteString(p.th.disabledMark.Render("  nothing to do") + "\n")
		}
	}
	b.WriteString("\n" + p.th.meta.Render("  esc close"))
	return p.th.modalBorder.Padding(1, 2).Render(strings.TrimRight(b.String(), "\n"))
}

// openPlan previews the plan (p) — the classification read, no gates.
func (m *Compositor) openPlan() tea.Cmd {
	if m.applyFor == nil {
		m.message, m.msgErr = "no apply path in this shell", true
		return nil
	}
	l := m.applyFor(m.facts)
	return func() tea.Msg {
		previews, err := l.Preview(service.ApplyOpts{ProfilePath: m.root})
		return planPreviewMsg{previews: previews, err: err}
	}
}

// --- writes menu ---

// openWritesMenu opens the writes actions (w): onboard / restore /
// generate, each its M14 dialog absorbed as a modal.
func (m *Compositor) openWritesMenu() {
	if m.writesFor == nil {
		m.message, m.msgErr = "no writes path in this shell", true
		return
	}
	m.modals = append(m.modals, &writesMenuModel{c: m, th: m.th})
}

// writesMenuModel is the three-entry chooser.
type writesMenuModel struct {
	c    *Compositor
	th   theme
	cur  int
	done bool
}

func (w *writesMenuModel) finished() bool { return w.done }

func (w *writesMenuModel) update(msg tea.Msg) tea.Cmd {
	k, ok := msg.(tea.KeyPressMsg)
	if !ok {
		return nil
	}
	switch k.String() {
	case "up", "k":
		if w.cur > 0 {
			w.cur--
		}
	case "down", "j":
		if w.cur < 2 {
			w.cur++
		}
	case "enter":
		w.done = true
		choice := w.cur
		w.c.afterPop = func() tea.Cmd {
			w.c.openWritesDialog(choice)
			return nil
		}
	}
	return nil
}

func (w *writesMenuModel) view(_, _ int) string {
	entries := []string{"onboard — link existing dotfiles into the profile",
		"restore — take backups back", "generate — scaffold a module"}
	var b strings.Builder
	b.WriteString(w.th.modalTitle.Render("writes") + "\n\n")
	for i, e := range entries {
		if i == w.cur {
			b.WriteString(w.th.cursorRow.Render(" "+e) + "\n")
		} else {
			b.WriteString("  " + e + "\n")
		}
	}
	b.WriteString("\n" + w.th.meta.Render("  up/down pick · enter open · esc back"))
	return w.th.modalBorder.Padding(1, 2).Render(strings.TrimRight(b.String(), "\n"))
}

// openWritesDialog opens the chosen M14 writes dialog as a modal.
func (m *Compositor) openWritesDialog(choice int) {
	wr := m.writesFor(m.facts)
	var d dialog
	switch choice {
	case 0:
		d = newOnboardDialog(wr, m.root)
	case 1:
		d = newRestoreDialog(wr, m.root, m.restoreIndex)
	default:
		d = newGenerateDialog(wr, m.root)
	}
	m.modals = append(m.modals, &dialogModal{d: d, th: m.th})
}

// restoreIndex feeds the restore dialog from the last modules read.
func (m *Compositor) restoreIndex() (map[string]map[string][]service.RestoreHit, error) {
	if m.modulesRead == nil || m.modulesRead.Profile == nil {
		return nil, errNoProfile
	}
	return service.IndexBackups(service.ModuleLayers(m.modulesRead.Profile)), nil
}

// --- mouse on the base screen ---

// baseMouse handles clicks and wheels on the two panes (modal content
// handles its own; this runs only when no modal is open). Row hit math:
// header 1 + border 1 + group title 1 → pane rows start at y=3; the
// workspace's title line takes one more (y=4).
func (m *Compositor) baseMouse(msg tea.Msg) tea.Cmd {
	navW, _, _ := m.layout()
	switch msg := msg.(type) {
	case tea.MouseWheelMsg:
		if msg.X < navW {
			switch msg.Button {
			case tea.MouseWheelDown:
				m.nav.move(1)
			case tea.MouseWheelUp:
				m.nav.move(-1)
			}
			return m.syncWorkspace()
		}
		switch msg.Button {
		case tea.MouseWheelDown:
			m.ws.move(1)
		case tea.MouseWheelUp:
			m.ws.move(-1)
		}
	case tea.MouseClickMsg:
		if msg.Button != tea.MouseLeft {
			return nil
		}
		if msg.X < navW {
			m.focus = focusNav
			idx := m.nav.offset + msg.Y - 3
			if idx >= 0 && idx < len(m.nav.rows()) {
				m.nav.cursor = idx
			}
			return m.syncWorkspace()
		}
		m.focus = focusWork
		idx := m.ws.offset + msg.Y - 4
		if m.ws.selectableRow(idx) {
			m.ws.cursor = idx
			if m.doubleClick(idx) {
				m.startEdit()
			}
		}
	}
	return nil
}

// doubleClick tracks the last workspace click: same row within 500ms is
// a double-click (edit).
func (m *Compositor) doubleClick(idx int) bool {
	now := time.Now()
	double := idx == m.lastClickRow && now.Sub(m.lastClickAt) < 500*time.Millisecond
	m.lastClickRow, m.lastClickAt = idx, now
	return double
}
