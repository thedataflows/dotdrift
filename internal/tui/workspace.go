package tui

// The M15 workspace (T-tui-workspace): module.toml rendered as a
// sectioned, status-annotated surface — meta, packages, links, writes,
// when, hooks, systemd.units, tools, and a trailing "other" group for the
// remaining schema sections. Rendering consumes the config area's strict
// decode plus raw text (the 0065 seam); the workspace never parses TOML
// itself. A broken file degrades to a read-only raw-text mode naming the
// error. The row cursor walks entry rows so T-tui-editing hangs edit
// mode off the same cursor.

import (
	"errors"
	"fmt"
	"maps"
	"os"
	"slices"
	"strings"

	"github.com/thedataflows/dotdrift/internal/profile"
	"github.com/thedataflows/dotdrift/internal/service"
)

// LayerReader is the 0065 seam the workspace reads through (ADR-0008's
// doorway), satisfied by *service.ConfigArea.
type LayerReader interface {
	ReadModuleLayer(dir string) (*service.ModuleLayerRead, error)
}

// layerLoadedMsg carries one layer file read; stale reads (the cursor
// moved on) are dropped by dir.
type layerLoadedMsg struct {
	dir  string
	read *service.ModuleLayerRead
	err  error
}

// wsRow is one rendered row; the cursor walks entry rows only.
type wsRow struct {
	section string
	text    string
	header  bool
}

type workspaceModel struct {
	moduleID  string
	tabs      []navLayer // the module's layers in base/user/host order
	active    int        // the active tab
	needsRoot bool       // a superuser overlay exists (0029 classification)

	pending   bool
	loadErr   string
	schemaErr error  // *service.SchemaError: broken file, read-only
	rawErr    string // the broken file's raw text
	read      *service.ModuleLayerRead
	loadedDir string // the dir the landed read belongs to

	rows   []wsRow
	cursor int
	offset int

	placeholder string // pre-reader identity text (and no-reader fallback)
}

// placeholderOrBody is the assertion seam for tests: the placeholder text
// or the joined row texts.
func (w *workspaceModel) placeholderOrBody() string {
	if len(w.rows) == 0 {
		return w.placeholder
	}
	var b strings.Builder
	for _, r := range w.rows {
		b.WriteString(r.text + "\n")
	}
	return b.String()
}

// activeDir is the directory of the active layer tab.
func (w *workspaceModel) activeDir() string {
	if w.active >= len(w.tabs) {
		return ""
	}
	return w.tabs[w.active].dir
}

// setRead applies a landed layer read and rebuilds the rows.
func (w *workspaceModel) setRead(r *service.ModuleLayerRead) {
	w.read = r
	w.loadedDir = r.Dir
	w.schemaErr = nil
	w.rawErr = ""
	w.rows = wsRows(r, w.needsRoot)
	w.cursor = w.firstEntry(0)
	w.offset = 0
}

// setSchemaError applies the broken-file state: read-only, error named,
// raw text visible (0065 behavior). The raw text is re-read from the
// named path — reading is not parsing.
func (w *workspaceModel) setSchemaError(err error) {
	w.read = nil
	w.schemaErr = err
	var se *service.SchemaError
	if errors.As(err, &se) && se.Path != "" {
		if raw, rerr := os.ReadFile(se.Path); rerr == nil {
			w.rawErr = string(raw)
		}
	}
	w.rows = nil
	w.cursor = 0
}

// move walks the cursor to the next entry row in direction delta.
func (w *workspaceModel) move(delta int) {
	i := w.cursor
	for {
		i += delta
		if i < 0 || i >= len(w.rows) {
			return
		}
		if !w.rows[i].header && !strings.HasPrefix(w.rows[i].text, "(none)") {
			w.cursor = i
			return
		}
	}
}

func (w *workspaceModel) firstEntry(from int) int {
	for i := from; i < len(w.rows); i++ {
		if !w.rows[i].header && !strings.HasPrefix(w.rows[i].text, "(none)") {
			return i
		}
	}
	return 0
}

// view renders the surface: the title + tab bar line, then the rows in
// the scroll window.
func (w *workspaceModel) view(width, h int, th theme) string {
	if w.placeholder != "" && w.read == nil && !w.pending && w.schemaErr == nil && w.loadErr == "" {
		return w.placeholder
	}
	var lines []string
	lines = append(lines, w.titleView(th))
	switch {
	case w.pending:
		lines = append(lines, th.loading.Render("loading "+w.moduleID+"…"))
	case w.loadErr != "":
		lines = append(lines, th.errorMark.Render("load failed: "+w.loadErr))
	case w.schemaErr != nil:
		// Every line is width-clamped: parse errors carry a multi-line
		// source snippet and raw text is arbitrary — neither may break
		// the pane border. The path is shown relative to the layer dir
		// so the message itself survives truncation.
		clamp := func(s string) string { return th.rowText.MaxWidth(width).Render(s) }
		errText := w.schemaErr.Error()
		if dir := w.activeDir(); dir != "" {
			errText = strings.ReplaceAll(errText, dir+"/", "")
		}
		lines = append(lines, th.errorMark.MaxWidth(width).Render("broken module.toml — read-only"))
		for _, l := range strings.Split(errText, "\n") {
			lines = append(lines, clamp(th.meta.Render("  "+l)))
		}
		if w.rawErr != "" {
			lines = append(lines, "")
			for _, l := range strings.Split(strings.TrimRight(w.rawErr, "\n"), "\n") {
				lines = append(lines, clamp(l))
			}
		}
	default:
		if w.read != nil && !w.read.Exists {
			lines = append(lines, th.disabledMark.Render("(no module.toml in this layer)"))
		}
		for i := w.offset; i < len(w.rows) && len(lines) < h; i++ {
			lines = append(lines, w.rowView(w.rows[i], i == w.cursor, th, width))
		}
	}
	return strings.Join(lines, "\n")
}

// titleView renders "demo — [base] · user cri · host myhost": the module
// id, the tab bar with the active tab bracketed, and the needs-root
// marker.
func (w *workspaceModel) titleView(th theme) string {
	if w.moduleID == "" {
		return ""
	}
	s := th.viewTitle.Render(w.moduleID)
	var tabs []string
	for i, t := range w.tabs {
		if i == w.active {
			tabs = append(tabs, th.selection.Render("["+t.label+"]"))
		} else {
			tabs = append(tabs, th.meta.Render(t.label))
		}
	}
	if len(tabs) > 0 {
		s += "  " + strings.Join(tabs, th.meta.Render(" · "))
	}
	if w.needsRoot {
		s += "  " + th.reasonMark.Render("needs root")
	}
	return s
}

func (w *workspaceModel) rowView(r wsRow, cursor bool, th theme, width int) string {
	if r.header {
		return th.sectionLabel.Render(r.text)
	}
	if strings.HasPrefix(r.text, "(none)") {
		return th.disabledMark.MaxWidth(width).Render("  " + r.text)
	}
	if cursor {
		return th.selection.MaxWidth(width).Render("  " + r.text)
	}
	return th.rowText.MaxWidth(width).Render("  " + r.text)
}

// wsRows builds the section rows for one layer's config. Section order is
// fixed and matches the schema docs; schema sections outside the list
// (secrets, mounts, smb) render in a trailing "other" group rather than
// being dropped.
func wsRows(r *service.ModuleLayerRead, needsRoot bool) []wsRow {
	cfg := r.Config
	if cfg == nil {
		cfg = &profile.ModuleConfig{}
	}
	var rows []wsRow
	section := func(name string, entries []string) {
		rows = append(rows, wsRow{section: name, text: name, header: true})
		if len(entries) == 0 {
			rows = append(rows, wsRow{section: name, text: "(none)"})
			return
		}
		for _, e := range entries {
			rows = append(rows, wsRow{section: name, text: e})
		}
	}

	var meta []string
	if cfg.ID != "" {
		meta = append(meta, "id "+cfg.ID)
	}
	if cfg.App != "" && cfg.App != cfg.ID {
		meta = append(meta, "app "+cfg.App)
	}
	if cfg.Description != "" {
		meta = append(meta, cfg.Description)
	}
	if cfg.Scope != "" {
		meta = append(meta, "scope "+cfg.ScopeOrDefault())
	}
	if cfg.Disabled {
		meta = append(meta, "disabled")
	}
	section("meta", meta)

	var pkgs []string
	for _, p := range cfg.Packages.Present {
		pkgs = append(pkgs, "+ "+p)
	}
	for _, p := range cfg.Packages.Absent {
		pkgs = append(pkgs, "− "+p)
	}
	section("packages", pkgs)

	var links, writes []string
	for _, target := range slices.Sorted(maps.Keys(cfg.Dotfiles)) {
		d := cfg.Dotfiles[target]
		switch {
		case d.Source != "":
			links = append(links, target+" ← "+d.Source+" ("+d.Mode+")")
		case d.Line != "":
			writes = append(writes, target+" (edit: line)")
		case d.Block != "":
			writes = append(writes, target+" (edit: block)")
		}
	}
	section("links", links)
	section("writes", writes)
	section("when", whenRows(cfg.When))

	var hooks []string
	for _, h := range cfg.Hooks.Pre {
		hooks = append(hooks, "pre: "+h.Command)
	}
	for _, h := range cfg.Hooks.Post {
		hooks = append(hooks, "post: "+h.Command)
	}
	section("hooks", hooks)

	var units []string
	for _, name := range slices.Sorted(maps.Keys(cfg.Systemd.Units)) {
		units = append(units, name)
	}
	section("systemd.units", units)

	var tools []string
	for _, name := range slices.Sorted(maps.Keys(cfg.Tools)) {
		tools = append(tools, name+" = "+cfg.Tools[name])
	}
	section("tools", tools)

	var other []string
	for _, name := range slices.Sorted(maps.Keys(cfg.Secrets)) {
		s := cfg.Secrets[name]
		other = append(other, "secret "+name+" (env "+s.Env+")")
	}
	for _, name := range slices.Sorted(maps.Keys(cfg.Mounts)) {
		m := cfg.Mounts[name]
		other = append(other, "mount "+name+" → "+m.Destination)
	}
	for _, name := range slices.Sorted(maps.Keys(cfg.Smb.Shares)) {
		other = append(other, "smb share "+name)
	}
	section("other", other)

	return rows
}

// whenRows renders the top-level conditions; nested groups render as
// counts (the full tree builder is an explicit non-goal).
func whenRows(w profile.When) []string {
	var out []string
	list := func(name string, vs []string) {
		if len(vs) > 0 {
			out = append(out, name+" "+strings.Join(vs, ", "))
		}
	}
	list("hosts", w.Hosts)
	list("users", w.Users)
	list("os", w.OS)
	if w.GPU != "" {
		out = append(out, "gpu "+w.GPU)
	}
	if w.Kernel != "" {
		out = append(out, "kernel "+w.Kernel)
	}
	list("packages", w.Packages)
	list("tools", w.Tools)
	if len(w.And) > 0 {
		out = append(out, fmt.Sprintf("and (%d conditions)", len(w.And)))
	}
	if len(w.Or) > 0 {
		out = append(out, fmt.Sprintf("or (%d conditions)", len(w.Or)))
	}
	if w.Not != nil {
		out = append(out, "not (1 condition)")
	}
	return out
}
