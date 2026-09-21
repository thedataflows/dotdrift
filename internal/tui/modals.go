package tui

// T-tui-modals: the modal family on the compositor. The elevation modal
// (one prompt per elevation, the plan's privileged steps listed as
// reasons, three failures abort, cancel aborts before anything runs,
// password a zeroed []byte never drafted); the apply driver (preview →
// destructive confirm → elevation gate → run; the badge + footer carry
// ambient status; the detail modal inspects); the dialogModal adapter
// absorbing the M14 manage/writes dialogs. Session events flow to the
// compositor regardless of open modals — the detail modal is an
// inspector, never the owner of the run.

import (
	"context"
	"os/exec"
	"strconv"
	"strings"

	tea "charm.land/bubbletea/v2"
	"github.com/thedataflows/dotdrift/internal/service"
)

// --- elevation ---

// elevationModel is the one-prompt-per-elevation modal. It owns no
// policy: check validates the password (production: executil.SudoValidate,
// which leaves a fresh sudo timestamp the run's privileged steps use),
// onOK proceeds, onCancel aborts. The password is a []byte zeroed on
// submission, never logged, never drafted.
type elevationModel struct {
	th       theme
	reasons  []string
	input    []byte
	failures int
	failed   bool // the last submission failed: show it
	check    func(pw []byte) error
	onOK     func()
	onCancel func(aborted bool)
	done     bool
}

func (e *elevationModel) finished() bool { return e.done }

// cancel is the compositor's esc-pop hook: cancelling the prompt aborts
// the operation before anything is touched.
func (e *elevationModel) cancel() {
	if e.onCancel != nil {
		e.onCancel(false)
	}
}

func (e *elevationModel) update(msg tea.Msg) tea.Cmd {
	if p, ok := msg.(tea.PasteMsg); ok {
		// 0091: pasting a password (a password manager's primary move)
		// extends the buffer; control runes drop out.
		e.input = append(e.input, []byte(string(pasteRunes(p.Content)))...)
		e.failed = false
		return nil
	}
	k, ok := msg.(tea.KeyPressMsg)
	if !ok {
		return nil
	}
	switch k.String() {
	case "enter":
		if len(e.input) == 0 {
			return nil
		}
		pw := e.input
		err := e.check(pw)
		for i := range pw {
			pw[i] = 0 // the buffer never outlives the submission
		}
		e.input = e.input[:0]
		if err == nil {
			e.done = true
			if e.onOK != nil {
				e.onOK()
			}
			return nil
		}
		e.failures++
		e.failed = true
		if e.failures >= 3 {
			e.done = true
			if e.onCancel != nil {
				e.onCancel(true)
			}
		}
		return nil
	case "backspace":
		if len(e.input) > 0 {
			e.input = e.input[:len(e.input)-1]
		}
	default:
		if k.Text != "" {
			e.input = append(e.input, []byte(k.Text)...)
			e.failed = false
		}
	}
	return nil
}

func (e *elevationModel) view(w, h int) string {
	var b strings.Builder
	b.WriteString(e.th.modalTitle.Render("sudo elevation required"))
	b.WriteString("\n\n")
	for _, r := range e.reasons {
		b.WriteString("  • " + r + "\n")
	}
	b.WriteString("\n  password: " + strings.Repeat("•", len(e.input)) + "▏\n")
	if e.failed {
		b.WriteString(e.th.errorMark.Render("  ✗ authentication failed") + "\n")
	}
	b.WriteString("\n" + e.th.meta.Render("  enter submit · esc cancel"))
	return e.th.modalBorder.Padding(1, 2).Render(b.String())
}

// privilegedReasons collects the elevation reason list: every step whose
// classification names sudo.
func privilegedReasons(previews []service.StepPreview) []string {
	var out []string
	for _, p := range previews {
		if p.NeedsTTY && strings.Contains(strings.ToLower(p.Reason), "sudo") {
			out = append(out, p.Name+": "+p.Reason)
		}
	}
	return out
}

// overwriteTargets collects the destructive-apply signal across steps.
func overwriteTargets(previews []service.StepPreview) []string {
	var out []string
	for _, p := range previews {
		out = append(out, p.Overwrites...)
	}
	return out
}

// --- apply driver ---

// applyRunState is the compositor's ambient apply state: the run, its
// step rows, the output ring, and the terminal verdict.
type applyRunState struct {
	run      ApplyRun
	rows     []*applyRow
	ring     *outputRing
	backups  []string
	progress string
	ended    *applyEndedState
}

// applyDrainedMsg signals the event stream's close (Wait's turn).
type applyDrainedMsg struct{}

// startApply previews the would-be session (P). The gates — destructive
// confirm, then elevation — hang off the preview's landing. A non-empty
// yank set snapshots here and rides the chain into Start (0093).
func (m *Compositor) startApply() tea.Cmd {
	if m.applying || m.applyFor == nil {
		return nil
	}
	m.applyModules = m.nav.yankedIDs()
	l := m.applyFor(m.facts)
	return func() tea.Msg {
		previews, err := l.Preview(service.ApplyOpts{ProfilePath: m.root, Modules: m.applyModules})
		return applyPreviewMsg{previews: previews, err: err}
	}
}

// applyPreview routes the landed classification through the gates.
func (m *Compositor) applyPreview(msg applyPreviewMsg) tea.Cmd {
	if msg.err != nil {
		m.message, m.msgErr = firstLineOf(msg.err.Error()), true
		return nil
	}
	m.applyPreviews = msg.previews
	if ow := overwriteTargets(msg.previews); len(ow) > 0 {
		body := []string{
			"overwrites " + strconv.Itoa(len(ow)) + " existing file(s) — backups are taken",
		}
		if len(ow) <= 6 {
			body = append(body, ow...)
		} else {
			body = append(body, ow[:6]...)
		}
		m.modals = append(m.modals, &confirmModel{
			th:    m.th,
			title: "apply overwrites existing files?",
			body:  body,
			onAnswer: func(ok bool) {
				if ok {
					m.afterPop = m.gateElevation
				} else {
					m.message, m.msgErr = "apply cancelled", false
				}
			},
		})
		return nil
	}
	return m.gateElevation()
}

// gateElevation opens the one elevation prompt when the plan carries
// privileged steps; otherwise the run starts.
func (m *Compositor) gateElevation() tea.Cmd {
	reasons := privilegedReasons(m.applyPreviews)
	if len(reasons) > 0 && m.sudoCheck != nil {
		m.modals = append(m.modals, &elevationModel{
			th:      m.th,
			reasons: reasons,
			check:   m.sudoCheck,
			onOK: func() {
				m.afterPop = m.startApplyRun
			},
			onCancel: func(aborted bool) {
				m.message, m.msgErr = "apply cancelled", aborted
			},
		})
		return nil
	}
	return m.startApplyRun()
}

// startApplyRun starts the session: rows from the frozen classification,
// badge on, the event pump armed.
func (m *Compositor) startApplyRun() tea.Cmd {
	l := m.applyFor(m.facts)
	m.apply = &applyRunState{ring: newOutputRing(outputRingMax)}
	m.applying = true
	opts := service.ApplyOpts{
		ProfilePath: m.root,
		Yes:         true,
		Modules:     m.applyModules, // the P-press yank snapshot (0093); nil = all
	}
	if m.send != nil {
		avail := true
		opts.HandoverAvailable = &avail
		opts.Handover = m.handover
	}
	return func() tea.Msg {
		run, err := l.Start(context.Background(), opts)
		return applyStartedMsg{run: run, err: err}
	}
}

// applyStarted arms the event pump on the started run.
func (m *Compositor) applyStarted(msg applyStartedMsg) tea.Cmd {
	if msg.err != nil {
		m.applying = false
		m.apply = nil
		m.message, m.msgErr = firstLineOf(msg.err.Error()), true
		return nil
	}
	m.apply.run = msg.run
	for _, p := range msg.run.Preview() {
		m.apply.rows = append(m.apply.rows, &applyRow{name: p.Name, needsTTY: p.NeedsTTY})
	}
	return m.waitApplyEvent()
}

// waitApplyEvent reads one event per cmd, coalescing output bursts into
// the ring non-blockingly (research 0059 §2 without a drain goroutine:
// the pump re-arms from Update).
func (m *Compositor) waitApplyEvent() tea.Cmd {
	ch := m.apply.run.Events()
	ring := m.apply.ring
	return func() tea.Msg {
		var ev service.Event
		var ok bool
		if m.pumpNoBlock {
			// Tests drive cmds synchronously; a blocking read on an
			// empty, open channel would hang the harness.
			select {
			case ev, ok = <-ch:
			default:
				return nil
			}
		} else {
			ev, ok = <-ch
		}
		if !ok {
			return applyDrainedMsg{}
		}
		for {
			if out, isOut := ev.(service.StepOutput); isOut {
				ring.append(string(out.Chunk))
				select {
				case ev, ok = <-ch:
					if !ok {
						return applyDrainedMsg{}
					}
					continue
				default:
					return applyTickMsg{} // output landed; repaint
				}
			}
			return applyEventMsg{ev: ev}
		}
	}
}

// applyEvent absorbs one event and re-arms the pump.
func (m *Compositor) applyEvent(msg applyEventMsg) tea.Cmd {
	absorbEvent(m.apply.rows, m.apply.ring, &m.apply.backups, &m.apply.progress, msg.ev)
	return m.waitApplyEvent()
}

// applyDrained turns the closed stream into the terminal result.
func (m *Compositor) applyDrained() tea.Cmd {
	return func() tea.Msg {
		res, err := m.apply.run.Wait()
		return applyWaitedMsg{res: res, err: err}
	}
}

// applyWaited records the verdict and reports it in the footer.
func (m *Compositor) applyWaited(msg applyWaitedMsg) tea.Cmd {
	end := applyEndState(msg.res, msg.err)
	m.apply.ended = &end
	m.applying = false
	switch end.outcome {
	case service.OutcomeCompleted:
		m.message, m.msgErr = "apply: completed", false
	case service.OutcomeCancelled:
		m.message, m.msgErr = "apply cancelled at "+end.stepName, true
	default:
		m.message, m.msgErr = "apply failed: "+end.errText, true
	}
	return nil
}

// handover is the session's Handover seam: the child goes to the program
// as applyHandoverMsg (Update answers tea.ExecProcess) and the step's
// goroutine blocks until the child ran.
func (m *Compositor) handover(cmd *exec.Cmd) error {
	done := make(chan error, 1)
	m.send(applyHandoverMsg{cmd: cmd, done: done})
	return <-done
}

// --- apply detail modal ---

// applyDetailModel inspects the ambient run: step rows, the output tail,
// the verdict. Closing never cancels; ctrl+c inside asks first.
type applyDetailModel struct {
	c      *Compositor
	scroll int
}

func (d *applyDetailModel) update(msg tea.Msg) tea.Cmd {
	k, ok := msg.(tea.KeyPressMsg)
	if !ok {
		return nil
	}
	switch k.String() {
	case "j", "down":
		if d.c.apply != nil {
			// The window shows 8 lines; scroll walks back from the end.
			if maxScroll := len(d.c.apply.ring.snapshot()) - 8; d.scroll < maxScroll {
				d.scroll++
			}
		}
	case "k", "up":
		if d.scroll > 0 {
			d.scroll--
		}
	case "ctrl+c":
		if d.c.applying {
			d.c.modals = append(d.c.modals, &confirmModel{
				th:    d.c.th,
				title: "cancel apply?",
				body: []string{
					"the run stops at the current step",
					"the next apply resumes from the last completed step",
				},
				onAnswer: func(ok bool) {
					if ok && d.c.apply != nil && d.c.apply.run != nil {
						d.c.apply.run.Cancel()
					}
				},
			})
		}
	}
	return nil
}

func (d *applyDetailModel) view(w, h int) string {
	st := d.c.apply
	if st == nil {
		return ""
	}
	var b strings.Builder
	status := "running"
	if st.progress != "" {
		status += " " + st.progress
	}
	if st.ended != nil {
		status = string(st.ended.outcome)
		if st.ended.stepName != "" {
			status += " · " + st.ended.stepName
		}
	}
	b.WriteString(d.c.th.modalTitle.Render("apply — "+status) + "\n\n")
	for _, r := range st.rows {
		b.WriteString(renderApplyRow(d.c.th, r))
	}
	if len(st.backups) > 0 {
		for _, line := range st.backups {
			b.WriteString(d.c.th.meta.Render("  "+line) + "\n")
		}
	}
	lines := st.ring.snapshot()
	if len(lines) > 0 {
		b.WriteString("\n")
		// The tail window: scroll walks back from the end.
		end := len(lines) - d.scroll
		if end < 0 {
			end = 0
		}
		start := max(0, end-8)
		for _, l := range lines[start:end] {
			b.WriteString(d.c.th.meta.Render("  "+strings.TrimRight(l, "\n")) + "\n")
		}
	}
	if st.ended != nil && st.ended.errText != "" {
		b.WriteString(d.c.th.errorMark.Render("  "+st.ended.errText) + "\n")
	}
	b.WriteString("\n" + d.c.th.meta.Render("  esc close · ctrl+c cancel run"))
	return d.c.th.modalBorder.Padding(1, 2).Width(min(w-6, 110)).Render(b.String())
}

// openApplyDetail opens the inspector (a) while a run lives or its
// verdict stands.
func (m *Compositor) openApplyDetail() {
	if m.apply == nil {
		return
	}
	m.modals = append(m.modals, &applyDetailModel{c: m})
}

// --- manage dialog absorption ---

// dialogModal adapts an M14 dialog (manage, writes) to the compositor's
// modal interface: the domain logic is untouched, the shell changes from
// pushed view to modal layer, and esc pops the dialog through the
// compositor's pop rule.
type dialogModal struct {
	d  dialog
	th theme
	// reload runs after a successful write: every dialog whose write
	// changes the profile sets it (manage since 0082, onboard/generate
	// since 0084's follow-up) so the nav shows the change without a
	// restart. Restore writes live targets only — no reload.
	reload func() tea.Cmd
}

func (m *dialogModal) view(w, h int) string {
	return m.th.modalBorder.Padding(1, 2).Render(m.d.View(m.th))
}

func (m *dialogModal) update(msg tea.Msg) tea.Cmd {
	if k, ok := msg.(tea.KeyPressMsg); ok {
		return m.d.HandleKey(k.String())
	}
	if p, ok := msg.(tea.PasteMsg); ok {
		// 0091: bracketed paste is a message, not keys — the dialog's
		// paste hook lands it in the focused field.
		if pd, ok := m.d.(interface{ paste(string) }); ok {
			pd.paste(p.Content)
		}
		return nil
	}
	if rp, ok := msg.(restorePlanMsg); ok {
		// 0084: the resolve cmd's message lands on the one dialog that
		// produces it — without this the dialog never passes the
		// targets row.
		if rd, ok := m.d.(*restoreDialog); ok {
			rd.applyPlan(rp)
		}
		return nil
	}
	if wf, ok := msg.(writeFinishedMsg); ok {
		m.d.applyFinished(wf)
		if wf.err == nil && m.reload != nil {
			return m.reload()
		}
	}
	return nil
}

// openManageCreate opens the manage dialog straight in create mode, the
// module field prefilled (the palette's ctrl+n escape from no-results —
// explicit, never auto-offered).
func (m *Compositor) openManageCreate(prefill string) {
	ce, ok := m.reader.(service.ConfigEditor)
	if !ok {
		m.message, m.msgErr = "module management needs the config area", true
		return
	}
	sel := m.nav.selected()
	item := manageSel{moduleID: sel.moduleID, dir: sel.dir}
	d := newManageDialog(ce, m.root, m.facts, item)
	d.openEntry() // menu cursor defaults to "create module"
	if prefill != "" && len(d.rows) > 0 && d.rows[0].field != nil {
		d.rows[0].field.value = []rune(prefill)
		d.rows[0].field.cursor = len([]rune(prefill))
	}
	m.modals = append(m.modals, &dialogModal{d: d, th: m.th, reload: m.reloadNav})
}

// openManage opens the module-management menu (m) as a modal over the
// nav selection.
func (m *Compositor) openManage() {
	ce, ok := m.reader.(service.ConfigEditor)
	if !ok {
		m.message, m.msgErr = "module management needs the config area", true
		return
	}
	sel := m.nav.selected()
	item := manageSel{moduleID: sel.moduleID, dir: sel.dir}
	if sel.layer == "" && sel.moduleID != "" {
		for _, mod := range m.nav.modules {
			if mod.id == sel.moduleID && len(mod.layers) > 0 {
				item.dir = mod.layers[0].dir
			}
		}
	}
	m.modals = append(m.modals, &dialogModal{d: newManageDialog(ce, m.root, m.facts, item), th: m.th, reload: m.reloadNav})
}
