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
	"maps"
	"os"
	"slices"
	"strconv"
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
// Container rows (0074) own a structural entry (a unit, a secret, a
// mount, a share, a when group): they refuse field edits, d removes the
// entry, a adds `field = value` beneath them. Disclosure (0075): rows
// render only when they differ from the zero value — a header is
// selectable when it is the way in (an empty section, or the when/smb
// scope), and a hint row names the add gesture for an empty container.
type wsRow struct {
	section    string
	text       string
	header     bool
	selectable bool
	hint       bool
	family     string
	key        string
	value      string
	container  bool
	// noEdit marks a row that carries a family for removal and marking
	// but never opens a field edit (a block write: its payload is
	// multi-line file text, not a field). enter falls back to the add
	// form; d removes the entry (0087).
	noEdit bool
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
	bodyH   int      // the visible body height from the last layout: the paging step
	editing *wsEdit  // the active field/line input (T-tui-editing)
	draft   *wsDraft // the active layer's draft, if any

	placeholder string // pre-reader identity text (and no-reader fallback)
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

// selectableRow reports whether the cursor may rest on row i (0075):
// entry rows always; headers only when marked selectable (an empty
// section's header is the way in, and the when/smb headers are the
// sections' root scope); hint rows never.
func (w *workspaceModel) selectableRow(i int) bool {
	if i < 0 || i >= len(w.rows) {
		return false
	}
	r := w.rows[i]
	switch {
	case r.hint:
		return false
	case r.header:
		return r.selectable
	}
	return true
}

// move walks the cursor to the next selectable row in direction delta.
func (w *workspaceModel) move(delta int) {
	i := w.cursor
	for {
		i += delta
		if !w.selectableRow(i) && (i < 0 || i >= len(w.rows)) {
			return
		}
		if w.selectableRow(i) {
			w.cursor = i
			return
		}
	}
}

// page moves the cursor by a visible page of rows (the body height minus
// the title line), walking entry rows and clamping like move (0075
// T-tui-page).
func (w *workspaceModel) page(delta int) {
	for i, steps := 0, max(w.bodyH-1, 1); i < steps; i++ {
		before := w.cursor
		w.move(delta)
		if w.cursor == before {
			return
		}
	}
}

// home/end jump to the first/last selectable row.
func (w *workspaceModel) home() { w.cursor = w.firstEntry(0) }

func (w *workspaceModel) end() {
	for i := len(w.rows) - 1; i >= 0; i-- {
		if w.selectableRow(i) {
			w.cursor = i
			return
		}
	}
	w.cursor = 0
}

func (w *workspaceModel) firstEntry(from int) int {
	for i := from; i < len(w.rows); i++ {
		if w.selectableRow(i) {
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
	// An overlay tab states the merge rule (0082): only these families
	// merge; meta, scope, and when come from the base file, so editing
	// them here would be a silent no-op.
	if len(w.tabs) > 0 && w.tabs[w.active].layer != "base" {
		lines = append(lines, th.meta.MaxWidth(width).Render(
			"overlay: only packages, tools, dotfiles, hooks, mounts, smb merge; the rest comes from base"))
	}
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
		// The scroll window follows the cursor (0075 T-tui-page) — the
		// nav's rule: the offset clamps so the cursor row renders.
		if avail := h - len(lines); avail > 0 {
			if w.cursor < w.offset {
				w.offset = w.cursor
			}
			if w.cursor >= w.offset+avail {
				w.offset = w.cursor - avail + 1
			}
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
			tabs = append(tabs, th.sectionLabel.Render("["+t.label+"]"))
		} else {
			tabs = append(tabs, th.meta.Render(t.label))
		}
	}
	if len(tabs) > 0 {
		s += "  " + strings.Join(tabs, th.meta.Render(" · "))
	}
	// The cursor's section rides the title line (0076): an empty stretch
	// of surface still says where you are.
	if w.cursor < len(w.rows) {
		s += "  " + th.meta.Render("· "+w.rows[w.cursor].section)
	}
	if w.needsRoot {
		s += "  " + th.reasonMark.Render("needs root")
	}
	if w.draft.dirty() {
		s += "  " + th.dirtyMark.Render("●")
	}
	return s
}

// rowView renders one row (possibly several lines: the edit input and its
// tier-1 error render in place). Rows announce their gesture (0079
// T-tui-marks), computed from the same registry that decides behavior so
// the mark cannot lie: ✎ edits, ◂▸ cycles, ＋ grows. A committed edit's
// ● replaces the mark.
func (w *workspaceModel) rowView(r wsRow, i int, th theme, width int) []string {
	if r.header {
		text := r.text
		if addable(r.section) {
			text += " " + th.disabledMark.Render("＋")
		}
		if i == w.cursor {
			// A selectable header under the cursor renders the bar
			// (0076) — an empty section's header is the way in, it has
			// to say so.
			return []string{th.cursorRow.MaxWidth(width).Render(" " + text)}
		}
		return []string{th.sectionLabel.Render(text)}
	}
	if r.hint {
		return []string{th.disabledMark.MaxWidth(width).Render("  " + r.text)}
	}
	if w.editing != nil && w.editing.row == i {
		return w.editLines(w.editing, th, width)
	}
	text := r.text
	if w.rowDirty(r) {
		text += " " + th.dirtyMark.Render("●")
	} else {
		switch {
		case choiceSet(r) != nil:
			text += " " + th.disabledMark.Render("◂▸")
		case r.container:
			text += " " + th.disabledMark.Render("＋")
		case r.family != "" && !r.noEdit:
			text += " " + th.disabledMark.Render("✎")
		}
	}
	if w.draft != nil {
		if msg, bad := w.draft.errs[rowKey(r.family, r.key)]; bad {
			text += "  " + th.errorMark.Render("✗ "+msg)
		}
	}
	if i == w.cursor {
		return []string{th.cursorRow.MaxWidth(width).Render(" " + text)}
	}
	return []string{th.rowText.MaxWidth(width).Render("  " + text)}
}

// editLines renders the active input: the buffer with a cursor bar, plus
// the live tier-1 error beneath. The input row takes the cursor treatment
// (bar + accent, 0075).
func (w *workspaceModel) editLines(e *wsEdit, th theme, width int) []string {
	runes := e.input
	cur := min(e.cur, len(runes))
	shown := string(runes[:cur]) + "▏" + string(runes[cur:])
	lines := []string{th.cursorRow.MaxWidth(width).Render(" ▸ " + shown)}
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
// fixed and matches the schema docs. Disclosure (0075): a row renders
// only when it differs from the zero value — an unset field is nothing
// on the surface, an empty section is its header alone, and an empty
// container names the add gesture. The structural families (0074)
// render as container rows with their set field child rows.
func wsRows(cfg *profile.ModuleConfig, needsRoot bool) []wsRow {
	if cfg == nil {
		cfg = &profile.ModuleConfig{}
	}
	var rows []wsRow
	// A header is selectable when it is the way in: an empty section's
	// header takes the section's first entry, and a structural family's
	// header is its entry-level scope (`a` adds an entry there — the
	// containers add fields INTO their entries).
	section := func(name string, entries []wsRow) {
		rows = append(rows, wsRow{section: name, text: name, header: true,
			selectable: len(entries) == 0 || isStructuralFamily(addFamily(name))})
		rows = append(rows, entries...)
	}
	entry := func(section, text, family, key, value string) wsRow {
		return wsRow{section: section, text: text, family: family, key: key, value: value}
	}
	hint := func(section string) wsRow {
		return wsRow{section: section, text: `· a adds "field = value"`, hint: true}
	}

	var meta []wsRow
	if cfg.ID != "" {
		meta = append(meta, entry("meta", "id "+cfg.ID, "", "", ""))
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
			writes = append(writes, wsRow{section: "writes", text: target + " (edit: block)",
				family: profile.FamilyDotfiles, key: target, noEdit: true})
		}
	}
	section("links", links)
	section("writes", writes)

	var when []wsRow
	// The when tree (0074 grammar, 0075 disclosure): only set leaves
	// render; group containers always render (they exist in the config)
	// and an empty one shows the add hint. The when header is the root
	// scope — `a field = value` sets a leaf, `a and/or/not` grows groups.
	whenLeaf := func(depth int, group []string, field, value string) {
		if value == "" {
			return // an unset condition renders nothing
		}
		text := strings.Repeat("  ", depth) + field + " " + value
		segs := append(append([]string{}, group...), field)
		when = append(when, wsRow{
			section: "when", text: text,
			family: profile.FamilyWhen, key: pathKey(segs...), value: value,
		})
	}
	var renderGroup func(depth int, group []string, w *profile.When)
	renderGroup = func(depth int, group []string, w *profile.When) {
		label := group[len(group)-1]
		if label != "not" {
			label = group[len(group)-2] + "[" + label + "]"
		}
		before := len(when)
		when = append(when, wsRow{
			section: "when", text: strings.Repeat("  ", depth) + label,
			family: profile.FamilyWhen, key: pathKey(group...), container: true,
		})
		d := depth + 1
		whenLeaf(d, group, "hosts", strings.Join(w.Hosts, ", "))
		whenLeaf(d, group, "users", strings.Join(w.Users, ", "))
		whenLeaf(d, group, "os", strings.Join(w.OS, ", "))
		whenLeaf(d, group, "gpu", w.GPU)
		whenLeaf(d, group, "kernel", w.Kernel)
		whenLeaf(d, group, "packages", strings.Join(w.Packages, ", "))
		whenLeaf(d, group, "tools", strings.Join(w.Tools, ", "))
		for i := range w.And {
			renderGroup(d, append(append([]string{}, group...), "and", strconv.Itoa(i)), &w.And[i])
		}
		for i := range w.Or {
			renderGroup(d, append(append([]string{}, group...), "or", strconv.Itoa(i)), &w.Or[i])
		}
		if w.Not != nil {
			renderGroup(d, append(append([]string{}, group...), "not"), w.Not)
		}
		if len(when) == before+1 {
			when = append(when, hint("when"))
		}
	}
	whenLeaf(0, nil, "hosts", strings.Join(cfg.When.Hosts, ", "))
	whenLeaf(0, nil, "users", strings.Join(cfg.When.Users, ", "))
	whenLeaf(0, nil, "os", strings.Join(cfg.When.OS, ", "))
	whenLeaf(0, nil, "gpu", cfg.When.GPU)
	whenLeaf(0, nil, "kernel", cfg.When.Kernel)
	whenLeaf(0, nil, "packages", strings.Join(cfg.When.Packages, ", "))
	whenLeaf(0, nil, "tools", strings.Join(cfg.When.Tools, ", "))
	for i := range cfg.When.And {
		renderGroup(0, []string{"and", strconv.Itoa(i)}, &cfg.When.And[i])
	}
	for i := range cfg.When.Or {
		renderGroup(0, []string{"or", strconv.Itoa(i)}, &cfg.When.Or[i])
	}
	if cfg.When.Not != nil {
		renderGroup(0, []string{"not"}, cfg.When.Not)
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
		before := len(units)
		units = append(units, wsRow{
			section: "systemd.units", text: name,
			family: profile.FamilySystemd, key: name, container: true,
		})
		unit := cfg.Systemd.Units[name]
		for _, directive := range slices.Sorted(maps.Keys(unit)) {
			encoded := profile.EncodeTomlValue(unit[directive])
			if encoded == "" {
				continue // value shapes the encoder skips render nothing either
			}
			units = append(units, entry("systemd.units", "  "+directive+" = "+encoded,
				profile.FamilySystemd, pathKey(name, directive), editableTomlValue(unit[directive])))
		}
		if len(units) == before+1 {
			units = append(units, hint("systemd.units"))
		}
	}
	section("systemd.units", units)

	var tools []wsRow
	for _, name := range slices.Sorted(maps.Keys(cfg.Tools)) {
		tools = append(tools, entry("tools", name+" = "+cfg.Tools[name], profile.FamilyTools, name, cfg.Tools[name]))
	}
	section("tools", tools)

	// The structural entry sections (0074), disclosed (0075): one
	// container row per entry, its SET fields as indented child rows —
	// an unset field renders nothing, `a` on the container adds it.
	fieldRow := func(section, family, entry, field, value string) wsRow {
		text := "  " + field + " " + value
		return wsRow{section: section, text: text, family: family, key: pathKey(entry, field), value: value}
	}
	scalarRow := func(section, family, field, value string) wsRow {
		return wsRow{section: section, text: field + " " + value, family: family, key: field, value: value}
	}

	var secrets []wsRow
	for _, name := range slices.Sorted(maps.Keys(cfg.Secrets)) {
		s := cfg.Secrets[name]
		before := len(secrets)
		secrets = append(secrets, wsRow{
			section: "secrets", text: name,
			family: profile.FamilySecrets, key: name, container: true,
		})
		if s.Env != "" {
			secrets = append(secrets, fieldRow("secrets", profile.FamilySecrets, name, "env", s.Env))
		}
		if s.Description != "" {
			secrets = append(secrets, fieldRow("secrets", profile.FamilySecrets, name, "description", s.Description))
		}
		if s.AllowEmpty {
			secrets = append(secrets, fieldRow("secrets", profile.FamilySecrets, name, "allow_empty", "true"))
		}
		if len(secrets) == before+1 {
			secrets = append(secrets, hint("secrets"))
		}
	}
	section("secrets", secrets)

	var mounts []wsRow
	for _, name := range slices.Sorted(maps.Keys(cfg.Mounts)) {
		m := cfg.Mounts[name]
		before := len(mounts)
		mounts = append(mounts, wsRow{
			section: "mounts", text: name,
			family: profile.FamilyMounts, key: name, container: true,
		})
		if m.Source != "" {
			mounts = append(mounts, fieldRow("mounts", profile.FamilyMounts, name, "source", m.Source))
		}
		if m.Destination != "" {
			mounts = append(mounts, fieldRow("mounts", profile.FamilyMounts, name, "destination", m.Destination))
		}
		if m.Type != "" {
			mounts = append(mounts, fieldRow("mounts", profile.FamilyMounts, name, "type", m.Type))
		}
		if len(m.Options) > 0 {
			mounts = append(mounts, fieldRow("mounts", profile.FamilyMounts, name, "options", strings.Join(m.Options, ", ")))
		}
		if m.StartAt != "" {
			mounts = append(mounts, fieldRow("mounts", profile.FamilyMounts, name, "startat", m.StartAt))
		}
		if m.State != "" {
			mounts = append(mounts, fieldRow("mounts", profile.FamilyMounts, name, "state", m.State))
		}
		if len(mounts) == before+1 {
			mounts = append(mounts, hint("mounts"))
		}
	}
	section("mounts", mounts)

	var smb []wsRow
	if cfg.Smb.Group != "" {
		smb = append(smb, scalarRow("smb", profile.FamilySmb, "group", cfg.Smb.Group))
	}
	if len(cfg.Smb.Users) > 0 {
		smb = append(smb, scalarRow("smb", profile.FamilySmb, "users", strings.Join(cfg.Smb.Users, ", ")))
	}
	if cfg.Smb.Avahi != nil {
		smb = append(smb, scalarRow("smb", profile.FamilySmb, "avahi", strconv.FormatBool(*cfg.Smb.Avahi)))
	}
	for _, name := range slices.Sorted(maps.Keys(cfg.Smb.Shares)) {
		sh := cfg.Smb.Shares[name]
		before := len(smb)
		smb = append(smb, wsRow{
			section: "smb", text: name,
			family: profile.FamilySmb, key: name, container: true,
		})
		if sh.Path != "" {
			smb = append(smb, fieldRow("smb", profile.FamilySmb, name, "path", sh.Path))
		}
		if sh.Comment != "" {
			smb = append(smb, fieldRow("smb", profile.FamilySmb, name, "comment", sh.Comment))
		}
		if sh.ValidUsers != "" {
			smb = append(smb, fieldRow("smb", profile.FamilySmb, name, "valid_users", sh.ValidUsers))
		}
		if sh.Writable {
			smb = append(smb, fieldRow("smb", profile.FamilySmb, name, "writable", "true"))
		}
		if sh.Public {
			smb = append(smb, fieldRow("smb", profile.FamilySmb, name, "public", "true"))
		}
		if len(smb) == before+1 {
			smb = append(smb, hint("smb"))
		}
	}
	section("smb", smb)

	return rows
}
