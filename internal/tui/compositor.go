package tui

// The compositor (M15, issue 0073): one full-screen base shell — header,
// nav, workspace, footer — plus a stack of modal layers over it. The top
// layer owns all input while open; the base keeps rendering underneath,
// dimmed. esc is owned here, not by individual views: it pops the top
// layer — modal first, then edit mode, then focus back to the nav. This
// kills the M14 per-view input fallthrough class of bug by construction.
// The old view stack (shell.go) stays alive until T-tui-cleanup.

import (
	"errors"
	"regexp"
	"strconv"
	"time"

	"charm.land/bubbles/v2/help"
	"charm.land/bubbles/v2/key"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/thedataflows/dotdrift/internal/facts"
	"github.com/thedataflows/dotdrift/internal/service"
)

// paneFocus names the base panes; exactly one is focused at a time.
type paneFocus int

const (
	focusNav paneFocus = iota
	focusWork
)

// modal is one layer over the base. The compositor owns esc (pop); every
// other message goes to the top modal's update.
type modal interface {
	view(w, h int) string
	update(msg tea.Msg) tea.Cmd
}

// Footer chrome messages: any operation over ~200ms announces itself with
// opStartedMsg and reports with opFinishedMsg. A success note fades after
// msgFadeAfter; a failure persists until the next user action.
type opStartedMsg struct{ name string }
type opFinishedMsg struct {
	note string
	err  error
}
type msgFadeMsg struct{ token int }
type spinTickMsg struct{}

const (
	msgFadeAfter = 4 * time.Second
	spinInterval = 100 * time.Millisecond
)

var spinFrames = []string{"⠋", "⠙", "⠹", "⠸", "⠼", "⠴", "⠦", "⠧", "⠇", "⠏"}

var stripANSIRe = regexp.MustCompile(`\x1b\[[0-9;:?]*[a-zA-Z]`)

// navLoadedMsg carries the modules read (T-tui-nav).
type navLoadedMsg struct {
	read *service.ModulesRead
	err  error
}

// Compositor is the M15 shell model.
type Compositor struct {
	th     theme
	isDark bool
	w, h   int

	area Reads

	nav   navModel
	ws    workspaceModel
	focus paneFocus

	modals []modal

	// afterPop is set by a modal's answer and runs after the modal pops
	// (conflict reload, gated apply start). Deferred as a func: pushing
	// the next modal must not happen before the pop.
	afterPop func() tea.Cmd

	// Apply wiring (T-tui-modals): the launcher and the sudo checker are
	// injected seams (no real sudo near tests); send is the program's
	// Send, set by Run, for the handover bridge.
	applyFor      func(*facts.Facts) ApplyLauncher
	sudoCheck     func(pw []byte) error
	send          func(tea.Msg)
	apply         *applyRunState
	applyPreviews []service.StepPreview
	pumpNoBlock   bool // tests: the event pump never blocks a settle loop

	// paletteRecents: the palette's last-8 selections, session-only.
	paletteRecents []string

	// writesFor builds the writes area (T-tui-keymap's w menu);
	// modulesRead keeps the last modules read for the restore index.
	writesFor   func(*facts.Facts) Writes
	modulesRead *service.ModulesRead

	// lastClick tracks the workspace double-click (edit) gesture.
	lastClickRow int
	lastClickAt  time.Time

	// Layer reads (T-tui-workspace): layerFor builds the 0065 config seam
	// once facts land; reader is the cached instance.
	layerFor func(*facts.Facts) LayerReader
	reader   LayerReader
	facts    *facts.Facts

	// Header chrome.
	root     string
	host     string
	user     string
	applying bool

	// store is the 0065 file-scoped draft ledger: layer dir → draft.
	store map[string]*wsDraft

	// Footer chrome.
	op       string
	spinIdx  int
	message  string
	msgErr   bool
	msgToken int

	help help.Model
}

// NewCompositor builds the M15 shell over a profile root. layerFor builds
// the workspace's layer reader once facts land (nil: the workspace shows
// identity placeholder text only). Experimental until it reaches parity
// with the M14 shell (T-tui-cleanup).
func NewCompositor(area Reads, root string, layerFor func(*facts.Facts) LayerReader) *Compositor {
	return &Compositor{
		th:       newTheme(true), // corrected by BackgroundColorMsg
		area:     area,
		root:     root,
		layerFor: layerFor,
		help:     help.New(),
		isDark:   true,
		nav:      navModel{pending: true},
		store:    map[string]*wsDraft{},
	}
}

// SetWrites wires the writes-area seam for the w menu (the adapter in
// cmd; tests set the field directly).
func (m *Compositor) SetWrites(writesFor func(*facts.Facts) Writes) {
	m.writesFor = writesFor
}

// SetApply wires the apply launcher and sudo checker seams (the adapter
// in cmd; tests set the fields directly).
func (m *Compositor) SetApply(applyFor func(*facts.Facts) ApplyLauncher, sudoCheck func([]byte) error) {
	m.applyFor = applyFor
	m.sudoCheck = sudoCheck
}

// Run starts the program full-screen (altscreen is set on every View).
func (m *Compositor) Run() error {
	p := tea.NewProgram(m)
	m.send = p.Send
	_, err := p.Run()
	return err
}

func (m *Compositor) Init() tea.Cmd {
	if m.area == nil {
		return nil
	}
	return func() tea.Msg {
		r, err := m.area.Modules(m.root, nil)
		return navLoadedMsg{read: r, err: err}
	}
}

func (m *Compositor) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.w, m.h = msg.Width, msg.Height
		return m, nil
	case tea.BackgroundColorMsg:
		m.isDark = msg.IsDark()
		m.th = newTheme(m.isDark)
		return m, nil
	case navLoadedMsg:
		sel := m.nav.selected()
		m.nav.pending = false
		if msg.err != nil {
			m.nav.loadErr = msg.err.Error()
		} else {
			m.nav.modules = navModules(msg.read)
			m.nav.loadErr = ""
			if f := msg.read.Facts; f != nil {
				m.host, m.user = f.Hostname, f.Username
				m.facts = f
			}
		}
		m.modulesRead = msg.read
		m.nav.restore(sel)
		return m, m.syncWorkspace()
	case layerLoadedMsg:
		if msg.dir != m.ws.activeDir() {
			return m, nil // stale: the cursor moved on
		}
		m.ws.pending = false
		var se *service.SchemaError
		switch {
		case errors.As(msg.err, &se):
			m.ws.setSchemaError(msg.err)
		case msg.err != nil:
			m.ws.loadErr = msg.err.Error()
		default:
			m.ws.loadErr = ""
			m.ws.setRead(msg.read)
		}
		m.attachDraft()
		return m, nil
	case saveFinishedMsg:
		return m, m.saveFinished(msg)
	case opStartedMsg:
		m.op = msg.name
		m.message, m.msgErr = "", false
		m.msgToken++
		return m, m.spinTick()
	case opFinishedMsg:
		m.op = ""
		m.message, m.msgErr = msg.note, msg.err != nil
		m.msgToken++
		if m.msgErr {
			return m, nil
		}
		tok := m.msgToken
		return m, tea.Tick(msgFadeAfter, func(time.Time) tea.Msg { return msgFadeMsg{tok} })
	case msgFadeMsg:
		if msg.token == m.msgToken && !m.msgErr {
			m.message = ""
		}
		return m, nil
	case spinTickMsg:
		if m.op == "" {
			return m, nil
		}
		m.spinIdx++
		return m, m.spinTick()
	}

	// Apply session messages are compositor-level, not modal input: they
	// flow while the detail inspector (or any modal) is open.
	switch msg := msg.(type) {
	case applyPreviewMsg:
		return m, m.applyPreview(msg)
	case applyStartedMsg:
		return m, m.applyStarted(msg)
	case applyEventMsg:
		return m, m.applyEvent(msg)
	case applyTickMsg:
		if m.applying {
			return m, m.waitApplyEvent()
		}
		return m, nil
	case applyDrainedMsg:
		return m, m.applyDrained()
	case applyWaitedMsg:
		return m, m.applyWaited(msg)
	case planPreviewMsg:
		m.modals = append(m.modals, &planModel{th: m.th, previews: msg.previews, err: msg.err})
		return m, nil
	case applyHandoverMsg:
		return m, tea.ExecProcess(msg.cmd, func(err error) tea.Msg {
			msg.done <- err
			return applyHandoverDoneMsg{}
		})
	}

	// The modal stack owns all other input; esc pops the top layer. A
	// modal reporting finished() pops itself after its answer.
	if len(m.modals) > 0 {
		if k, ok := msg.(tea.KeyPressMsg); ok && k.String() == "esc" {
			top := m.modals[len(m.modals)-1]
			if cm, ok := top.(interface{ cancel() }); ok {
				cm.cancel() // the elevation prompt's esc aborts the gated op
			}
			m.modals = m.modals[:len(m.modals)-1]
			return m, nil
		}
		// ? opens help even over a modal — except the elevation prompt,
		// where ? is a legitimate password character.
		if k, ok := msg.(tea.KeyPressMsg); ok && k.String() == "?" {
			if _, isElevation := m.modals[len(m.modals)-1].(*elevationModel); !isElevation {
				m.openHelp()
				return m, nil
			}
		}
		top := m.modals[len(m.modals)-1]
		cmd := top.update(msg)
		if fm, ok := top.(interface{ finished() bool }); ok && fm.finished() {
			m.modals = m.modals[:len(m.modals)-1]
			if m.afterPop != nil {
				fn := m.afterPop
				m.afterPop = nil
				return m, fn()
			}
		}
		return m, cmd
	}

	// Edit mode owns all keys while a field input is active (? still
	// opens help).
	if m.ws.editing != nil {
		if k, ok := msg.(tea.KeyPressMsg); ok {
			if k.String() == "?" {
				m.openHelp()
				return m, nil
			}
			return m, m.editKey(k)
		}
		return m, nil
	}

	// Mouse on the base panes.
	switch msg.(type) {
	case tea.MouseWheelMsg, tea.MouseClickMsg:
		return m, m.baseMouse(msg)
	}

	if k, ok := msg.(tea.KeyPressMsg); ok {
		return m.handleKey(k)
	}
	return m, nil
}

func (m *Compositor) handleKey(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	// Any user action clears a persistent failure message.
	if m.msgErr {
		m.message, m.msgErr = "", false
	}
	if msg.String() == "esc" {
		// The compositor's one esc rule on the base: focus back to nav.
		if m.focus != focusNav {
			m.focus = focusNav
		}
		return m, nil
	}
	if cmd, ok := m.dispatchKey(msg.String()); ok {
		return m, cmd
	}
	return m, nil
}

// cycleLayer advances the active layer tab (base → user → host, wrapping)
// and pulls the nav cursor along — the two never disagree.
func (m *Compositor) cycleLayer() tea.Cmd {
	if len(m.ws.tabs) == 0 {
		return nil
	}
	m.ws.active = (m.ws.active + 1) % len(m.ws.tabs)
	m.syncNavToActiveLayer()
	return m.loadLayer()
}

// syncNavToActiveLayer moves the nav cursor onto the active layer tab's
// child row, expanding the module when needed.
func (m *Compositor) syncNavToActiveLayer() {
	if m.ws.moduleID == "" || m.ws.active >= len(m.ws.tabs) {
		return
	}
	if m.nav.expanded == nil {
		m.nav.expanded = map[string]bool{}
	}
	m.nav.expanded[m.ws.moduleID] = true
	want := m.ws.tabs[m.ws.active]
	for i, r := range m.nav.rows() {
		if r.moduleID == m.ws.moduleID && r.layer == want.layer && r.dir == want.dir {
			m.nav.cursor = i
			return
		}
	}
}

// syncWorkspace points the workspace at the nav selection — the two never
// disagree — and schedules the layer read when the target file changed.
func (m *Compositor) syncWorkspace() tea.Cmd {
	sel := m.nav.selected()
	if sel.moduleID == "" {
		m.ws = workspaceModel{}
		return nil
	}
	if sel.moduleID != m.ws.moduleID {
		m.ws = workspaceModel{moduleID: sel.moduleID, placeholder: sel.moduleID}
		for i := range m.nav.modules {
			mod := &m.nav.modules[i]
			if mod.id != sel.moduleID {
				continue
			}
			m.ws.tabs = mod.layers
			for _, l := range mod.layers {
				if l.super {
					m.ws.needsRoot = true
				}
			}
		}
	}
	if sel.layer != "" {
		m.ws.placeholder = sel.moduleID + " · " + sel.label
		for i, t := range m.ws.tabs {
			if t.layer == sel.layer && t.dir == sel.dir {
				m.ws.active = i
			}
		}
	} else {
		m.ws.placeholder = sel.moduleID
		m.ws.active = 0
	}
	m.attachDraft()
	return m.loadLayer()
}

// attachDraft points the workspace at the active layer's draft (if any)
// and re-renders from it — drafts survive navigation and layer switches
// because the store is keyed by file, not by selection.
func (m *Compositor) attachDraft() {
	d := m.store[m.ws.activeDir()]
	m.ws.draft = d
	m.ws.editing = nil
	switch {
	case d == nil:
		// Rows already reflect the landed read (or the placeholder).
	case d.cfg != nil:
		m.ws.schemaErr = nil
		m.ws.rows = wsRows(d.cfg, m.ws.needsRoot)
	default: // raw-mode draft: the file is still broken
		m.ws.rows = rawRows(d.raw)
	}
	if m.ws.cursor >= len(m.ws.rows) {
		m.ws.cursor = max(0, len(m.ws.rows)-1)
	}
}

// wsDraftFor returns the draft for a layer dir, if any.
func (m *Compositor) wsDraftFor(dir string) *wsDraft { return m.store[dir] }

// draftMarks derives the nav/header dirty marker set from the store.
func (m *Compositor) draftMarks() map[string]bool {
	if len(m.store) == 0 {
		return nil
	}
	marks := make(map[string]bool, len(m.store))
	for dir := range m.store {
		marks[dir] = true
	}
	return marks
}

// loadLayer schedules a read of the active layer file through the 0065
// seam. No reader (nil layerFor): the workspace keeps its placeholder.
func (m *Compositor) loadLayer() tea.Cmd {
	dir := m.ws.activeDir()
	if m.layerFor == nil || dir == "" {
		return nil
	}
	if dir == m.ws.loadedDir {
		return nil // already showing this file
	}
	m.ws.pending = true
	return func() tea.Msg {
		if m.reader == nil {
			m.reader = m.layerFor(m.facts)
		}
		r, err := m.reader.ReadModuleLayer(dir)
		return layerLoadedMsg{dir: dir, read: r, err: err}
	}
}

// quit implements q: a dirty draft ledger asks first; a clean shell
// quits directly. During an apply the footer redirects to the detail
// modal's ctrl+c (quitting mid-run is the cancel flow, not a shortcut).
func (m *Compositor) quit() tea.Cmd {
	if m.applying {
		m.message, m.msgErr = "apply running — a opens the detail, ctrl+c cancels", false
		return nil
	}
	if len(m.store) == 0 {
		return tea.Quit
	}
	m.modals = append(m.modals, &confirmModel{
		th:    m.th,
		title: "quit with unsaved drafts?",
		body: []string{
			strconv.Itoa(len(m.store)) + " file(s) with unsaved changes",
			"drafts live in memory only — quitting loses them",
		},
		onAnswer: func(ok bool) {
			if ok {
				m.afterPop = func() tea.Cmd { return tea.Quit }
			}
		},
	})
	return nil
}

func (m *Compositor) spinTick() tea.Cmd {
	return tea.Tick(spinInterval, func(time.Time) tea.Msg { return spinTickMsg{} })
}

// ShortHelp implements help.KeyMap: the hints render from the binding
// table (edit mode gets the edit keys), so the footer cannot drift.
func (m *Compositor) ShortHelp() []key.Binding {
	if m.ws.editing != nil {
		return []key.Binding{
			key.NewBinding(key.WithKeys("enter"), key.WithHelp("enter", "commit")),
			key.NewBinding(key.WithKeys("esc"), key.WithHelp("esc", "cancel")),
			key.NewBinding(key.WithKeys("?"), key.WithHelp("?", "help")),
		}
	}
	pane := "nav"
	if m.focus == focusWork {
		pane = "work"
	}
	var out []key.Binding
	add := func(k, h string) {
		out = append(out, key.NewBinding(key.WithKeys(k), key.WithHelp(k, h)))
	}
	seen := map[string]bool{}
	for _, b := range keyTable() {
		if b.pane == pane && !seen[b.help] {
			seen[b.help] = true
			add(b.key, b.help)
			if len(out) >= 3 {
				break
			}
		}
	}
	for _, k := range []struct{ key, help string }{{"/", "palette"}, {"?", "help"}, {"q", "quit"}} {
		add(k.key, k.help)
	}
	return out
}

// FullHelp implements help.KeyMap.
func (m *Compositor) FullHelp() [][]key.Binding { return [][]key.Binding{m.ShortHelp()} }

func (m *Compositor) View() tea.View {
	var content string
	if m.w < minWidth || m.h < minHeight {
		content = m.th.viewTitle.Render(" terminal too small for the shell (need 60x12)")
	} else if len(m.modals) > 0 {
		content = m.compositedFrame()
	} else {
		content = m.baseFrame()
	}
	v := tea.NewView(content)
	v.AltScreen = true
	return v
}

// compositedFrame renders the dimmed base with the top modal centered over
// it — the golden-pinned final frame.
func (m *Compositor) compositedFrame() string {
	dimmed := m.th.dimBase.Render(stripANSIRe.ReplaceAllString(m.baseFrame(), ""))
	box := m.modals[len(m.modals)-1].view(m.w, m.h)
	x := max((m.w-lipgloss.Width(box))/2, 0)
	y := max((m.h-lipgloss.Height(box))/2, 0)
	canvas := lipgloss.NewCanvas(max(m.w, 1), max(m.h, 1))
	canvas.Compose(lipgloss.NewCompositor(
		lipgloss.NewLayer(dimmed),
		lipgloss.NewLayer(box).X(x).Y(y),
	))
	return canvas.Render()
}

// baseFrame is header + panes + footer at the current size. In lipgloss
// v2 a style's Width/Height include the border, so pane content gets the
// outer dimensions minus two — a view handed the outer size overflows
// the box (the workspace-all golden caught this).
func (m *Compositor) baseFrame() string {
	navW, workW, paneH := m.layout()
	left := m.pane(m.focus == focusNav, navW-2, paneH, m.nav.view(navW-4, paneH-2, m.th, m.draftMarks()))
	right := m.pane(m.focus == focusWork, workW-2, paneH, m.ws.view(workW-4, paneH-2, m.th))
	return lipgloss.JoinVertical(lipgloss.Left,
		m.headerView(),
		lipgloss.JoinHorizontal(lipgloss.Top, left, right),
		m.footerView(),
	)
}

// layout returns the nav and workspace total widths and the pane content
// height: header 1 line, footer 2 lines, pane borders 2 rows.
func (m *Compositor) layout() (navW, workW, paneH int) {
	navW = m.w * 3 / 10
	navW = min(max(navW, 22), 44)
	return navW, m.w - navW, m.h - 5
}

func (m *Compositor) pane(focused bool, w, h int, content string) string {
	st := m.th.unfocusedBorder
	if focused {
		st = m.th.focusedBorder
	}
	return st.Width(w).Height(h).Render(content)
}

func (m *Compositor) headerView() string {
	s := m.th.headerTitle.Render(" dotdrift") +
		m.th.headerContext.Render(" · "+m.root)
	if m.host != "" {
		s += m.th.headerContext.Render(" · host " + m.host + " · user " + m.user)
	}
	if len(m.store) > 0 {
		s += m.th.dirtyMark.Render("  ● " + strconv.Itoa(len(m.store)))
	}
	if m.applying {
		s += m.th.applyBadge.Render("  ▶ apply")
	}
	return s
}

func (m *Compositor) footerView() string {
	var slot string
	switch {
	case m.op != "":
		slot = " " + spinFrames[m.spinIdx%len(spinFrames)] + " " + m.op
	case m.message != "" && m.msgErr:
		slot = m.th.errorMark.Render(" " + m.message)
	case m.message != "":
		slot = m.th.statusBar.Render(" " + m.message)
	default:
		slot = " "
	}
	return slot + "\n" + m.th.statusBar.Render(" "+m.help.View(m))
}
