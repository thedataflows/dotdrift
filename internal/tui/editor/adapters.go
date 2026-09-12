package editor

import (
	"maps"
	"reflect"
	"slices"
	"strings"

	"github.com/thedataflows/dotdrift/internal/profile"
	"github.com/thedataflows/dotdrift/internal/service"
)

// The form-shaped section adapters (0065-D6/D9): module keys, packages,
// tools, secrets. Validators reuse profile's/resolve's own checks — never a
// second schema. The custom models (dotfiles, when, hooks, systemd) live in
// their own files.

// buildAdapters builds the section adapters over a draft read, in tab
// order. `bootstrap.users` is deliberately absent: it is emitted mise
// config, not editable schema (0065-D3).
func buildAdapters(r *service.ModuleLayerRead) []Adapter {
	cfg := r.Config
	return []Adapter{
		newKeysAdapter(cfg),
		newPackagesAdapter(cfg),
		newToolsAdapter(cfg),
		newSecretsAdapter(cfg),
		newDotfilesAdapter(cfg),
		newWhenAdapter(cfg),
		newHooksAdapter(cfg),
		newMountsAdapter(cfg),
		newSmbAdapter(cfg),
		newSystemdAdapter(cfg),
	}
}

// field is one single-line text field with a tiny built-in line editor
// (value runes, cursor) — deterministic to render and to test.
type field struct {
	key      string
	label    string
	value    []rune
	cursor   int
	edit     bool
	baseline string
}

func newField(key, label, value string) field {
	return field{key: key, label: label, value: []rune(value), baseline: value}
}

// set replaces the value; dirty comparison is against the baseline.
func (fl *field) set(s string) {
	fl.value = []rune(s)
	fl.clampCursor()
}

func (fl *field) String() string { return string(fl.value) }

// Changed reports whether the field differs from its baseline.
func (fl *field) Changed() bool { return fl.baseline != fl.String() }

// reset restores the baseline value.
func (fl *field) reset() {
	fl.set(fl.baseline)
	fl.endEdit()
}

func (fl *field) beginEdit() { fl.edit = true; fl.clampCursor() }
func (fl *field) endEdit()   { fl.edit = false }

func (fl *field) clampCursor() {
	if fl.cursor > len(fl.value) {
		fl.cursor = len(fl.value)
	}
	if fl.cursor < 0 {
		fl.cursor = 0
	}
}

// type inserts a rune at the cursor while editing.
func (fl *field) typeRune(r rune) {
	if !fl.edit {
		return
	}
	fl.value = append(fl.value[:fl.cursor], append([]rune{r}, fl.value[fl.cursor:]...)...)
	fl.cursor++
}

func (fl *field) backspace() {
	if !fl.edit || fl.cursor == 0 {
		return
	}
	fl.value = append(fl.value[:fl.cursor-1], fl.value[fl.cursor:]...)
	fl.cursor--
	fl.clampCursor()
}

func (fl *field) left() {
	if fl.edit && fl.cursor > 0 {
		fl.cursor--
	}
}

func (fl *field) right() {
	if fl.edit && fl.cursor < len(fl.value) {
		fl.cursor++
	}
}

// handleEditKey applies the editing vocabulary to one field: returns true
// when the key was consumed.
func (fl *field) handleEditKey(key string) bool {
	switch {
	case key == "enter":
		fl.endEdit()
		return true
	case key == "esc":
		fl.set(fl.baseline) // cancel reverts the field
		fl.endEdit()
		return true
	case key == "backspace":
		fl.backspace()
		return true
	case key == "left":
		fl.left()
		return true
	case key == "right":
		fl.right()
		return true
	case len(key) == 1:
		fl.typeRune([]rune(key)[0])
		return true
	}
	return false
}

// keysAdapter edits the module-level keys — the preamble family.
type keysAdapter struct {
	id, app, description field
	disabled             bool
	disabledBaseline     bool
	scope                string
	scopeBaseline        string
}

func newKeysAdapter(cfg *profile.ModuleConfig) *keysAdapter {
	a := &keysAdapter{
		id:               newField("id", "id", cfg.ID),
		app:              newField("app", "app", cfg.App),
		description:      newField("description", "description", cfg.Description),
		disabled:         cfg.Disabled,
		disabledBaseline: cfg.Disabled,
		scope:            cfg.Scope,
		scopeBaseline:    cfg.Scope,
	}
	if a.scope == "" {
		a.scopeBaseline = ""
	}
	return a
}

func (a *keysAdapter) ID() string     { return "keys" }
func (a *keysAdapter) Title() string  { return "module keys" }
func (a *keysAdapter) Family() string { return service.FamilyKeys }

func (a *keysAdapter) config() profile.ModuleConfig {
	return profile.ModuleConfig{
		ID:          a.id.String(),
		App:         a.app.String(),
		Description: a.description.String(),
		Disabled:    a.disabled,
		Scope:       a.scope,
	}
}

func (a *keysAdapter) baseline() profile.ModuleConfig {
	return profile.ModuleConfig{
		ID: a.id.baseline, App: a.app.baseline, Description: a.description.baseline,
		Disabled: a.disabledBaseline, Scope: a.scopeBaseline,
	}
}

// Dirty deep-compares the draft keys against the baseline (0065-D8).
func (a *keysAdapter) Dirty() bool {
	return !reflect.DeepEqual(a.config(), a.baseline())
}

func (a *keysAdapter) Block() string {
	return profile.EncodeKeysSection(a.config())
}

// Errors reuses resolve's scope vocabulary (resolve.go: "unknown scope",
// valid: user, system) as the live check.
func (a *keysAdapter) Errors() []string {
	var out []string
	if s := a.scope; s != "" && s != profile.ScopeUser && s != profile.ScopeSystem {
		out = append(out, "unknown scope \""+s+"\" (valid: user, system)")
	}
	return out
}

func (a *keysAdapter) HandleKey(key string) {
	for _, fl := range []*field{&a.id, &a.app, &a.description} {
		if fl.edit {
			fl.handleEditKey(key)
			return
		}
	}
	switch key {
	case "1":
		a.id.beginEdit()
	case "2":
		a.app.beginEdit()
	case "3":
		a.description.beginEdit()
	case "4":
		a.disabled = !a.disabled
	case "5":
		a.scope = nextScope(a.scope)
	}
}

// nextScope cycles the scope choice: "" → user → system → "" (the
// vocabulary resolve accepts).
func nextScope(s string) string {
	switch s {
	case "":
		return profile.ScopeUser
	case profile.ScopeUser:
		return profile.ScopeSystem
	default:
		return ""
	}
}

func (a *keysAdapter) View(p Palette) string {
	var b strings.Builder
	b.WriteString(p.Meta("1") + " id          " + renderField(p, &a.id) + "\n")
	b.WriteString(p.Meta("2") + " app         " + renderField(p, &a.app) + "\n")
	b.WriteString(p.Meta("3") + " description " + renderField(p, &a.description) + "\n")
	b.WriteString(p.Meta("4") + " disabled    " + renderBool(p, a.disabled, a.disabledBaseline) + "\n")
	b.WriteString(p.Meta("5") + " scope       " + renderChoice(p, a.scope, a.scopeBaseline))
	if note := a.scopeNote(); note != "" {
		b.WriteString("\n" + p.Meta(note))
	}
	return b.String()
}

func (a *keysAdapter) scopeNote() string {
	return "scope: user → dotfiles as you; system → with sudo"
}

func renderField(p Palette, fl *field) string {
	s := fl.String()
	if fl.edit {
		s = s[:fl.cursor] + "▌" + s[fl.cursor:]
		return p.Mark(s)
	}
	if fl.Changed() {
		return p.Mark(s)
	}
	return s
}

func renderBool(p Palette, v, baseline bool) string {
	s := "false"
	if v {
		s = "true"
	}
	if v != baseline {
		return p.Mark(s)
	}
	return s
}

func renderChoice(p Palette, v, baseline string) string {
	s := v
	if s == "" {
		s = "(user default)"
	}
	if v != baseline {
		return p.Mark(s)
	}
	return s
}

// packagesAdapter edits the ordered present/absent package lists.
type packagesAdapter struct {
	rows       []pkgRow
	baseline   []pkgRow
	cur        int
	prompting  bool
	prompt     field
	listAbsent bool // the prompt's target list toggle
}

type pkgRow struct {
	Name   string
	Absent bool
}

func newPackagesAdapter(cfg *profile.ModuleConfig) *packagesAdapter {
	a := &packagesAdapter{}
	for _, name := range cfg.Packages.Present {
		a.rows = append(a.rows, pkgRow{Name: name})
	}
	for _, name := range cfg.Packages.Absent {
		a.rows = append(a.rows, pkgRow{Name: name, Absent: true})
	}
	a.baseline = slices.Clone(a.rows)
	return a
}

func (a *packagesAdapter) ID() string     { return "packages" }
func (a *packagesAdapter) Title() string  { return "packages" }
func (a *packagesAdapter) Family() string { return service.FamilyPackages }

func (a *packagesAdapter) Dirty() bool { return !reflect.DeepEqual(a.rows, a.baseline) }

func (a *packagesAdapter) Block() string {
	var present []profile.PackageEntry
	var absent []string
	for _, r := range a.rows {
		if r.Absent {
			absent = append(absent, r.Name)
		} else {
			present = append(present, profile.PackageEntry{Name: r.Name})
		}
	}
	return profile.EncodePackagesSection(present, absent)
}

func (a *packagesAdapter) Errors() []string {
	var out []string
	for _, r := range a.rows {
		if strings.TrimSpace(r.Name) == "" {
			out = append(out, "package name is required")
		}
	}
	return out
}

func (a *packagesAdapter) HandleKey(key string) {
	if a.prompting {
		if a.prompt.handleEditKey(key) {
			if !a.prompt.edit { // committed
				if name := strings.TrimSpace(a.prompt.String()); name != "" {
					a.rows = append(a.rows, pkgRow{Name: name, Absent: a.listAbsent})
				}
				a.prompting = false
			}
		}
		return
	}
	switch {
	case key == "j" || key == "down":
		if a.cur < len(a.rows)-1 {
			a.cur++
		}
	case key == "k" || key == "up":
		if a.cur > 0 {
			a.cur--
		}
	case key == "n":
		a.prompting = true
		a.listAbsent = false
		a.prompt = newField("name", "package", "")
		a.prompt.beginEdit()
	case key == "x":
		if a.cur < len(a.rows) {
			a.rows = append(a.rows[:a.cur], a.rows[a.cur+1:]...)
			if a.cur >= len(a.rows) && a.cur > 0 {
				a.cur--
			}
		}
	case key == " ":
		if a.cur < len(a.rows) {
			a.rows[a.cur].Absent = !a.rows[a.cur].Absent
		}
	case key == "<":
		if a.cur > 0 {
			a.rows[a.cur-1], a.rows[a.cur] = a.rows[a.cur], a.rows[a.cur-1]
			a.cur--
		}
	case key == ">":
		if a.cur < len(a.rows)-1 {
			a.rows[a.cur+1], a.rows[a.cur] = a.rows[a.cur], a.rows[a.cur+1]
			a.cur++
		}
	}
}

func (a *packagesAdapter) View(p Palette) string {
	if a.prompting {
		return p.Meta("package name: ") + renderField(p, &a.prompt) + p.Meta("  enter confirm · esc cancel")
	}
	if len(a.rows) == 0 {
		return p.Meta("no packages — n adds one")
	}
	var b strings.Builder
	for i, r := range a.rows {
		cur := "  "
		if i == a.cur {
			cur = "> "
		}
		mark := "+"
		if r.Absent {
			mark = "−"
		}
		line := cur + mark + " " + r.Name
		if i == a.cur {
			line = p.Mark(line)
		}
		b.WriteString(line + "\n")
	}
	return b.String()
}

// toolsAdapter edits the [tools] map (name → version).
type toolsAdapter struct {
	names    []string // sorted
	values   map[string]field
	baseline map[string]string
	cur      int
	editing  bool
}

func newToolsAdapter(cfg *profile.ModuleConfig) *toolsAdapter {
	a := &toolsAdapter{
		values:   map[string]field{},
		baseline: map[string]string{},
	}
	for k, v := range cfg.Tools {
		a.values[k] = newField(k, k, v)
		a.baseline[k] = v
		a.names = append(a.names, k)
	}
	a.names = slices.Sorted(maps.Keys(a.values))
	return a
}

func (a *toolsAdapter) ID() string     { return "tools" }
func (a *toolsAdapter) Title() string  { return "tools" }
func (a *toolsAdapter) Family() string { return service.FamilyTools }

func (a *toolsAdapter) Dirty() bool {
	if len(a.values) != len(a.baseline) {
		return true
	}
	for k, fl := range a.values {
		base, ok := a.baseline[k]
		if !ok || base != fl.String() {
			return true
		}
	}
	return false
}

func (a *toolsAdapter) Block() string {
	tools := make(map[string]string, len(a.values))
	for k := range a.values {
		fl := a.values[k]
		tools[k] = fl.String()
	}
	return profile.EncodeToolsSection(tools)
}

func (a *toolsAdapter) Errors() []string {
	var out []string
	for _, k := range a.names {
		fl := a.values[k]
		if strings.TrimSpace(fl.String()) == "" {
			out = append(out, "tool "+k+" needs a version")
		}
	}
	return out
}

func (a *toolsAdapter) View(p Palette) string {
	if len(a.names) == 0 {
		return p.Meta("no tools — declare them with generate or edit module.toml")
	}
	var b strings.Builder
	for i, k := range a.names {
		cur := "  "
		if i == a.cur {
			cur = "> "
		}
		fl := a.values[k]
		line := cur + k + " = " + fl.String()
		if i == a.cur {
			line = p.Mark(line)
		}
		b.WriteString(line + "\n")
	}
	return b.String()
}

func (a *toolsAdapter) HandleKey(key string) {
	if a.editing && a.cur < len(a.names) {
		fl := a.values[a.names[a.cur]]
		if fl.handleEditKey(key) {
			a.values[a.names[a.cur]] = fl
			if !fl.edit {
				a.editing = false
			}
			return
		}
	}
	switch {
	case key == "j" || key == "down":
		if a.cur < len(a.names)-1 {
			a.cur++
		}
	case key == "k" || key == "up":
		if a.cur > 0 {
			a.cur--
		}
	case key == "enter":
		if a.cur < len(a.names) {
			fl := a.values[a.names[a.cur]]
			fl.beginEdit()
			a.values[a.names[a.cur]] = fl
			a.editing = true
		}
	case key == "x":
		if a.cur < len(a.names) {
			delete(a.values, a.names[a.cur])
			a.names = slices.Delete(a.names, a.cur, a.cur+1)
			if a.cur >= len(a.names) && a.cur > 0 {
				a.cur--
			}
		}
	}
}

// secretsAdapter edits the [secrets] map with its two spellings.
type secretsAdapter struct {
	names    []string
	entries  map[string]secretsEntry
	baseline map[string]profile.Secret
	cur      int
}

type secretsEntry struct {
	env         field
	description field
	allowEmpty  bool
}

func newSecretsAdapter(cfg *profile.ModuleConfig) *secretsAdapter {
	a := &secretsAdapter{
		entries:  map[string]secretsEntry{},
		baseline: map[string]profile.Secret{},
	}
	for k, s := range cfg.Secrets {
		a.entries[k] = secretsEntry{
			env:         newField("env", "env", s.Env),
			description: newField("description", "description", s.Description),
			allowEmpty:  s.AllowEmpty,
		}
		a.baseline[k] = s
		a.names = append(a.names, k)
	}
	a.names = slices.Sorted(maps.Keys(a.entries))
	return a
}

func (a *secretsAdapter) ID() string     { return "secrets" }
func (a *secretsAdapter) Title() string  { return "secrets" }
func (a *secretsAdapter) Family() string { return service.FamilySecrets }

func (a *secretsAdapter) draft() map[string]profile.Secret {
	out := make(map[string]profile.Secret, len(a.entries))
	for k, e := range a.entries {
		out[k] = profile.Secret{Env: e.env.String(), Description: e.description.String(), AllowEmpty: e.allowEmpty}
	}
	return out
}

func (a *secretsAdapter) Dirty() bool {
	return !reflect.DeepEqual(a.draft(), a.baseline)
}

func (a *secretsAdapter) Block() string {
	return profile.EncodeSecretsSection(a.draft())
}

func (a *secretsAdapter) Errors() []string {
	var out []string
	for _, k := range a.names {
		e := a.entries[k]
		if strings.TrimSpace(e.env.String()) == "" {
			out = append(out, k+" needs an env var")
		}
	}
	return out
}

func (a *secretsAdapter) HandleKey(key string) {
	switch {
	case key == "j" || key == "down":
		if a.cur < len(a.names)-1 {
			a.cur++
		}
	case key == "k" || key == "up":
		if a.cur > 0 {
			a.cur--
		}
	case key == "enter":
		if a.cur < len(a.names) {
			e := a.entries[a.names[a.cur]]
			e.env.beginEdit()
			a.entries[a.names[a.cur]] = e
		}
	}
}

func (a *secretsAdapter) View(p Palette) string {
	if len(a.names) == 0 {
		return p.Meta("no secrets — edit module.toml by hand to add the first one") + p.Meta("  (n arrives with T-tui-writes)")
	}
	var b strings.Builder
	for i, k := range a.names {
		e := a.entries[k]
		cur := "  "
		if i == a.cur {
			cur = "> "
		}
		line := cur + k + " → " + e.env.String()
		if e.description.String() != "" {
			line += " · " + e.description.String()
		}
		if e.allowEmpty {
			line += " (allow_empty)"
		}
		if i == a.cur {
			line = p.Mark(line)
		}
		b.WriteString(line + "\n")
	}
	b.WriteString(p.Meta("smb.users becomes OS accounts at apply; secrets never hold values — env indirection only"))
	return b.String()
}
