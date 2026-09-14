package tui

// The M15 workspace (T-tui-workspace): module.toml rendered as a
// sectioned, status-annotated surface — meta, packages, links, writes,
// when, hooks, systemd.units, tools, and a trailing "other" group for the
// remaining schema sections. Rendering consumes the config area's strict
// decode plus raw text (the 0065 seam); the workspace never parses TOML
// itself. A broken file degrades to selectable raw-text rows naming the
// error. Rows carry their edit metadata (family, key, value) so
// T-tui-editing hangs the inline input off the same cursor; when a draft
// exists the surface renders from it, dirty-marked per row.

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

// LayerWriter is the save half of the seam (0065-D8's pipeline).
type LayerWriter interface {
	WriteModuleLayer(req service.SaveRequest) (*service.SaveResult, error)
}

// layerLoadedMsg carries one layer file read; stale reads (the cursor
// moved on) are dropped by dir.
type layerLoadedMsg struct {
	dir  string
	read *service.ModuleLayerRead
	err  error
}

// wsRow is one rendered row; the cursor walks entry rows only. family is
// the row's splice family ("raw" marks a raw-text line of a broken
// file), key its stable identity within the family, value the editable
// value without presentation decoration. Empty family means read-only.
type wsRow struct {
	section string
	text    string
	header  bool
	family  string
	key     string
	value   string
}

type workspaceModel struct {
	moduleID  string
	tabs      []navLayer // the module's layers in base/user/host order
	active    int        // the active tab
	needsRoot bool       // a superuser overlay exists (0029 classification)

	pending   bool
	loadErr   string
	schemaErr error  // *service.SchemaError: broken file, raw-text rows
	rawErr    string // the broken file's raw text
	read      *service.ModuleLayerRead
	loadedDir string // the dir the landed read belongs to

	rows    []wsRow
	cursor  int
	offset  int
	editing *wsEdit  // the active field/line input (T-tui-editing)
	draft   *wsDraft // the active layer's draft, if any

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

// atSection reports whether the cursor sits in the named section.
func (w *workspaceModel) atSection(name string) bool {
	return w.cursor < len(w.rows) && w.rows[w.cursor].section == name
}

// draftEdits returns the committed-edit marker set, nil without a draft.
func (w *workspaceModel) draftEdits() map[string]bool {
	if w.draft == nil {
		return nil
	}
	return w.draft.edited
}

// draftErrs returns the staged tier-1 errors, nil without a draft.
func (w *workspaceModel) draftErrs() map[string]string {
	if w.draft == nil {
		return nil
	}
	return w.draft.errs
}

// setRead applies a landed layer read and rebuilds the rows.
func (w *workspaceModel) setRead(r *service.ModuleLayerRead) {
	w.read = r
	w.loadedDir = r.Dir
	w.schemaErr = nil
	w.rawErr = ""
	w.rows = wsRows(r.Config, w.needsRoot)
	w.cursor = w.firstEntry(0)
	w.offset = 0
}

// setSchemaError applies the broken-file state: the error named, the raw
// text as selectable, line-editable rows (T-tui-editing's raw mode). The
// raw text is re-read from the named path — reading is not parsing.
func (w *workspaceModel) setSchemaError(err error) {
	w.read = nil
	w.schemaErr = err
	var se *service.SchemaError
	if errors.As(err, &se) && se.Path != "" {
		if raw, rerr := os.ReadFile(se.Path); rerr == nil {
			w.rawErr = string(raw)
		}
	}
	w.rows = rawRows(w.rawErr)
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
// the scroll window, with the active field input and staged errors
// rendered in place.
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
	default:
		if w.schemaErr != nil {
			// The error, width-clamped, above the raw rows; the path is
			// relative to the layer dir so the message survives
			// truncation.
			errText := w.schemaErr.Error()
			if dir := w.activeDir(); dir != "" {
				errText = strings.ReplaceAll(errText, dir+"/", "")
			}
			lines = append(lines, th.errorMark.MaxWidth(width).Render("broken module.toml — raw text mode"))
			lines = append(lines, th.rowText.MaxWidth(width).Render(th.meta.Render("  "+firstLineOf(errText))))
		}
		if w.read != nil && !w.read.Exists {
			lines = append(lines, th.disabledMark.Render("(no module.toml in this layer)"))
		}
		for i := w.offset; i < len(w.rows) && len(lines) < h; i++ {
			lines = append(lines, w.rowView(w.rows[i], i, th, width)...)
		}
	}
	return strings.Join(lines, "\n")
}

// titleView renders "demo  [base] · user cri · host myhost": the module
// id, the tab bar with the active tab bracketed, and the needs-root and
// draft markers.
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
	if w.draft != nil {
		s += "  " + th.dirtyMark.Render("●")
	}
	return s
}

// rowView renders one row (possibly several lines: the edit input and its
// tier-1 error render in place).
func (w *workspaceModel) rowView(r wsRow, i int, th theme, width int) []string {
	if r.header {
		return []string{th.sectionLabel.Render(r.text)}
	}
	if strings.HasPrefix(r.text, "(none)") {
		return []string{th.disabledMark.MaxWidth(width).Render("  " + r.text)}
	}
	if w.editing != nil && w.editing.row == i && !w.editing.add {
		return w.editLines(w.editing, th, width)
	}
	text := "  " + r.text
	if w.rowDirty(r) {
		text += " " + th.dirtyMark.Render("●")
	}
	if w.draft != nil {
		if msg, bad := w.draft.errs[rowKey(r.family, r.key)]; bad {
			text += "  " + th.errorMark.Render("✗ "+msg)
		}
	}
	if i == w.cursor {
		return []string{th.selection.MaxWidth(width).Render(text)}
	}
	return []string{th.rowText.MaxWidth(width).Render(text)}
}

// editLines renders the active input: the buffer with a cursor bar, plus
// the live tier-1 error beneath.
func (w *workspaceModel) editLines(e *wsEdit, th theme, width int) []string {
	runes := e.input
	cur := min(e.cur, len(runes))
	shown := string(runes[:cur]) + "▏" + string(runes[cur:])
	prefix := "  ▸ "
	if e.add {
		prefix = "  + "
	}
	lines := []string{th.selection.MaxWidth(width).Render(prefix + shown)}
	if e.err != "" {
		lines = append(lines, th.errorMark.MaxWidth(width).Render("    ✗ "+e.err))
	}
	return lines
}

// rowDirty: the draft marks this row edited (committed edits, adds).
func (w *workspaceModel) rowDirty(r wsRow) bool {
	if w.draft == nil || r.family == "" {
		return false
	}
	return w.draft.edited[rowKey(r.family, r.key)]
}

// firstLineOf returns s up to the first newline.
func firstLineOf(s string) string {
	if i := strings.IndexByte(s, '\n'); i >= 0 {
		return s[:i]
	}
	return s
}

// wsRows builds the section rows for one layer's config, carrying each
// editable row's family/key/value for T-tui-editing. Section order is
// fixed and matches the schema docs; schema sections outside the list
// (secrets, mounts, smb) render in a trailing "other" group rather than
// being dropped.
func wsRows(cfg *profile.ModuleConfig, needsRoot bool) []wsRow {
	if cfg == nil {
		cfg = &profile.ModuleConfig{}
	}
	var rows []wsRow
	section := func(name string, entries []wsRow) {
		rows = append(rows, wsRow{section: name, text: name, header: true})
		if len(entries) == 0 {
			rows = append(rows, wsRow{section: name, text: "(none)"})
			return
		}
		rows = append(rows, entries...)
	}
	entry := func(section, text, family, key, value string) wsRow {
		return wsRow{section: section, text: text, family: family, key: key, value: value}
	}

	var meta []wsRow
	if cfg.ID != "" {
		meta = append(meta, entry("meta", "id "+cfg.ID, "", "", ""))
	}
	if cfg.App != "" && cfg.App != cfg.ID {
		meta = append(meta, entry("meta", "app "+cfg.App, profile.FamilyKeys, "app", cfg.App))
	}
	meta = append(meta, entry("meta", "description "+cfg.Description, profile.FamilyKeys, "description", cfg.Description))
	meta = append(meta, entry("meta", "scope "+cfg.ScopeOrDefault(), profile.FamilyKeys, "scope", cfg.Scope))
	if cfg.Disabled {
		meta = append(meta, entry("meta", "disabled", "", "", ""))
	}
	section("meta", meta)

	var pkgs []wsRow
	for _, p := range cfg.Packages.Present {
		pkgs = append(pkgs, entry("packages", "+ "+p, profile.FamilyPackages, "present:"+p, p))
	}
	for _, p := range cfg.Packages.Absent {
		pkgs = append(pkgs, entry("packages", "− "+p, profile.FamilyPackages, "absent:"+p, "-"+p))
	}
	section("packages", pkgs)

	var links, writes []wsRow
	for _, target := range slices.Sorted(maps.Keys(cfg.Dotfiles)) {
		d := cfg.Dotfiles[target]
		switch {
		case d.Source != "":
			links = append(links, entry("links", target+" ← "+d.Source+" ("+d.Mode+")",
				profile.FamilyDotfiles, target, d.Source))
		case d.Line != "":
			writes = append(writes, entry("writes", target+" (edit: line)",
				profile.FamilyDotfiles, target, d.Line))
		case d.Block != "":
			writes = append(writes, entry("writes", target+" (edit: block)", "", target, ""))
		}
	}
	section("links", links)
	section("writes", writes)

	var when []wsRow
	listRow := func(name string, vs []string) {
		if len(vs) > 0 {
			when = append(when, entry("when", name+" "+strings.Join(vs, ", "),
				profile.FamilyWhen, name, strings.Join(vs, ", ")))
		}
	}
	listRow("hosts", cfg.When.Hosts)
	listRow("users", cfg.When.Users)
	listRow("os", cfg.When.OS)
	if cfg.When.GPU != "" {
		when = append(when, entry("when", "gpu "+cfg.When.GPU, profile.FamilyWhen, "gpu", cfg.When.GPU))
	}
	if cfg.When.Kernel != "" {
		when = append(when, entry("when", "kernel "+cfg.When.Kernel, profile.FamilyWhen, "kernel", cfg.When.Kernel))
	}
	listRow("packages", cfg.When.Packages)
	listRow("tools", cfg.When.Tools)
	if len(cfg.When.And) > 0 {
		when = append(when, entry("when", fmt.Sprintf("and (%d conditions)", len(cfg.When.And)), "", "", ""))
	}
	if len(cfg.When.Or) > 0 {
		when = append(when, entry("when", fmt.Sprintf("or (%d conditions)", len(cfg.When.Or)), "", "", ""))
	}
	if cfg.When.Not != nil {
		when = append(when, entry("when", "not (1 condition)", "", "", ""))
	}
	section("when", when)

	var hooks []wsRow
	for _, h := range cfg.Hooks.Pre {
		hooks = append(hooks, entry("hooks", "pre: "+h.Command, profile.FamilyHooks, "pre:"+h.Command, h.Command))
	}
	for _, h := range cfg.Hooks.Post {
		hooks = append(hooks, entry("hooks", "post: "+h.Command, profile.FamilyHooks, "post:"+h.Command, h.Command))
	}
	section("hooks", hooks)

	var units []wsRow
	for _, name := range slices.Sorted(maps.Keys(cfg.Systemd.Units)) {
		units = append(units, entry("systemd.units", name, "", "", ""))
	}
	section("systemd.units", units)

	var tools []wsRow
	for _, name := range slices.Sorted(maps.Keys(cfg.Tools)) {
		tools = append(tools, entry("tools", name+" = "+cfg.Tools[name], profile.FamilyTools, name, cfg.Tools[name]))
	}
	section("tools", tools)

	var other []wsRow
	for _, name := range slices.Sorted(maps.Keys(cfg.Secrets)) {
		s := cfg.Secrets[name]
		other = append(other, entry("other", "secret "+name+" (env "+s.Env+")", "", "", ""))
	}
	for _, name := range slices.Sorted(maps.Keys(cfg.Mounts)) {
		m := cfg.Mounts[name]
		other = append(other, entry("other", "mount "+name+" → "+m.Destination, "", "", ""))
	}
	for _, name := range slices.Sorted(maps.Keys(cfg.Smb.Shares)) {
		other = append(other, entry("other", "smb share "+name, "", "", ""))
	}
	section("other", other)

	return rows
}
