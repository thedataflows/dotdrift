package tui

import (
	"context"
	"fmt"
	"path/filepath"
	"strings"

	"charm.land/bubbles/v2/help"
	"charm.land/bubbles/v2/key"
	"charm.land/bubbles/v2/tree"
	"charm.land/bubbles/v2/viewport"
	"charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/thedataflows/dotdrift/internal/drift"
	"github.com/thedataflows/dotdrift/internal/facts"
	"github.com/thedataflows/dotdrift/internal/profile"
	"github.com/thedataflows/dotdrift/internal/resolve"
	"github.com/thedataflows/dotdrift/internal/service"
	"github.com/thedataflows/dotdrift/internal/tui/editor"
)

// The two-pane shell (T-tui-shell, prototype approved as issue 0063):
// tree left, one active view right, one focused pane (focus = border
// color), the 0062-D5 key baseline, one-line header, status-bar help
// hints. The shell owns navigation, focus, chrome, and the view stack;
// view bodies render through the reads areas (views.go) and never block
// on slow reads — plan/status/module-resolution load async into
// placeholders.

// Reads is the consumer-side narrow interface over the service reads
// areas (ADR-0008's doorway): exactly what the shell needs, satisfied
// implicitly by *service.ReadsArea.
type Reads interface {
	Modules(profilePath string, modules []string) (*service.ModulesRead, error)
	Plan(profilePath string, modules []string, f *facts.Facts) (*service.PlanRead, error)
	Status(ctx context.Context, opts service.StatusOpts) (*service.StatusRead, error)
	Diff(plan *resolve.Plan, profileRoot string) ([]service.DiffEntry, error)
	ModuleConfigAt(dir string) (*profile.ModuleConfig, error)
}

// Options carries the shell's construction inputs: where the profile
// lives, the probe seam the status view needs (the caller's sudo /
// backend seams, same shape as the CLI adapter's), the config-area
// factory the editor suite opens drafts through, the writes factory
// the Profile dialogs run through, and the apply-area factory the plan
// gate previews and starts sessions through — the facts arrive with the
// first read, so the factories are called lazily.
type Options struct {
	ProfilePath string
	StatePath   string
	ProbesFor   func(*facts.Facts) drift.Probes
	ConfigFor   func(*facts.Facts) service.ConfigEditor
	WritesFor   func(*facts.Facts) Writes
	ApplyFor    func(*facts.Facts) ApplyLauncher
}

type focus int

const (
	focusTree focus = iota
	focusMain
)

const (
	minWidth  = 60
	minHeight = 12
)

// Async load results. Each carries the view it belongs to; a result whose
// view has left the stack is dropped (stale).
type (
	modulesLoadedMsg struct {
		read *service.ModulesRead
		err  error
	}
	modulePlanLoadedMsg struct {
		id  viewID
		pr  *service.PlanRead
		err error
	}
	statusLoadedMsg struct {
		id  viewID
		r   *service.StatusRead
		err error
	}
	planLoadedMsg struct {
		id   viewID
		pr   *service.PlanRead
		diff []service.DiffEntry
		err  error
	}
)

// Key registry (0062-D5): every visible binding lives here; help renders
// itself from it.
var (
	keySwitchPane = key.NewBinding(
		key.WithKeys("tab", "shift+tab"),
		key.WithHelp("tab", "switch pane"))
	keyQuit   = key.NewBinding(key.WithKeys("q"), key.WithHelp("q", "quit"))
	keyHelp   = key.NewBinding(key.WithKeys("?"), key.WithHelp("?", "help"))
	keyBack   = key.NewBinding(key.WithKeys("esc"), key.WithHelp("esc", "back"))
	keyRaw    = key.NewBinding(key.WithKeys("r"), key.WithHelp("r", "raw/resolved"))
	keyUp     = key.NewBinding(key.WithKeys("up", "k"), key.WithHelp("k", "up"))
	keyDown   = key.NewBinding(key.WithKeys("down", "j"), key.WithHelp("j", "down"))
	keyOpen   = key.NewBinding(key.WithKeys("enter"), key.WithHelp("enter", "open"))
	keyJump   = key.NewBinding(key.WithKeys("g", "G"), key.WithHelp("g/G", "top/bottom"))
	keyScroll = key.NewBinding(key.WithKeys("j", "k"), key.WithHelp("j/k", "scroll"))
)

// Shell is the whole program model.
type Shell struct {
	area Reads
	opts Options

	ready bool
	read  *service.ModulesRead
	roots []branch // the derived tree model

	tree    tree.Model
	vp      viewport.Model
	help    help.Model
	stack   viewStack
	focus   focus
	confirm bool // quit confirmation showing

	th     theme
	isDark bool

	w, h        int
	leftW       int
	rightW      int
	paneH       int
	walking     bool // cursor reselection in progress (suppress view sync)
	modulePlans map[string]*service.PlanRead
	frames      map[string]*editor.Frame // open editor frames, keyed by module dir
	cfg         editor.Config            // built lazily via opts.ConfigFor
	dialogs     map[string]dialog        // Profile dialogs, keyed by action
	writes      Writes                   // built lazily via opts.WritesFor
	apply       *applyModel              // the apply mode replacing the main pane (nil = off)
	applier     ApplyLauncher            // built lazily via opts.ApplyFor
	send        func(tea.Msg)            // the program's Send, captured in Run
}

// New builds the shell; Init kicks off the modules read.
func New(area Reads, opts Options) *Shell {
	m := &Shell{
		area:        area,
		opts:        opts,
		isDark:      true, // corrected by BackgroundColorMsg once reported
		modulePlans: map[string]*service.PlanRead{},
		frames:      map[string]*editor.Frame{},
	}
	m.th = newTheme(m.isDark)
	m.help = help.New()
	m.tree = tree.New(tree.Root(treeItem{kind: kindGroup}), 30, 10)
	m.tree.SetShowHelp(false) // the status bar owns help
	m.vp = viewport.New(viewport.WithWidth(60), viewport.WithHeight(20))
	return m
}

// Run starts the bubbletea program for this shell (the cmd adapter's
// runner seam calls it). The program's Send is captured for the apply
// mode's drain goroutine — out-of-band messages ride it.
func (m *Shell) Run() error {
	p := tea.NewProgram(m)
	m.send = p.Send
	_, err := p.Run()
	return err
}

func (m *Shell) Init() tea.Cmd {
	return func() tea.Msg {
		r, err := m.area.Modules(m.opts.ProfilePath, nil)
		return modulesLoadedMsg{read: r, err: err}
	}
}

func (m *Shell) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	// Apply mode messages: everything the apply model absorbs or answers
	// (the handover message's answer is the tea.ExecProcess cmd).
	switch msg.(type) {
	case applyPreviewMsg, applyStartedMsg, applyEventMsg, applyTickMsg,
		applyWaitedMsg, applyHandoverMsg, applyHandoverDoneMsg:
		if m.apply == nil {
			return m, nil
		}
		return m, m.apply.update(msg)
	}

	switch msg := msg.(type) {
	case modulesLoadedMsg:
		if msg.err != nil {
			m.ready = true
			m.read = &service.ModulesRead{Profile: nil}
			m.syncViewport()
			return m, nil
		}
		m.ready = true
		m.read = msg.read
		m.buildTree()
		m.placeCursorOnFirstModule()
		cmd := m.syncSelection()
		return m, cmd

	case modulePlanLoadedMsg:
		return m, m.applyLoaded(msg.id, func(v view) view {
			v.loading = false
			if msg.err != nil {
				v.content = renderLoadError(m.th, v.title, msg.err)
			} else {
				m.modulePlans[msg.id.key] = msg.pr
				v.content = renderModuleResolved(m, msg.pr, msg.id.key)
			}
			return v
		})

	case statusLoadedMsg:
		return m, m.applyLoaded(msg.id, func(v view) view {
			v.loading = false
			if msg.err != nil {
				v.content = renderLoadError(m.th, v.title, msg.err)
			} else {
				v.content = renderStatusView(m.th, msg.r)
			}
			return v
		})

	case planLoadedMsg:
		return m, m.applyLoaded(msg.id, func(v view) view {
			v.loading = false
			if msg.err != nil {
				v.content = renderLoadError(m.th, v.title, msg.err)
			} else {
				v.content = renderPlanView(m, msg.pr, msg.diff)
			}
			return v
		})

	case tea.BackgroundColorMsg:
		m.isDark = msg.IsDark()
		m.th = newTheme(m.isDark)
		m.restyleTree()
		return m, nil

	case tea.WindowSizeMsg:
		m.setSize(msg.Width, msg.Height)
		return m, nil

	case tea.KeyPressMsg:
		return m.handleKey(msg)

	case writeFinishedMsg:
		if d, ok := m.dialogs[msg.key]; ok {
			d.applyFinished(msg)
			m.syncDialog(msg.key)
		}
		return m, nil

	case restorePlanMsg:
		if d, ok := m.dialogs["restore"].(*restoreDialog); ok && msg.key == "restore" {
			d.applyPlan(msg)
			m.syncDialog("restore")
		}
		return m, nil
	}
	return m, nil
}

// handleKey is the 0062-D5 vocabulary. Global keys are reserved by the
// shell; everything else goes to the focused pane (exactly one focused).
// The apply mode, when on, owns the main pane's keys: the shell's
// globals stay reserved above it, except quit — during a running apply,
// q and ctrl+c route into the cancel gate (the explicit exit; killing
// the program would orphan the session and its children).
func (m *Shell) handleKey(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	s := msg.String()

	// The quit confirmation owns the keyboard while showing.
	if m.confirm {
		switch s {
		case "y", "ctrl+c":
			return m, tea.Quit
		case "n", "esc", "q":
			m.confirm = false
		}
		return m, nil
	}

	if m.apply != nil {
		switch s {
		case "tab", "shift+tab", "?":
			// shell globals — handled below
		case "ctrl+c":
			if m.apply.phase == applyRunning {
				m.apply.handleKey("x") // into the cancel gate, never a kill
				return m, nil
			}
			return m, tea.Quit
		default:
			var cmd tea.Cmd
			if m.focus == focusMain {
				if s == "q" && m.apply.phase == applyRunning {
					m.apply.handleKey("x")
				} else {
					cmd = m.apply.handleKey(s)
				}
				m.dropApplyIfClosed()
				return m, cmd
			}
		}
	}

	// A Profile dialog on top owns the main pane's keys: the dialog's
	// form vocabulary runs first; the shell's globals stay reserved
	// above it (same discipline as the editor frames).
	if top := m.stack.top(); top.id.kind == viewAction && m.focus == focusMain {
		switch s {
		case "tab", "shift+tab", "?", "q", "ctrl+c":
			// handled by the shell below
		default:
			if d := m.dialogs[top.id.key]; d != nil {
				cmd := d.HandleKey(s)
				m.syncDialog(top.id.key)
				return m, cmd
			}
		}
	}

	// An editor view on top owns the main pane's keys: the frame's
	// vocabulary (sections, fields, save, dirty-confirm) runs first; the
	// shell's globals (tab, ?, q, ctrl+c) stay reserved above it.
	if top := m.stack.top(); top.id.kind == viewEditor && m.focus == focusMain {
		switch s {
		case "tab", "shift+tab", "?", "q", "ctrl+c":
			// handled by the shell below
		default:
			frame := m.editorFrameFor(top.id.key)
			if frame != nil {
				frame.HandleKey(s)
				m.syncEditorView(top.id.key)
				if frame.PopRequested() {
					return m.escBack()
				}
				return m, nil
			}
		}
	}

	switch s {
	case "ctrl+c":
		return m, tea.Quit
	case "q":
		if m.stack.dirtyAnywhere() {
			m.confirm = true
			return m, nil
		}
		return m, tea.Quit
	case "tab", "shift+tab":
		if m.focus == focusTree {
			m.focus = focusMain
		} else {
			m.focus = focusTree
		}
		return m, nil
	case "?":
		m.help.ShowAll = !m.help.ShowAll
		return m, nil
	case "esc":
		return m.escBack()
	case "r":
		return m.toggleRaw()
	case "e":
		return m.openEditor()
	case "a":
		return m, m.openApply()
	}

	if m.focus == focusMain {
		switch s {
		case "j", "down":
			m.vp.ScrollDown(1)
			return m, nil
		case "k", "up":
			m.vp.ScrollUp(1)
			return m, nil
		}
		var cmd tea.Cmd
		m.vp, cmd = m.vp.Update(msg)
		return m, cmd
	}

	// Tree focus.
	switch s {
	case "j", "down":
		m.tree.Down()
	case "k", "up":
		m.tree.Up()
	case "g":
		m.tree.GoToTop()
	case "G":
		m.tree.GoToBottom()
	case "enter":
		m.tree.ToggleCurrentNode()
	default:
		var cmd tea.Cmd
		m.tree, cmd = m.tree.Update(msg)
		if cmd != nil {
			return m, cmd
		}
	}
	if m.walking {
		return m, nil
	}
	return m, m.syncSelection()
}

// escBack pops the view stack: first it un-raws a module view (back to the
// resolved stack), else it pops and walks the tree cursor back to the
// restored view's node. Popping the bottom view is a no-op.
func (m *Shell) escBack() (tea.Model, tea.Cmd) {
	top := m.stack.top()
	if top.id.kind == viewModule && top.raw {
		m.showResolved(top)
		return m, nil
	}
	if m.stack.len() <= 1 {
		return m, nil
	}
	m.stack.pop()
	restored := m.stack.top()
	m.syncViewport()
	m.walkTo(restored.item)
	return m, nil
}

// toggleRaw flips a module view between the resolved overlay stack and
// the raw per-layer declarations (0062-D3). Re-rendering uses the cached
// plan read when one has landed.
func (m *Shell) toggleRaw() (tea.Model, tea.Cmd) {
	top := m.stack.top()
	if top.id.kind != viewModule {
		return m, nil
	}
	if !top.raw {
		top.raw = true
		top.content = renderModuleRaw(m, top.item)
		m.stack.setTop(top)
		m.syncViewport()
		return m, nil
	}
	m.showResolved(top)
	return m, nil
}

// openEditor opens the editor frame for the selected origin's module.toml
// (0065-D2: editors open only from the raw overlay-stack node — the tree
// position IS the layer choice, and the editor chrome names the file).
func (m *Shell) openEditor() (tea.Model, tea.Cmd) {
	top := m.stack.top()
	if top.id.kind != viewOrigin && top.item.kind != kindOrigin {
		return m, nil
	}
	dir := top.item.dir
	if dir == "" {
		return m, nil
	}
	frame := m.editorFrameFor(dir)
	if frame == nil {
		return m, nil
	}
	v := view{
		id:      viewID{kind: viewEditor, key: dir},
		title:   "EDIT " + top.item.moduleID,
		item:    top.item,
		dirty:   frame.Dirty(),
		content: frame.View(),
	}
	m.stack.open(v)
	m.syncViewport()
	return m, nil
}

// openApply arms the apply mode from the plan view — the TUI's only
// door to convergence (0062-D10). The mode replaces the main pane; the
// gate's classification load starts immediately. A nil ApplyFor keeps
// the door closed.
func (m *Shell) openApply() tea.Cmd {
	top := m.stack.top()
	if top.id.kind != viewPlan || m.apply != nil || m.opts.ApplyFor == nil {
		return nil
	}
	if m.applier == nil {
		var f *facts.Facts
		if m.read != nil {
			f = m.read.Facts
		}
		m.applier = m.opts.ApplyFor(f)
	}
	if m.applier == nil {
		return nil
	}
	send := m.send
	if send == nil {
		send = func(tea.Msg) {}
	}
	m.apply = newApplyModel(m.applier, m.opts.ProfilePath, m.opts.StatePath, send, m.th)
	m.focus = focusMain
	return m.apply.loadPreview()
}

// dropApplyIfClosed tears the mode down once the model asked to leave
// (gate declined, ended acknowledged) — the underlying view returns.
func (m *Shell) dropApplyIfClosed() {
	if m.apply != nil && m.apply.closeRequested() {
		m.apply = nil
		m.syncViewport()
	}
}

// editorFrameFor returns the open frame for a module dir, opening one
// lazily through the config-area factory (the facts have landed by the
// time an editor can be opened — the first read populated them).
func (m *Shell) editorFrameFor(dir string) *editor.Frame {
	if f, ok := m.frames[dir]; ok {
		return f
	}
	if m.cfg == nil && m.opts.ConfigFor != nil {
		var f *facts.Facts
		if m.read != nil {
			f = m.read.Facts
		}
		m.cfg = m.opts.ConfigFor(f)
	}
	if m.cfg == nil {
		return nil
	}
	frame := editor.NewFrame(m.cfg, m.th, dir)
	m.frames[dir] = frame
	return frame
}

// syncEditorView re-renders the open editor view (content + dirty mark).
func (m *Shell) syncEditorView(dir string) {
	frame := m.frames[dir]
	if frame == nil {
		return
	}
	for i := len(m.stack.stack) - 1; i >= 0; i-- {
		if m.stack.stack[i].id.kind == viewEditor && m.stack.stack[i].id.key == dir {
			m.stack.stack[i].dirty = frame.Dirty()
			m.stack.stack[i].content = frame.View()
			if i == len(m.stack.stack)-1 {
				m.syncViewport()
			}
			return
		}
	}
}

// dialogFor returns the Profile dialog for an action, building it
// lazily through the writes factory (the facts have landed by the time
// an action can be opened — the first read populated them). A nil
// WritesFor keeps the disabled stub views.
func (m *Shell) dialogFor(action string) dialog {
	if m.opts.WritesFor == nil {
		return nil
	}
	if m.dialogs == nil {
		m.dialogs = map[string]dialog{}
	}
	if d, ok := m.dialogs[action]; ok {
		return d
	}
	if m.writes == nil {
		var f *facts.Facts
		if m.read != nil {
			f = m.read.Facts
		}
		m.writes = m.opts.WritesFor(f)
	}
	var d dialog
	switch action {
	case "onboard":
		d = newOnboardDialog(m.writes, m.opts.ProfilePath)
	case "restore":
		d = newRestoreDialog(m.writes, m.opts.ProfilePath, m.restoreIndex)
	case "generate":
		d = newGenerateDialog(m.writes, m.opts.ProfilePath)
	default:
		return nil
	}
	m.dialogs[action] = d
	return d
}

// restoreIndex reads the profile's backup index for the restore dialog's
// generation picking (a pure service read over the already-loaded
// profile).
func (m *Shell) restoreIndex() (map[string]map[string][]service.RestoreHit, error) {
	if m.read == nil || m.read.Profile == nil {
		return nil, fmt.Errorf("restore: the profile is not loaded yet")
	}
	return service.IndexBackups(service.ModuleLayers(m.read.Profile)), nil
}

// syncDialog re-renders a dialog's stacked view (its form, confirm gate,
// or report).
func (m *Shell) syncDialog(action string) {
	d := m.dialogs[action]
	if d == nil {
		return
	}
	for i := len(m.stack.stack) - 1; i >= 0; i-- {
		v := m.stack.stack[i]
		if v.id.kind == viewAction && v.id.key == action {
			m.stack.stack[i].content = d.View(m.th)
			if i == len(m.stack.stack)-1 {
				m.syncViewport()
			}
			return
		}
	}
}

// showResolved restores a module view's resolved rendering from the
// cached plan read (loading placeholder if the read hasn't landed).
func (m *Shell) showResolved(v view) {
	v.raw = false
	if pr, ok := m.modulePlans[v.id.key]; ok {
		v.content = renderModuleResolved(m, pr, v.id.key)
	} else {
		v.loading = true
		v.content = renderLoading(m.th, v.title, "resolving overlay stack…")
	}
	m.stack.setTop(v)
	m.syncViewport()
}

// walkTo moves the tree cursor onto target's node without triggering
// selection syncs (the esc-pop path): search upward from the cursor, then
// downward from the top.
func (m *Shell) walkTo(target treeItem) {
	m.walking = true
	defer func() { m.walking = false }()
	bound := len(m.allItems()) + 1

	// Upward from the current cursor.
	for i := 0; i < bound; i++ {
		if itemEq(m.selectedItem(), target) {
			return
		}
		before := m.tree.NodeAtCurrentOffset()
		m.tree.Up()
		if m.tree.NodeAtCurrentOffset() == before {
			break // top reached
		}
	}
	if itemEq(m.selectedItem(), target) {
		return
	}
	// Downward from the top.
	m.tree.GoToTop()
	for i := 0; i < bound; i++ {
		if itemEq(m.selectedItem(), target) {
			return
		}
		before := m.tree.NodeAtCurrentOffset()
		m.tree.Down()
		if m.tree.NodeAtCurrentOffset() == before {
			return
		}
	}
}

// setSize lays out header (1) + bordered panes + status (1); border cells
// are part of the budget (research 0058: frames eat two cells per edge).
func (m *Shell) setSize(w, h int) {
	m.w, m.h = w, h
	left := w * 3 / 10
	if left < 22 {
		left = 22
	}
	if left > 44 {
		left = 44
	}
	m.leftW = left
	m.rightW = w - left - 4 // two borders, two cells each
	m.paneH = h - 6         // header 1 + status 1 + borders 4
	m.tree.SetSize(m.leftW-2, m.paneH)
	m.vp.SetWidth(m.rightW - 2)
	m.vp.SetHeight(m.paneH)
	m.syncViewport()
}

// buildTree derives the tree model and rebuilds the bubbles tree.
func (m *Shell) buildTree() {
	m.roots = buildTree(m.read)
	m.restyleTree()
}

// restyleTree re-applies registry styles (initial build + background
// changes).
func (m *Shell) restyleTree() {
	label := "profile"
	if m.opts.ProfilePath != "" {
		label = filepath.Base(m.opts.ProfilePath)
	}
	root := tree.Root(treeItem{kind: kindGroup, label: label})
	for _, b := range m.roots {
		root.Child(branchNode(b))
	}
	m.tree.SetNodes(root)
	m.tree.SetStyles(m.th.treeStyles())
}

func branchNode(b branch) *tree.Node {
	n := tree.Root(b.item)
	for _, k := range b.kids {
		n.Child(branchNode(k))
	}
	return n
}

// placeCursorOnFirstModule walks down to the first module node — the
// shell opens on a module, not on the tree root.
func (m *Shell) placeCursorOnFirstModule() {
	m.walking = true
	defer func() { m.walking = false }()
	for range 4 * len(m.allItems()) {
		if it := m.selectedItem(); it.kind == kindModule {
			return
		}
		before := m.tree.NodeAtCurrentOffset()
		m.tree.Down()
		if m.tree.NodeAtCurrentOffset() == before {
			return
		}
	}
}

func (m *Shell) allItems() []treeItem {
	nodes := m.tree.Root().AllNodes()
	items := make([]treeItem, 0, len(nodes))
	for _, n := range nodes {
		if it, ok := n.GivenValue().(treeItem); ok {
			items = append(items, it)
		}
	}
	return items
}

func (m *Shell) selectedItem() treeItem {
	n := m.tree.NodeAtCurrentOffset()
	if n == nil {
		return treeItem{kind: kindGroup}
	}
	if it, ok := n.GivenValue().(treeItem); ok {
		return it
	}
	return treeItem{kind: kindGroup, label: n.Value()}
}

// itemEq compares the identity-relevant fields of two items (labels alone
// are ambiguous: every module has a "base" origin).
func itemEq(a, b treeItem) bool {
	return a.kind == b.kind && a.label == b.label && a.moduleID == b.moduleID &&
		a.origin == b.origin && a.dir == b.dir && a.action == b.action &&
		a.acctKind == b.acctKind && a.acctName == b.acctName
}

// syncSelection opens the selected node's view (resolved-by-default,
// 0062-D3) and returns any async load it schedules. Group nodes are
// navigation only — they leave the view stack untouched.
func (m *Shell) syncSelection() tea.Cmd {
	it := m.selectedItem()
	if it.kind == kindGroup {
		return nil
	}
	v, cmd := m.viewFor(it)
	m.stack.open(v)
	m.syncViewport()
	return cmd
}

// viewFor builds the view bound to a tree item: synchronous views render
// now; slow reads (resolve, drift probes) render as placeholders and load
// async.
func (m *Shell) viewFor(it treeItem) (view, tea.Cmd) {
	switch it.kind {
	case kindModule:
		v := view{id: viewID{kind: viewModule, key: it.moduleID}, title: "MODULE " + it.moduleID, item: it}
		if it.reason != "" {
			v.content = renderSkipNotice(m.th, v.title, it.reason)
			return v, nil
		}
		v.loading = true
		v.content = renderLoading(m.th, v.title, "resolving overlay stack…")
		return v, m.loadModulePlan(v.id)
	case kindOrigin:
		v := view{id: viewID{kind: viewOrigin, key: it.dir}, title: "RAW " + it.origin, item: it}
		v.content = renderOriginView(m, it) + "\n" + m.th.meta.Render("e edits this layer's module.toml")
		return v, nil
	case kindAccount:
		v := view{id: viewID{kind: viewAccount, key: it.acctKind + "/" + it.acctName}, title: accountTitle(it), item: it}
		v.content = renderAccountView(m, it)
		return v, nil
	case kindView:
		switch it.view {
		case viewStatus:
			v := view{id: viewID{kind: viewStatus}, title: "STATUS", item: it}
			v.loading = true
			v.content = renderLoading(m.th, v.title, "probing the live system…")
			return v, m.loadStatus(v.id)
		case viewPlan:
			v := view{id: viewID{kind: viewPlan}, title: "PLAN", item: it}
			v.loading = true
			v.content = renderLoading(m.th, v.title, "resolving the plan…")
			return v, m.loadPlan(v.id)
		}
	case kindAction:
		v := view{id: viewID{kind: viewAction, key: it.action}, title: strings.ToUpper(it.action), item: it}
		if d := m.dialogFor(it.action); d != nil {
			v.content = d.View(m.th)
		} else {
			v.content = renderActionStub(m.th, it.action)
		}
		return v, nil
	}
	v := view{id: viewID{kind: viewGroup, key: it.label}, title: it.label, item: it}
	v.content = renderHint(m.th, "Select a module, account, or profile entry.")
	return v, nil
}

func (m *Shell) loadModulePlan(id viewID) tea.Cmd {
	f := m.read.Facts
	return func() tea.Msg {
		pr, err := m.area.Plan(m.opts.ProfilePath, []string{id.key}, f)
		return modulePlanLoadedMsg{id: id, pr: pr, err: err}
	}
}

func (m *Shell) loadStatus(id viewID) tea.Cmd {
	return func() tea.Msg {
		probesFor := m.opts.ProbesFor
		if probesFor == nil {
			probesFor = func(*facts.Facts) drift.Probes { return drift.Probes{} }
		}
		r, err := m.area.Status(context.Background(), service.StatusOpts{
			ProfilePath: m.opts.ProfilePath,
			StatePath:   m.opts.StatePath,
			ProbesFor:   probesFor,
		})
		return statusLoadedMsg{id: id, r: r, err: err}
	}
}

func (m *Shell) loadPlan(id viewID) tea.Cmd {
	f := m.read.Facts
	return func() tea.Msg {
		pr, err := m.area.Plan(m.opts.ProfilePath, nil, f)
		if err != nil {
			return planLoadedMsg{id: id, err: err}
		}
		diff, err := m.area.Diff(pr.Plan, mustAbs(m.read.Profile.Root))
		return planLoadedMsg{id: id, pr: pr, diff: diff, err: err}
	}
}

// applyLoaded fills the view a load result belongs to, wherever it sits in
// the stack; stale results (view gone) are dropped. Returns whether the
// viewport needs a refresh — as a cmd for Update's return slot.
func (m *Shell) applyLoaded(id viewID, fill func(view) view) tea.Cmd {
	for i := len(m.stack.stack) - 1; i >= 0; i-- {
		if m.stack.stack[i].id == id {
			m.stack.stack[i] = fill(m.stack.stack[i])
			if i == len(m.stack.stack)-1 {
				m.syncViewport()
			}
			return nil
		}
	}
	return nil
}

// syncViewport re-feeds the active view's content to the viewport.
func (m *Shell) syncViewport() {
	m.vp.SetContent(m.stack.top().content)
	m.vp.GotoTop()
}

// View renders header + panes + status bar (the approved prototype
// layout), the loading screen until the modules read lands, and the
// too-small guard.
func (m *Shell) View() tea.View {
	if m.w < minWidth || m.h < minHeight {
		return tea.NewView(m.th.viewTitle.Render(" terminal too small for the two-pane shell (need 60x12)"))
	}
	if !m.ready {
		return tea.NewView(m.th.viewTitle.Render(" dotdrift tui") + "\n" + m.th.loading.Render("loading profile…"))
	}

	header := m.th.headerTitle.Render(" dotdrift tui") +
		m.th.headerContext.Render(" · "+m.opts.ProfilePath)
	if f := m.read.Facts; f != nil {
		header += m.th.headerContext.Render(" · host " + f.Hostname + " · user " + f.Username)
	}
	if m.stack.dirtyAnywhere() {
		header += m.th.dirtyMark.Render("  ● unsaved")
	}

	main := m.vp.View()
	if m.apply != nil {
		main = m.apply.view()
	}
	left := m.pane(m.th.focusedBorder, m.th.unfocusedBorder, m.focus == focusTree, m.leftW-2, m.tree.View())
	right := m.pane(m.th.focusedBorder, m.th.unfocusedBorder, m.focus == focusMain, m.rightW-2, main)
	body := lipgloss.JoinHorizontal(lipgloss.Top, left, right)

	status := m.th.statusBar.Render(" " + m.help.View(m))
	if m.confirm {
		status = m.th.confirmLine.Render(" Really quit? Unsaved changes will be lost. (y/n)")
	}
	return tea.NewView(lipgloss.JoinVertical(lipgloss.Left, header, body, status))
}

// pane frames one pane's content; focus is the border color (0063).
func (m *Shell) pane(focused, unfocused lipgloss.Style, isFocused bool, w int, content string) string {
	st := unfocused
	if isFocused {
		st = focused
	}
	return st.Width(w).Height(m.paneH).Render(content)
}

// ShortHelp is the focused pane's status-bar line (0062-D7).
func (m *Shell) ShortHelp() []key.Binding {
	base := []key.Binding{keySwitchPane, keyQuit, keyHelp}
	if m.apply != nil {
		switch m.apply.phase {
		case applyGate:
			run := key.NewBinding(key.WithKeys("y", "n"), key.WithHelp("y/n", "run/decline"))
			return append([]key.Binding{run}, base...)
		case applyRunning:
			cancel := key.NewBinding(key.WithKeys("x"), key.WithHelp("x", "cancel"))
			return append([]key.Binding{cancel}, base...)
		case applyEnded:
			back := key.NewBinding(key.WithKeys("esc"), key.WithHelp("esc", "back"))
			return append([]key.Binding{back}, base...)
		}
	}
	if m.focus == focusTree {
		return append([]key.Binding{keyDown, keyOpen, keyJump}, base...)
	}
	raw := key.NewBinding(key.WithKeys("r"), key.WithHelp("r", "raw/resolved"))
	if m.stack.top().id.kind != viewModule {
		raw = key.NewBinding(key.WithDisabled())
	}
	if m.stack.top().id.kind == viewOrigin {
		edit := key.NewBinding(key.WithKeys("e"), key.WithHelp("e", "edit layer"))
		return append([]key.Binding{edit, raw, keyBack, keyScroll}, base...)
	}
	if m.stack.top().id.kind == viewEditor {
		save := key.NewBinding(key.WithKeys("s"), key.WithHelp("s", "save"))
		sections := key.NewBinding(key.WithKeys("[", "]"), key.WithHelp("[ ]", "section"))
		return append([]key.Binding{save, sections, keyBack}, base...)
	}
	return append([]key.Binding{raw, keyBack, keyScroll}, base...)
}

// FullHelp is `?`'s expanded view.
func (m *Shell) FullHelp() [][]key.Binding {
	return [][]key.Binding{
		{keySwitchPane, keyHelp, keyQuit},
		{keyUp, keyDown, keyJump, keyOpen},
		{keyScroll, keyRaw, keyBack},
	}
}

// mustAbs is the profile-root absolutization for diff walks; the root
// comes from the profile loader, which has already validated it.
func mustAbs(root string) string {
	abs, err := filepath.Abs(root)
	if err != nil {
		return root
	}
	return abs
}
