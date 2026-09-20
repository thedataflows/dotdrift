package tui

import (
	"bytes"
	"errors"
	"os/exec"
	"strings"

	"charm.land/bubbletea/v2"
	"github.com/thedataflows/dotdrift/internal/generate"
	"github.com/thedataflows/dotdrift/internal/profile"
	"github.com/thedataflows/dotdrift/internal/service"
)

// The Profile dialogs (T-tui-writes): onboard, restore, and generate as
// interactive forms on the shell's view stack. Each dialog owns the main
// pane's keys while its view is on top, runs its write through the
// Writes interface once the confirm gate answers yes, and renders the
// service's own report lines when the write lands. The assembly reuses
// generate's shared builders (contract 15) — the CLI and the dialogs
// build the same input the same way.

// writeFinishedMsg carries one finished dialog write: the service's
// report lines and its error. Key names the dialog.
type writeFinishedMsg struct {
	key    string
	report string
	err    error
}

// restorePlanMsg carries the resolved restore plan for the restore
// dialog.
type restorePlanMsg struct {
	key  string
	plan []service.RestorePlanItem
	err  error
}

// dialog is one Profile action's interactive model.
type dialog interface {
	HandleKey(string) tea.Cmd
	View(th theme) string
	applyFinished(writeFinishedMsg)
	// back is the esc discipline (issue 0072): true when the dialog
	// consumed the back step (a confirm gate clears and stays, a form
	// steps back), false when the shell should pop the view.
	back() bool
}

// tuiRestoreHandover refuses privileged children: the TUI has no
// terminal to hand over yet (the apply work owns that bridge), and the
// restore dialog skips elevated targets before any restore runs. This is
// the safety net behind that filter, carrying the same session Handover
// contract (0064-D9).
func tuiRestoreHandover(*exec.Cmd) error {
	return errors.New("needs a terminal: run dotdrift restore from the CLI")
}

// dlgField is one editable text line: a label and a value with a cursor.
type dlgField struct {
	label  string
	value  []rune
	cursor int
	hint   string
}

func newDlgField(label, value string) *dlgField {
	return &dlgField{label: label, value: []rune(value), cursor: len([]rune(value))}
}

func (f *dlgField) String() string { return string(f.value) }

// set replaces the value (the kind row keeps the module id in step).
func (f *dlgField) set(s string) {
	f.value = []rune(s)
	f.cursor = len(f.value)
}

func (f *dlgField) typeRune(r rune) {
	f.value = append(f.value[:f.cursor], append([]rune{r}, f.value[f.cursor:]...)...)
	f.cursor++
}

func (f *dlgField) backspace() {
	if f.cursor == 0 {
		return
	}
	f.value = append(f.value[:f.cursor-1], f.value[f.cursor:]...)
	f.cursor--
}

func (f *dlgField) left() {
	if f.cursor > 0 {
		f.cursor--
	}
}

func (f *dlgField) right() {
	if f.cursor < len(f.value) {
		f.cursor++
	}
}

// dlgChoice is one choice line: a fixed option list cycled with
// left/right.
type dlgChoice struct {
	label string
	opts  []string
	cur   int
}

func newDlgChoice(label string, opts ...string) *dlgChoice {
	return &dlgChoice{label: label, opts: opts}
}

func (c *dlgChoice) String() string { return c.opts[c.cur] }

func (c *dlgChoice) left() {
	if c.cur > 0 {
		c.cur--
	}
}

func (c *dlgChoice) right() {
	if c.cur < len(c.opts)-1 {
		c.cur++
	}
}

// dlgRow is one dialog line: a field or a choice.
type dlgRow struct {
	field  *dlgField
	choice *dlgChoice
}

func fieldRow(f *dlgField) dlgRow   { return dlgRow{field: f} }
func choiceRow(c *dlgChoice) dlgRow { return dlgRow{choice: c} }

func (r dlgRow) left() {
	if r.choice != nil {
		r.choice.left()
	} else {
		r.field.left()
	}
}

func (r dlgRow) right() {
	if r.choice != nil {
		r.choice.right()
	} else {
		r.field.right()
	}
}

func (r dlgRow) typeRune(rr rune) {
	if r.field != nil {
		r.field.typeRune(rr)
	}
}

func (r dlgRow) backspace() {
	if r.field != nil {
		r.field.backspace()
	}
}

// render lays out one row: label and value, no marker — styling is
// renderRow's business.
func (r dlgRow) render() string {
	var val string
	if r.choice != nil {
		val = r.choice.String() + "  (< > to change)"
	} else {
		val = r.field.String()
		if val == "" && r.field.hint != "" {
			val = "(" + r.field.hint + ")"
		}
	}
	return r.rowLabel() + " " + val
}

// renderRow styles one row through the registry: the focused row takes
// the cursor treatment (bar + accent, 0075), plain rows keep the lead.
func (r dlgRow) renderRow(th theme, focused bool) string {
	if focused {
		return th.cursorRow.Render(" " + r.render())
	}
	return "  " + r.render()
}

func (r dlgRow) rowLabel() string {
	if r.choice != nil {
		return r.choice.label
	}
	return r.field.label
}

// splitList splits a comma- or space-separated field into entries.
func splitList(s string) []string {
	return strings.FieldsFunc(s, func(r rune) bool { return r == ' ' || r == ',' })
}

// finishView renders a dialog's tail: the confirm gate or the run's
// report.
func finishView(th theme, confirmText, report string, err error, running bool) string {
	switch {
	case err != nil:
		return "\n" + th.errorMark.Render("error: "+err.Error())
	case report != "":
		return "\n" + th.sectionLabel.Render("report") + "\n" + report
	case running:
		return "\n" + th.loading.Render("running…")
	case confirmText != "":
		return "\n" + th.reasonMark.Render(confirmText+"  y/n")
	}
	return ""
}

// onboardDialog adopts live paths into a module: app, paths, the layer
// choice, and the optional declarations, with a dry-run switch.
type onboardDialog struct {
	w       Writes
	profile string
	rows    []dlgRow
	cur     int
	confirm bool
	running bool
	report  string
	err     error
}

func newOnboardDialog(w Writes, profilePath string) *onboardDialog {
	d := &onboardDialog{
		w:       w,
		profile: profilePath,
		rows: []dlgRow{
			fieldRow(newDlgField("app", "")),
			fieldRow(newDlgField("paths", "")),
			choiceRow(newDlgChoice("layer", "base", "host", "user")),
			fieldRow(&dlgField{label: "owner", hint: "detected account"}),
			choiceRow(newDlgChoice("mode", "symlink", "symlink-each", "copy", "template")),
			fieldRow(newDlgField("packages", "")),
			fieldRow(newDlgField("tools", "")),
			choiceRow(newDlgChoice("dry-run", "no", "yes")),
		},
	}
	return d
}

func (d *onboardDialog) HandleKey(key string) tea.Cmd {
	if d.confirm {
		switch key {
		case "y":
			d.confirm = false
			d.running = true
			d.report, d.err = "", nil
			return d.run
		case "n":
			d.confirm = false
		}
		return nil
	}
	switch key {
	case "up":
		if d.cur > 0 {
			d.cur--
		}
	case "down":
		if d.cur < len(d.rows)-1 {
			d.cur++
		}
	case "left":
		d.rows[d.cur].left()
	case "right":
		d.rows[d.cur].right()
	case "backspace":
		d.rows[d.cur].backspace()
	case "enter":
		app := d.rows[0].field.String()
		paths := d.rows[1].field.String()
		if app == "" || paths == "" {
			return nil
		}
		d.confirm = true
	default:
		r := []rune(key)
		if len(r) == 1 {
			d.rows[d.cur].typeRune(r[0])
		}
	}
	return nil
}

func (d *onboardDialog) run() tea.Msg {
	layer := d.rows[2].choice.String()
	owner := d.rows[3].field.String()
	var buf bytes.Buffer
	err := d.w.Onboard(service.OnboardOpts{
		ProfileRoot: d.profile,
		Paths:       splitList(d.rows[1].field.String()),
		App:         d.rows[0].field.String(),
		Mode:        d.rows[4].choice.String(),
		Packages:    splitList(d.rows[5].field.String()),
		Tools:       splitList(d.rows[6].field.String()),
		HostSet:     layer == "host",
		Hostname:    owner,
		UserSet:     layer == "user",
		Username:    owner,
		DryRun:      d.rows[7].choice.String() == "yes",
		Out:         &buf,
	})
	return writeFinishedMsg{key: "onboard", report: buf.String(), err: err}
}

func (d *onboardDialog) applyFinished(m writeFinishedMsg) {
	d.running = false
	d.report, d.err = m.report, m.err
}

// back is the shared esc discipline (issue 0072): the gate consumes esc,
// the form's back step pops the dialog.
func (d *onboardDialog) back() bool {
	if d.confirm {
		d.confirm = false
		return true
	}
	return false
}

func (d *onboardDialog) View(th theme) string {
	var b strings.Builder
	b.WriteString(th.viewTitle.Render("ONBOARD"))
	b.WriteString("\nadopt live paths into a module, then apply it\n\n")
	for i, r := range d.rows {
		b.WriteString(r.renderRow(th, i == d.cur))
		b.WriteString("\n")
	}
	b.WriteString(finishView(th, "run onboard?", d.report, d.err, d.running))
	b.WriteString("\n" + th.meta.Render("up/down pick a row · type to edit · enter runs · esc back"))
	return b.String()
}

// restoreDialog copies backed-up files back to their live targets: type
// the targets, resolve them to generations, pin one per target, run.
type restoreDialog struct {
	w        Writes
	profile  string
	index    func() (map[string]map[string][]service.RestoreHit, error)
	targets  *dlgField
	plan     []service.RestorePlanItem
	gens     map[string][]string // target → generations, newest first
	picked   map[string]int      // target → index into gens (0 = newest)
	resolved bool
	confirm  bool
	running  bool
	report   string
	err      error
	cur      int
}

func newRestoreDialog(w Writes, profilePath string, index func() (map[string]map[string][]service.RestoreHit, error)) *restoreDialog {
	return &restoreDialog{
		w:       w,
		profile: profilePath,
		index:   index,
		targets: newDlgField("targets", ""),
	}
}

// applyPlan absorbs a resolved plan: the target rows come from the
// service; the generation options come from the profile's backup index.
func (d *restoreDialog) applyPlan(m restorePlanMsg) {
	if m.err != nil {
		d.err = m.err
		return
	}
	d.plan = m.plan
	d.gens = map[string][]string{}
	d.picked = map[string]int{}
	if d.index != nil {
		if hits, err := d.index(); err == nil {
			for target, byModule := range hits {
				seen := map[string]bool{}
				var gens []string
				for _, list := range byModule {
					for _, h := range list {
						if !seen[h.Gen] {
							seen[h.Gen] = true
							gens = append(gens, h.Gen)
						}
					}
				}
				d.gens[target] = gens // newest-first (generations iterate newest-first)
			}
		}
	}
	d.resolved = true
}

// pickLabel renders one plan row: target, pinned generation, and the
// elevated marker.
func (d *restoreDialog) pickLabel(item service.RestorePlanItem, focused bool) string {
	mark := "  "
	if focused {
		mark = "> "
	}
	if item.Elevated {
		return mark + item.Target + "\n    needs a terminal — run: dotdrift restore " + item.Target
	}
	gens := d.gens[item.Target]
	pin := "newest"
	if i, ok := d.picked[item.Target]; ok && gens != nil && i > 0 && i < len(gens) {
		pin = gens[i]
	}
	return mark + item.Target + "\n    generation: " + pin + "  (" + item.Label + ")  (< > to change)"
}

func (d *restoreDialog) HandleKey(key string) tea.Cmd {
	if d.confirm {
		switch key {
		case "y":
			d.confirm = false
			d.running = true
			d.report, d.err = "", nil
			return d.run
		case "n":
			d.confirm = false
		}
		return nil
	}
	if !d.resolved {
		switch key {
		case "enter":
			if d.targets.String() == "" {
				return nil
			}
			return d.resolve
		case "backspace":
			d.targets.backspace()
		default:
			r := []rune(key)
			if len(r) == 1 {
				d.targets.typeRune(r[0])
			}
		}
		return nil
	}
	switch key {
	case "up":
		if d.cur > 0 {
			d.cur--
		}
	case "down":
		if d.cur < len(d.plan)-1 {
			d.cur++
		}
	case "left", "right":
		if d.cur < len(d.plan) {
			item := d.plan[d.cur]
			gens := d.gens[item.Target]
			if len(gens) > 0 && !item.Elevated {
				i := d.picked[item.Target]
				if key == "left" && i < len(gens)-1 {
					i++
				}
				if key == "right" && i > 0 {
					i--
				}
				d.picked[item.Target] = i
			}
		}
	case "enter":
		d.confirm = true
	}
	return nil
}

func (d *restoreDialog) resolve() tea.Msg {
	plan, err := d.w.RestorePlan(service.RestoreOpts{ProfilePath: d.profile, Targets: splitList(d.targets.String())})
	return restorePlanMsg{key: "restore", plan: plan, err: err}
}

func (d *restoreDialog) run() tea.Msg {
	var buf bytes.Buffer
	for _, item := range d.plan {
		if item.Elevated {
			continue // named with its CLI command; the TUI never runs sudo
		}
		gen := ""
		if i, ok := d.picked[item.Target]; ok && i > 0 {
			gen = d.gens[item.Target][i]
		}
		err := d.w.Restore(service.RestoreOpts{
			ProfilePath: d.profile,
			Targets:     []string{item.Target},
			Gen:         gen,
			Out:         &buf,
			Handover:    tuiRestoreHandover,
		})
		if err != nil {
			return writeFinishedMsg{key: "restore", report: buf.String(), err: err}
		}
	}
	return writeFinishedMsg{key: "restore", report: buf.String()}
}

func (d *restoreDialog) applyFinished(m writeFinishedMsg) {
	d.running = false
	d.report, d.err = m.report, m.err
}

func (d *restoreDialog) back() bool {
	if d.confirm {
		d.confirm = false
		return true
	}
	return false
}

func (d *restoreDialog) View(th theme) string {
	var b strings.Builder
	b.WriteString(th.viewTitle.Render("RESTORE"))
	b.WriteString("\ncopy backed-up files back to their live targets\n\n")
	if !d.resolved {
		b.WriteString("> targets " + d.targets.String() + "\n")
		b.WriteString("\n" + th.meta.Render("enter resolves the targets to their backup generations"))
		b.WriteString(finishView(th, "", d.report, d.err, d.running))
		return b.String()
	}
	for i, item := range d.plan {
		b.WriteString(d.pickLabel(item, i == d.cur))
		b.WriteString("\n")
	}
	b.WriteString(finishView(th, "run restore?", d.report, d.err, d.running))
	b.WriteString("\n" + th.meta.Render("up/down pick a target · < > pin a generation · enter runs · esc back"))
	return b.String()
}

// generateDialog materializes a new mounts or smb module at a chosen
// layer, assembling the input through generate's shared builders and
// writing through the service — the one assembly path (contract 15).
type generateDialog struct {
	w        Writes
	profile  string
	kind     *dlgChoice
	layer    *dlgChoice
	module   *dlgField
	hostname *dlgField
	username *dlgField
	mounts   []dlgRow
	smb      []dlgRow
	cur      int
	confirm  bool
	running  bool
	report   string
	err      error
}

func newGenerateDialog(w Writes, profilePath string) *generateDialog {
	return &generateDialog{
		w:        w,
		profile:  profilePath,
		kind:     newDlgChoice("kind", "mounts", "smb"),
		layer:    newDlgChoice("layer", "base", "host", "user"),
		module:   newDlgField("module", "mounts"),
		hostname: &dlgField{label: "hostname", hint: "detected for host layer"},
		username: &dlgField{label: "username", hint: "detected for user layer"},
		mounts: []dlgRow{
			fieldRow(newDlgField("name", "")),
			fieldRow(newDlgField("source", "")),
			fieldRow(newDlgField("destination", "")),
			fieldRow(newDlgField("type", "")),
			fieldRow(newDlgField("options", "")),
			fieldRow(newDlgField("startat", "")),
			choiceRow(newDlgChoice("state", "enabled", "disabled")),
		},
		smb: []dlgRow{
			fieldRow(&dlgField{label: "group", hint: "smb"}),
			fieldRow(&dlgField{label: "users", hint: "invoking user"}),
			fieldRow(newDlgField("shares", "")),
			choiceRow(newDlgChoice("avahi", "on", "off")),
			choiceRow(newDlgChoice("writable", "on", "off")),
			choiceRow(newDlgChoice("readonly", "no", "yes")),
			choiceRow(newDlgChoice("public", "no", "yes")),
		},
	}
}

func (d *generateDialog) rows() []dlgRow {
	if d.kind.String() == "smb" {
		return d.smb
	}
	return d.mounts
}

// prefix is the fixed head rows: kind, layer. The identity rows come
// after the kind-specific fields.
func (d *generateDialog) prefix() []dlgRow {
	return []dlgRow{
		choiceRow(d.kind),
		choiceRow(d.layer),
	}
}

// identity rows: the module id and the overlay owners (auto-detected
// when empty).
func (d *generateDialog) identity() []dlgRow {
	return []dlgRow{
		fieldRow(d.module),
		fieldRow(d.hostname),
		fieldRow(d.username),
	}
}

func (d *generateDialog) all() []dlgRow {
	rows := append(d.prefix(), d.rows()...)
	return append(rows, d.identity()...)
}

func (d *generateDialog) HandleKey(key string) tea.Cmd {
	rows := d.all()
	if d.confirm {
		switch key {
		case "y":
			d.confirm = false
			d.running = true
			d.report, d.err = "", nil
			return d.run
		case "n":
			d.confirm = false
		}
		return nil
	}
	switch key {
	case "up":
		if d.cur > 0 {
			d.cur--
		}
	case "down":
		if d.cur < len(rows)-1 {
			d.cur++
		}
	case "left":
		rows[d.cur].left()
		if d.cur == 0 {
			d.module.set("smb") // follow the kind
		}
	case "right":
		rows[d.cur].right()
		if d.cur == 0 {
			d.module.set("mounts")
		}
	case "backspace":
		rows[d.cur].backspace()
	case "enter":
		if !d.valid() {
			return nil
		}
		d.confirm = true
	default:
		r := []rune(key)
		if len(r) == 1 {
			rows[d.cur].typeRune(r[0])
		}
	}
	return nil
}

// valid checks the required fields loudly: the mounts flow needs
// name/source/destination/type, the smb flow needs at least one share.
func (d *generateDialog) valid() bool {
	if d.kind.String() == "smb" {
		return d.smb[2].field.String() != ""
	}
	for _, i := range []int{0, 1, 2, 3} {
		if d.mounts[i].field.String() == "" {
			return false
		}
	}
	return true
}

func (d *generateDialog) run() tea.Msg {
	sel, err := d.w.GenerateSelection(generate.Selection{
		Layer:    d.layer.String(),
		ModuleID: d.module.String(),
		Hostname: d.hostname.String(),
		Username: d.username.String(),
	})
	if err != nil {
		return writeFinishedMsg{key: "generate", err: err}
	}
	uid, gid, user, err := generate.InvokingUser()
	if err != nil {
		return writeFinishedMsg{key: "generate", err: err}
	}
	var input generate.Input
	if d.kind.String() == "smb" {
		users := splitList(d.smb[1].field.String())
		on := func(row dlgRow) *bool {
			if row.choice.String() == "on" {
				return nil // the builder's default is on
			}
			off := false
			return &off
		}
		readonly := d.smb[5].choice.String() == "yes"
		public := d.smb[6].choice.String() == "yes"
		shares, err := generate.ParseShareFlags(splitList(d.smb[2].field.String()), generate.ResolveWritable(on(d.smb[4]), readonly), public)
		if err != nil {
			return writeFinishedMsg{key: "generate", err: err}
		}
		input = generate.SmbInput(d.smb[0].field.String(), users, on(d.smb[3]), shares, user, uid, gid)
	} else {
		spec := profile.MountSpec{
			Source:      d.mounts[1].field.String(),
			Destination: d.mounts[2].field.String(),
			Type:        d.mounts[3].field.String(),
			Options:     splitList(d.mounts[4].field.String()),
			StartAt:     d.mounts[5].field.String(),
			State:       d.mounts[6].choice.String(),
		}
		input = generate.MountsInput(map[string]profile.MountSpec{
			d.mounts[0].field.String(): spec,
		}, uid, gid)
	}
	var buf bytes.Buffer
	if err := d.w.WriteGenerate(d.profile, sel, input, &buf); err != nil {
		return writeFinishedMsg{key: "generate", report: buf.String(), err: err}
	}
	return writeFinishedMsg{key: "generate", report: buf.String()}
}

func (d *generateDialog) applyFinished(m writeFinishedMsg) {
	d.running = false
	d.report, d.err = m.report, m.err
}

func (d *generateDialog) back() bool {
	if d.confirm {
		d.confirm = false
		return true
	}
	return false
}

func (d *generateDialog) View(th theme) string {
	var b strings.Builder
	b.WriteString(th.viewTitle.Render("GENERATE"))
	b.WriteString("\nmaterialize a new module from the fields below\n\n")
	for i, r := range d.all() {
		b.WriteString(r.renderRow(th, i == d.cur))
		b.WriteString("\n")
	}
	b.WriteString(finishView(th, "run generate?", d.report, d.err, d.running))
	b.WriteString("\n" + th.meta.Render("up/down pick a row · type to edit · enter runs · esc back"))
	return b.String()
}
