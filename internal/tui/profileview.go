package tui

// T-tui-views slice 1c (issue 0107): the Profile view body on the slice
// 1b frame. One cursor-less read-only list: the detect facts (host,
// user, os, kernel, distro, GPU, backend), the profile root, the state
// path, the resume line (the same rule as RenderStatusHeader — a
// pending cursor only when its input hash still matches, the 0106 mask
// already applied by ReadsArea.Profile), and the other-accounts note.
// The read is async: entering the view schedules it, profileLoadedMsg
// lands it (the navLoadedMsg pattern). r re-reads the active view, R
// re-reads all views; a write or a finished apply marks the read views
// stale (markReadViewsStale) and the row titles carry the stale word
// until a refresh clears it.

import (
	"fmt"
	"strings"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"
	"github.com/thedataflows/dotdrift/internal/profile"
	"github.com/thedataflows/dotdrift/internal/service"
)

// profileLoadedMsg carries the Profile view's light read (0107).
type profileLoadedMsg struct {
	read *service.ProfileRead
	err  error
}

// profileModel is the Profile view's state: no cursor, no scroll — the
// list fits the pane or the pane clips it.
type profileModel struct {
	pending bool
	seq     int
	loadErr string
	read    *service.ProfileRead
}

// switchView activates a view on the row and schedules the view's read
// on first entry.
func (m *Compositor) switchView(v viewKind) tea.Cmd {
	m.view = v
	if v == viewProfile && m.profile.read == nil && !m.profile.pending && m.profile.loadErr == "" {
		return m.reloadProfile()
	}
	if v == viewStatus && m.status.read == nil && !m.status.pending && m.status.loadErr == "" {
		return m.reloadStatus()
	}
	if v == viewPlan && m.planv.read == nil && !m.planv.pending && m.planv.loadErr == "" {
		return m.reloadPlan()
	}
	return nil
}

// reloadProfile re-runs the light profile read (r on the Profile view,
// first entry). Read-only and probe-free, like every read on the seam.
func (m *Compositor) reloadProfile() tea.Cmd {
	if m.area == nil || m.profile.pending {
		return nil
	}
	m.profile.pending = true
	m.profile.seq = m.staleSeq[viewProfile]
	m.profile.loadErr = ""
	return func() tea.Msg {
		r, err := m.area.Profile(m.root, nil)
		return profileLoadedMsg{read: r, err: err}
	}
}

// refreshView re-reads the active view (r). The Modules view re-runs the
// nav read.
func (m *Compositor) refreshView() tea.Cmd {
	switch m.view {
	case viewModules:
		return m.reloadNav()
	case viewStatus:
		return m.reloadStatus()
	case viewPlan:
		return m.reloadPlan()
	case viewProfile:
		return m.reloadProfile()
	}
	return nil
}

// refreshAllViews re-reads every view that has a read (R).
func (m *Compositor) refreshAllViews() tea.Cmd {
	return tea.Batch(m.reloadNav(), m.reloadProfile(), m.reloadStatus(), m.reloadPlan())
}

// profileBody renders the Profile view's list into the view pane.
func (m *Compositor) profileBody(w int) string {
	p := &m.profile
	switch {
	case p.loadErr != "":
		return m.th.errorMark.Render(" profile read failed: " + firstLineOf(p.loadErr))
	case p.pending:
		return m.th.loading.Render(" …")
	case p.read == nil:
		return m.th.loading.Render(" not loaded")
	}
	r := p.read
	var b strings.Builder
	row := func(label, value string) {
		if value == "" { // an undetected fact (kernel, distro, gpu) renders nothing
			return
		}
		line := m.th.rowKey.Render(fmt.Sprintf(" %-8s", label+":")) + " " + m.th.rowText.Render(value)
		b.WriteString(ansi.Truncate(line, w, "") + "\n")
	}
	if f := r.Facts; f != nil {
		row("host", f.Hostname)
		row("user", f.Username)
		row("os", f.OS)
		row("kernel", f.Kernel)
		row("distro", f.Distro)
		row("gpu", f.GPU)
		row("backend", f.Backend)
	}
	b.WriteString("\n")
	row("profile", r.ProfilePath)
	row("state", r.StatePath)
	// The resume rule is Status's (RenderStatusHeader): clean unless a
	// cursor is pending. A pending message carries the warn hue (the
	// reasonMark amber); the clean message renders as a plain value.
	resume := "clean - next apply starts from the beginning"
	pending := r.State != nil && r.State.LastCompleted != ""
	if pending {
		resume = fmt.Sprintf("last completed %q - next apply resumes after it", r.State.LastCompleted)
	}
	resumeLine := m.th.rowKey.Render(fmt.Sprintf(" %-8s", "resume:")) + " "
	if pending {
		resumeLine += m.th.reasonMark.Render(resume)
	} else {
		resumeLine += m.th.rowText.Render(resume)
	}
	b.WriteString(ansi.Truncate(resumeLine, w, "") + "\n")
	if len(r.Others) > 0 {
		// The 0038 note, Status's wording: name each other account with
		// configuration on this machine and its apply command.
		b.WriteString("\n")
		b.WriteString(ansi.Truncate(" note: configuration exists for other accounts on this machine:", w, "") + "\n")
		for _, acct := range r.Others {
			b.WriteString(ansi.Truncate(m.th.rowText.Render("   users/"+acct.Name+" — apply with: "+accountApplyCmd(acct)), w, "") + "\n")
		}
		b.WriteString(ansi.Truncate(m.th.meta.Render("   (each account needs dotdrift on its PATH — install it system-wide, e.g. via mise, system scope)"), w, "") + "\n")
	}
	return strings.TrimRight(b.String(), "\n")
}

// accountApplyCmd is the per-account apply instruction (the rule of
// service's applyCommandFor, 0038): the uid-0 account converges with
// plain sudo; any other account needs a login shell so its own PATH
// applies.
func accountApplyCmd(acct profile.Account) string {
	if acct.Uid == "0" {
		return "sudo dotdrift apply"
	}
	return "sudo -iu " + acct.Name + " dotdrift apply"
}
