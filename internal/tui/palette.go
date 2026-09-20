package tui

// T-tui-palette: the `/` fuzzy palette. Three indexed scopes in fixed
// section order — modules+layers, contextually valid actions, the
// current module's fields — ranked by sahilm/fuzzy (vendored) within a
// section, with in-session recents as tiebreak. The palette is a modal:
// it captures all keys while open, esc pops and restores the exact prior
// state (a navigator, not a command line). Choosing runs through the
// compositor's afterPop so the palette is gone before jumps/confirm
// modals land.

import (
	"slices"
	"sort"
	"strconv"
	"strings"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/sahilm/fuzzy"
	"github.com/thedataflows/dotdrift/internal/service"
)

// paletteSection is the fixed section order.
type paletteSection int

const (
	sectionModules paletteSection = iota
	sectionActions
	sectionFields
)

// paletteEntry is one indexed target. id is stable for recents:
// "mod:<id>/<layer>", "act:<action>", "field:<section>".
type paletteEntry struct {
	id           string
	section      paletteSection
	label        string
	context      string
	moduleID     string
	layer        string // "" for the module row itself
	dir          string
	fieldSection string // fields: the workspace section to deep-link
	action       string // actions: apply | save | discard | manage | new-module
}

// paletteModel is the fuzzy palette modal.
type paletteModel struct {
	c      *Compositor
	th     theme
	query  []rune
	all    []paletteEntry
	rows   []paletteEntry // the filtered, ranked view
	sel    int
	scroll int
	done   bool

	// esc restores exactly this.
	snapFocus  paneFocus
	snapNavCur int
	snapWsCur  int
	snapWsOff  int

	// boxY of the last render, for mouse row hit-testing.
	boxY int
}

const paletteMaxVisible = 10

// openPalette builds and pushes the palette (/).
func (m *Compositor) openPalette() {
	p := &paletteModel{c: m, th: m.th}
	p.all = m.paletteEntries()
	p.snapFocus, p.snapNavCur = m.focus, m.nav.cursor
	p.snapWsCur, p.snapWsOff = m.ws.cursor, m.ws.offset
	p.refilter()
	m.modals = append(m.modals, p)
}

func (p *paletteModel) finished() bool { return p.done }

// cancel is the esc-pop hook: the exact prior state returns.
func (p *paletteModel) cancel() {
	m := p.c
	m.focus = p.snapFocus
	m.nav.cursor = p.snapNavCur
	m.ws.cursor = p.snapWsCur
	m.ws.offset = p.snapWsOff
}

// paletteEntries indexes the three scopes from live compositor state.
func (m *Compositor) paletteEntries() []paletteEntry {
	var out []paletteEntry
	for _, mod := range m.nav.modules {
		out = append(out, paletteEntry{
			id: "mod:" + mod.id + "/", section: sectionModules,
			label: mod.id, context: "module", moduleID: mod.id,
		})
		for _, l := range mod.layers {
			if l.layer == "base" {
				continue // the module row covers base
			}
			out = append(out, paletteEntry{
				id: "mod:" + mod.id + "/" + l.layer, section: sectionModules,
				label: mod.id + " · " + l.label, context: "module",
				moduleID: mod.id, layer: l.layer, dir: l.dir,
			})
		}
	}
	out = append(out, m.paletteActions()...)
	// Fields: the current module's sections that have content rows.
	seen := map[string]int{}
	for _, r := range m.ws.rows {
		if r.header || r.hint {
			continue
		}
		seen[r.section]++
	}
	for _, sec := range []string{"meta", "packages", "links", "writes", "when", "hooks", "systemd.units", "tools", "secrets", "mounts", "smb"} {
		if n := seen[sec]; n > 0 {
			out = append(out, paletteEntry{
				id: "field:" + sec, section: sectionFields,
				label: sec, context: "field · " + m.ws.moduleID,
				moduleID: m.ws.moduleID, fieldSection: sec,
			})
		}
	}
	return out
}

// paletteActions lists the contextually valid verbs — only actions that
// can run right now appear.
func (m *Compositor) paletteActions() []paletteEntry {
	var out []paletteEntry
	act := func(key, label, hint string) {
		out = append(out, paletteEntry{id: "act:" + key, section: sectionActions, label: label, context: "action", action: key})
	}
	if m.applyFor != nil && !m.applying {
		act("apply", "apply", "run the plan")
	}
	if _, ok := m.reader.(service.ConfigEditor); ok {
		act("manage", "manage modules", "create / move / delete")
		act("new-module", "new module", "scaffold a module")
	}
	if m.ws.draft != nil {
		act("save", "save draft", "write "+m.ws.moduleID+" changes")
		act("discard", "discard draft", "drop "+m.ws.moduleID+" changes")
	}
	return out
}

// refilter recomputes rows from the query: empty → recents then all
// modules; otherwise per-section fuzzy ranking (fixed section order,
// score first, recents as tiebreak).
func (p *paletteModel) refilter() {
	m := p.c
	recentRank := map[string]int{}
	for i, id := range m.paletteRecents {
		recentRank[id] = i
	}
	byID := map[string]paletteEntry{}
	for _, e := range p.all {
		byID[e.id] = e
	}

	p.rows = nil
	if len(p.query) == 0 {
		for _, id := range m.paletteRecents {
			if e, ok := byID[id]; ok {
				p.rows = append(p.rows, e)
			}
		}
		for _, e := range p.all {
			if e.section == sectionModules {
				if _, isRecent := recentRank[e.id]; !isRecent {
					p.rows = append(p.rows, e)
				}
			}
		}
	} else {
		q := string(p.query)
		for sec := sectionModules; sec <= sectionFields; sec++ {
			var entries []paletteEntry
			for _, e := range p.all {
				if e.section == sec {
					entries = append(entries, e)
				}
			}
			labels := make([]string, len(entries))
			for i, e := range entries {
				labels[i] = e.label
			}
			ranks := fuzzy.Find(q, labels)
			// Score first (lower is better) with prefix/exact affinity
			// (an exact-prefix module outranks a longer fuzzy hit),
			// recency the tiebreak, then declaration order.
			affinity := func(idx int) int {
				label := strings.ToLower(entries[idx].label)
				if label == strings.ToLower(q) {
					return -2000
				}
				if strings.HasPrefix(label, strings.ToLower(q)) {
					return -1000
				}
				return 0
			}
			recency := func(idx int) int {
				if r, ok := recentRank[entries[idx].id]; ok {
					return r
				}
				return len(p.all) // never-recent sorts last
			}
			// Sort: affinity tier (exact > prefix > fuzzy), then shorter
			// labels, then the library score, then recency, then
			// declaration order.
			sort.SliceStable(ranks, func(i, j int) bool {
				ai, aj := affinity(ranks[i].Index), affinity(ranks[j].Index)
				if ai != aj {
					return ai < aj
				}
				li, lj := len(entries[ranks[i].Index].label), len(entries[ranks[j].Index].label)
				if li != lj {
					return li < lj // "demo" outranks "demo · user cri"
				}
				if ranks[i].Score != ranks[j].Score {
					return ranks[i].Score < ranks[j].Score
				}
				if ri, rj := recency(ranks[i].Index), recency(ranks[j].Index); ri != rj {
					return ri < rj
				}
				return ranks[i].Index < ranks[j].Index
			})
			for _, r := range ranks {
				p.rows = append(p.rows, entries[r.Index])
			}
		}
	}
	if p.sel >= len(p.rows) {
		p.sel = max(0, len(p.rows)-1)
	}
	p.scroll = 0
}

// visibleLabels is the test seam: labels of the current rows.
func (p *paletteModel) visibleLabels() []string {
	var out []string
	for _, r := range p.rows {
		out = append(out, r.label)
	}
	return out
}

// actionLabels is the test seam: the currently valid action labels.
func (p *paletteModel) actionLabels() []string {
	var out []string
	for _, e := range p.all {
		if e.section == sectionActions {
			out = append(out, e.label)
		}
	}
	return out
}

func (p *paletteModel) update(msg tea.Msg) tea.Cmd {
	switch msg := msg.(type) {
	case tea.KeyPressMsg:
		switch msg.String() {
		case "enter":
			if len(p.rows) > 0 {
				e := p.rows[p.sel]
				p.choose(e)
			}
			return nil
		case "up", "ctrl+p":
			if p.sel > 0 {
				p.sel--
			}
		case "down", "ctrl+n":
			// ctrl+n only creates from the no-results state; otherwise it
			// is the emacs down.
			if len(p.rows) == 0 {
				p.done = true
				p.c.afterPop = func() tea.Cmd {
					p.c.openManageCreate(string(p.query))
					return nil
				}
				return nil
			}
			if p.sel < len(p.rows)-1 {
				p.sel++
			}
		case "backspace":
			if len(p.query) > 0 {
				p.query = p.query[:len(p.query)-1]
				p.refilter()
			}
		default:
			if msg.Text != "" {
				p.query = append(p.query, []rune(msg.Text)...)
				p.refilter()
				p.sel = 0
			}
		}
		if p.sel < p.scroll {
			p.scroll = p.sel
		}
		if p.sel >= p.scroll+paletteMaxVisible {
			p.scroll = p.sel - paletteMaxVisible + 1
		}
	case tea.MouseWheelMsg:
		if msg.Button == tea.MouseWheelUp && p.sel > 0 {
			p.sel--
		}
		if msg.Button == tea.MouseWheelDown && p.sel < len(p.rows)-1 {
			p.sel++
		}
	case tea.MouseClickMsg:
		if msg.Button == tea.MouseLeft {
			// Row hit-test against the last render's box layout.
			if idx := p.hitRow(msg.Y); idx >= 0 && idx < len(p.rows) {
				p.choose(p.rows[idx])
			}
		}
	}
	return nil
}

// hitRow maps a click Y to a visible row index, or -1.
func (p *paletteModel) hitRow(y int) int {
	// Box: border 1, padding 1, input line 1, blank 1 → rows start here.
	first := p.boxY + 4
	idx := y - first
	if idx < 0 || idx >= paletteMaxVisible {
		return -1
	}
	return p.scroll + idx
}

// choose remembers the jump (recents, cap 8, in-session only) and defers
// the effect past the pop.
func (p *paletteModel) choose(e paletteEntry) {
	m := p.c
	m.paletteRecents = append([]string{e.id},
		slices.DeleteFunc(slices.Clone(m.paletteRecents), func(x string) bool { return x == e.id })...)
	if len(m.paletteRecents) > 8 {
		m.paletteRecents = m.paletteRecents[:8]
	}
	p.done = true
	m.afterPop = func() tea.Cmd { return m.runPaletteEntry(e) }
}

// runPaletteEntry executes the chosen entry after the palette closed:
// module/layer jumps keep nav and workspace in sync (focus to the
// workspace), fields deep-link the cursor, actions run their usual path.
func (m *Compositor) runPaletteEntry(e paletteEntry) tea.Cmd {
	switch e.section {
	case sectionModules:
		m.jumpToModule(e.moduleID, e.layer)
		m.focus = focusWork
		return m.syncWorkspace()
	case sectionFields:
		m.jumpToModule(e.moduleID, "")
		m.focus = focusWork
		cmd := m.syncWorkspace()
		for i, r := range m.ws.rows {
			if r.section == e.fieldSection && !r.header && !r.hint {
				m.ws.cursor = i
				break
			}
		}
		return cmd
	case sectionActions:
		switch e.action {
		case "apply":
			return m.startApply()
		case "save":
			return m.saveDraft()
		case "discard":
			m.confirmDiscard()
		case "manage":
			m.openManage()
		case "new-module":
			m.openManageCreate("")
		}
	}
	return nil
}

// jumpToModule moves the nav cursor onto the module (expanding for a
// layer child) without touching drafts — jumping away never prompts.
func (m *Compositor) jumpToModule(id, layer string) {
	if m.nav.expanded == nil {
		m.nav.expanded = map[string]bool{}
	}
	if layer != "" {
		m.nav.expanded[id] = true
	}
	for i, r := range m.nav.rows() {
		if r.moduleID == id && r.layer == layer {
			m.nav.cursor = i
			return
		}
	}
}

// sectionGlyph marks the entry's scope.
func (e paletteEntry) glyph() string {
	switch e.section {
	case sectionModules:
		return "◆"
	case sectionActions:
		return "▶"
	default:
		return "→"
	}
}

func (p *paletteModel) view(w, h int) string {
	var b strings.Builder
	b.WriteString(p.th.cursorRow.Render("/ " + string(p.query) + "▏"))
	b.WriteString("\n\n")
	if len(p.rows) == 0 {
		b.WriteString(p.th.disabledMark.Render("  no matches") + "\n")
		b.WriteString(p.th.meta.Render("  ctrl+n creates a module named «"+string(p.query)+"»") + "\n")
	} else {
		for i := p.scroll; i < len(p.rows) && i < p.scroll+paletteMaxVisible; i++ {
			e := p.rows[i]
			line := "  " + e.glyph() + " " + e.label + "  " + p.th.meta.Render(e.context)
			if i == p.sel {
				line = p.th.cursorRow.Render(" "+e.glyph()+" "+e.label) + "  " + p.th.meta.Render(e.context)
			}
			b.WriteString(line + "\n")
		}
		if len(p.rows) > paletteMaxVisible {
			b.WriteString(p.th.meta.Render("  … "+strconv.Itoa(len(p.rows)-paletteMaxVisible)+" more") + "\n")
		}
	}
	box := p.th.modalBorder.Padding(1, 2).Render(strings.TrimRight(b.String(), "\n"))
	p.boxY = max((h-lipgloss.Height(box))/2, 0)
	return box
}
