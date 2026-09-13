package tui

import (
	"path/filepath"
	"strings"

	tea "charm.land/bubbletea/v2"

	"github.com/thedataflows/dotdrift/internal/facts"
	"github.com/thedataflows/dotdrift/internal/service"
)

// The module-management dialogs (issue 0072): a context menu over the
// config area's module ops — create scaffolds a minimal module, move
// moves the directory wholesale (refused on a target collision), delete
// previews the orphans before the confirm gate (tui.md, "Module
// management"). One dialog: the menu's entries open in place as forms
// over the same stacked view, esc steps back through the menu, then out.

const manageKey = "modules"

type manageMode int

const (
	manageMenu manageMode = iota
	manageCreate
	manageMove
	manageDelete
)

type manageDialog struct {
	cfg   service.ConfigEditor
	root  string
	facts *facts.Facts
	sel   treeItem // the selection the menu hangs off (module context)

	mode    manageMode
	cur     int
	rows    []dlgRow
	orphans []string
	confirm bool
	running bool
	report  string
	err     error
}

func newManageDialog(cfg service.ConfigEditor, root string, f *facts.Facts, sel treeItem) *manageDialog {
	d := &manageDialog{cfg: cfg, root: root, facts: f, sel: sel}
	d.toMenu()
	return d
}

func (d *manageDialog) toMenu() {
	d.mode = manageMenu
	d.cur = 0
	d.rows = nil
	d.orphans = nil
	d.confirm, d.running, d.report, d.err = false, false, "", nil
}

// entries is the menu's rows; move and delete need a selected module.
func (d *manageDialog) entries() []string {
	e := []string{"create module"}
	if d.sel.moduleID != "" {
		e = append(e, "move module", "delete module")
	}
	return e
}

// layerLabels is the create/move layer vocabulary: the base layer plus
// this machine's host and user overlays.
func (d *manageDialog) layerLabels() []string {
	e := []string{"base"}
	if d.facts != nil {
		e = append(e, "hosts/"+d.facts.Hostname, "users/"+d.facts.Username)
	}
	return e
}

// layerValue translates a display label into the service op's layer
// string ("" is the base layer).
func layerValue(label string) string {
	if label == "base" {
		return ""
	}
	return label
}

func layerName(layer string) string {
	if layer == "" {
		return "base"
	}
	return layer
}

// currentLayer derives the selected module's layer from its directory:
// the module dir minus root minus "modules/<app>".
func (d *manageDialog) currentLayer() string {
	rel, err := filepath.Rel(d.root, d.sel.dir)
	if err != nil {
		return ""
	}
	layer := filepath.Dir(filepath.Dir(rel))
	if layer == "." {
		return ""
	}
	return filepath.ToSlash(layer)
}

func (d *manageDialog) HandleKey(key string) tea.Cmd {
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
	switch d.mode {
	case manageMenu:
		switch key {
		case "up":
			if d.cur > 0 {
				d.cur--
			}
		case "down":
			if d.cur < len(d.entries())-1 {
				d.cur++
			}
		case "enter":
			d.openEntry()
		}
	case manageCreate, manageMove:
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
			if d.mode == manageCreate && d.rows[0].field.String() == "" {
				return nil
			}
			d.confirm = true
		default:
			if r := []rune(key); len(r) == 1 {
				d.rows[d.cur].typeRune(r[0])
			}
		}
	}
	return nil
}

// back is the esc discipline (issue 0072): a confirm gate consumes esc
// (clears, stays), a form steps back to the menu, a bare menu lets the
// shell pop.
func (d *manageDialog) back() bool {
	if d.confirm {
		d.confirm = false
		return true
	}
	if d.mode != manageMenu {
		d.toMenu()
		return true
	}
	return false
}

// openEntry swaps the menu's entry in place: create/move build their
// rows, delete pre-computes the orphan preview and sits at its gate.
func (d *manageDialog) openEntry() {
	switch d.entries()[d.cur] {
	case "create module":
		d.mode = manageCreate
		d.cur = 0
		d.rows = []dlgRow{
			fieldRow(newDlgField("app", "")),
			choiceRow(newDlgChoice("layer", d.layerLabels()...)),
		}
	case "move module":
		d.mode = manageMove
		d.cur = 0
		from := layerName(d.currentLayer())
		var targets []string
		for _, l := range d.layerLabels() {
			if l != from {
				targets = append(targets, l)
			}
		}
		d.rows = []dlgRow{choiceRow(newDlgChoice("move to", targets...))}
	case "delete module":
		d.mode = manageDelete
		d.orphans = nil
		orphans, err := d.cfg.DeletePreview(d.sel.moduleID, d.currentLayer())
		if err != nil {
			d.err = err
		} else {
			d.orphans = orphans
		}
		d.confirm = true
	}
}

func (d *manageDialog) run() tea.Msg {
	var err error
	var report string
	switch d.mode {
	case manageCreate:
		app := d.rows[0].field.String()
		layer := layerValue(d.rows[1].choice.String())
		if err = d.cfg.CreateModule(app, layer); err == nil {
			report = "created module " + app + " in " + layerName(layer)
		}
	case manageMove:
		to := layerValue(d.rows[0].choice.String())
		if err = d.cfg.MoveModule(d.sel.moduleID, d.currentLayer(), to); err == nil {
			report = "moved module " + d.sel.moduleID + " to " + layerName(to)
		}
	case manageDelete:
		if err = d.cfg.DeleteModule(d.sel.moduleID, d.currentLayer()); err == nil {
			report = "deleted module " + d.sel.moduleID
		}
	}
	return writeFinishedMsg{key: manageKey, report: report, err: err}
}

func (d *manageDialog) applyFinished(m writeFinishedMsg) {
	d.running = false
	d.report, d.err = m.report, m.err
}

func (d *manageDialog) View(th theme) string {
	var b strings.Builder
	b.WriteString(th.viewTitle.Render("MODULES"))
	b.WriteString("\nmodule management")
	if d.sel.moduleID != "" {
		b.WriteString(" — " + d.sel.moduleID + " (" + layerName(d.currentLayer()) + ")")
	}
	b.WriteString("\n\n")
	switch d.mode {
	case manageMenu:
		for i, e := range d.entries() {
			marker := "  "
			if i == d.cur {
				marker = "> "
			}
			b.WriteString(th.Label(marker+e) + "\n")
		}
		b.WriteString("\n" + th.meta.Render("up/down pick · enter open · esc back"))
	case manageCreate, manageMove:
		for i, r := range d.rows {
			b.WriteString(r.render(i == d.cur))
			b.WriteString("\n")
		}
		b.WriteString(finishView(th, d.confirmText(), d.report, d.err, d.running))
		b.WriteString("\n" + th.meta.Render("up/down pick a row · type to edit · enter runs · esc back"))
	case manageDelete:
		d.writeOrphans(&b, th)
		b.WriteString(finishView(th, d.confirmText(), d.report, d.err, d.running))
		b.WriteString("\n" + th.meta.Render("y deletes · n/esc back"))
	}
	return b.String()
}

func (d *manageDialog) writeOrphans(b *strings.Builder, th theme) {
	b.WriteString(th.meta.Render("files nothing references (removed with the module):") + "\n")
	if len(d.orphans) == 0 {
		b.WriteString(th.Label("  (none)") + "\n")
		return
	}
	for _, o := range d.orphans {
		b.WriteString(th.Label("  "+o) + "\n")
	}
}

func (d *manageDialog) confirmText() string {
	switch d.mode {
	case manageCreate:
		return "create module \"" + d.rows[0].field.String() + "\" in " +
			layerName(layerValue(d.rows[1].choice.String())) + "?"
	case manageMove:
		return "move module \"" + d.sel.moduleID + "\" to " + d.rows[0].choice.String() + "?"
	case manageDelete:
		return "delete module \"" + d.sel.moduleID + "\"?"
	}
	return ""
}
