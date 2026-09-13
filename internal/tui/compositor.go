package tui

// The compositor (M15, issue 0073): one full-screen base shell — header,
// nav, workspace, footer — plus a stack of modal layers over it. The top
// layer owns all input while open; the base keeps rendering underneath,
// dimmed. esc is owned here, not by individual views: it pops the top
// layer — modal first, then edit mode, then focus back to the nav. This
// kills the M14 per-view input fallthrough class of bug by construction.
// The old view stack (shell.go) stays alive until T-tui-cleanup.

import (
	"regexp"
	"strconv"
	"time"

	"charm.land/bubbles/v2/help"
	"charm.land/bubbles/v2/key"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
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

var keyPane = key.NewBinding(key.WithKeys("tab"), key.WithHelp("tab", "pane"))

// Compositor is the M15 shell model. Placeholder pane content
// (navText/workText) stands in until T-tui-nav and T-tui-workspace.
type Compositor struct {
	th     theme
	isDark bool
	w, h   int

	navText  string
	workText string
	focus    paneFocus
	editing  bool // workspace inline edit mode; T-tui-editing drives it

	modals []modal

	// Header chrome.
	root     string
	host     string
	user     string
	dirty    int
	applying bool

	// Footer chrome.
	op       string
	spinIdx  int
	message  string
	msgErr   bool
	msgToken int

	help help.Model
}

// NewCompositor builds the M15 shell over a profile root. Experimental
// until it reaches parity with the M14 shell (T-tui-cleanup).
func NewCompositor(root string) *Compositor {
	return &Compositor{
		th:     newTheme(true), // corrected by BackgroundColorMsg
		root:   root,
		help:   help.New(),
		isDark: true,
	}
}

// Run starts the program full-screen (altscreen is set on every View).
func (m *Compositor) Run() error {
	_, err := tea.NewProgram(m).Run()
	return err
}

func (m *Compositor) Init() tea.Cmd { return nil }

func (m *Compositor) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.w, m.h = msg.Width, msg.Height
		return m, nil
	case tea.BackgroundColorMsg:
		m.isDark = msg.IsDark()
		m.th = newTheme(m.isDark)
		return m, nil
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

	// The modal stack owns all other input; esc pops the top layer.
	if len(m.modals) > 0 {
		if k, ok := msg.(tea.KeyPressMsg); ok && k.String() == "esc" {
			m.modals = m.modals[:len(m.modals)-1]
			return m, nil
		}
		return m, m.modals[len(m.modals)-1].update(msg)
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
	switch msg.String() {
	case "tab", "shift+tab":
		m.focus = (m.focus + 1) % 2 // two panes: reverse is forward
	case "esc":
		if m.editing {
			m.editing = false
		} else if m.focus != focusNav {
			m.focus = focusNav
		}
	}
	return m, nil
}

func (m *Compositor) spinTick() tea.Cmd {
	return tea.Tick(spinInterval, func(time.Time) tea.Msg { return spinTickMsg{} })
}

// ShortHelp implements help.KeyMap; hints follow the focused pane.
func (m *Compositor) ShortHelp() []key.Binding {
	if m.focus == focusWork {
		return []key.Binding{keyScroll, keyPane, keyHelp, keyQuit}
	}
	return []key.Binding{keyUp, keyDown, keyOpen, keyPane, keyHelp, keyQuit}
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

// baseFrame is header + panes + footer at the current size.
func (m *Compositor) baseFrame() string {
	navW, workW, paneH := m.layout()
	left := m.pane(m.focus == focusNav, navW-2, paneH, m.navText)
	right := m.pane(m.focus == focusWork, workW-2, paneH, m.workText)
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
	if m.dirty > 0 {
		s += m.th.dirtyMark.Render("  ● " + strconv.Itoa(m.dirty))
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
