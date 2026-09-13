package editor

import (
	"fmt"
	"slices"
	"strings"

	"github.com/thedataflows/dotdrift/internal/profile"
	"github.com/thedataflows/dotdrift/internal/service"
)

// The dotfiles editor (0065-D9's custom model): an entry list with kind
// badges, a whole-file ↔ edit variant switch, contract-18 field
// exclusivity and the <file-path>/<edit-id> key split enforced live.

type dotfilesAdapter struct {
	targets   []string // sorted entry keys
	draft     map[string]profile.Dotfile
	baseline  map[string]profile.Dotfile
	cur       int
	inForm    bool
	formFocus int
	fields    [6]field // source, mode, line, block, comment, template
	formKey   string
	prompting bool
	prompt    field
}

var dotfileFieldSpecs = [6]struct {
	key   string
	label string
	pick  func(d profile.Dotfile) string
	apply func(d *profile.Dotfile, v string)
}{
	{"source", "source", func(d profile.Dotfile) string { return d.Source }, func(d *profile.Dotfile, v string) { d.Source = v }},
	{"mode", "mode", func(d profile.Dotfile) string { return d.Mode }, func(d *profile.Dotfile, v string) { d.Mode = v }},
	{"line", "line", func(d profile.Dotfile) string { return d.Line }, func(d *profile.Dotfile, v string) { d.Line = v }},
	{"block", "block", func(d profile.Dotfile) string { return d.Block }, func(d *profile.Dotfile, v string) { d.Block = v }},
	{"comment", "comment", func(d profile.Dotfile) string { return d.Comment }, func(d *profile.Dotfile, v string) { d.Comment = v }},
	{"template", "template", func(d profile.Dotfile) string { return d.Template }, func(d *profile.Dotfile, v string) { d.Template = v }},
}

func newDotfilesAdapter(cfg *profile.ModuleConfig) *dotfilesAdapter {
	a := &dotfilesAdapter{
		draft:    map[string]profile.Dotfile{},
		baseline: map[string]profile.Dotfile{},
	}
	for k, d := range cfg.Dotfiles {
		a.draft[k] = d
		a.baseline[k] = d
		a.targets = append(a.targets, k)
	}
	slices.Sort(a.targets)
	return a
}

func (a *dotfilesAdapter) ID() string     { return "dotfiles" }
func (a *dotfilesAdapter) Title() string  { return "dotfiles" }
func (a *dotfilesAdapter) Family() string { return service.FamilyDotfiles }

func (a *dotfilesAdapter) Dirty() bool {
	if len(a.draft) != len(a.baseline) {
		return true
	}
	for k, d := range a.draft {
		if b, ok := a.baseline[k]; !ok || b != d {
			return true
		}
	}
	return false
}

func (a *dotfilesAdapter) Block() string {
	return profile.EncodeDotfilesSection(a.draft)
}

// kindBadge names an entry's variant: whole-file or one of the edit kinds.
func kindBadge(d profile.Dotfile) string {
	switch {
	case d.Line != "":
		return "edit·line"
	case d.Block != "":
		return "edit·block"
	case d.Template != "":
		return "edit·tpl"
	case d.Mode == "edit":
		return "edit·src"
	default:
		return "whole"
	}
}

// Errors enforces contract 18 live, reusing resolve's rules: edit entries
// must not set mode; mode = "edit" is exclusive with line/block/template
// and requires a source; an edit variant is keyed "<file-path>/<edit-id>".
func (a *dotfilesAdapter) Errors() []string {
	var out []string
	for _, k := range a.sortedKeys() {
		d := a.draft[k]
		edit := d.Line != "" || d.Block != "" || d.Template != ""
		if edit && d.Mode != "" && d.Mode != "edit" {
			out = append(out, fmt.Sprintf("%q: edit entry must not set mode", k))
		}
		if d.Line != "" && d.Block != "" {
			out = append(out, fmt.Sprintf("%q: line and block are exclusive", k))
		}
		if d.Mode == "edit" {
			if d.Line != "" || d.Block != "" || d.Template != "" {
				out = append(out, fmt.Sprintf("%q: mode = \"edit\" is exclusive with line/block/template", k))
			}
			if d.Source == "" {
				out = append(out, fmt.Sprintf("%q: mode = \"edit\" requires source", k))
			}
		}
		if edit && !strings.Contains(k, "/") {
			out = append(out, fmt.Sprintf("%q: edit entries are keyed <file-path>/<edit-id>", k))
		}
	}
	return out
}

// variant cycles the focused entry's shape: whole → line → block →
// template → whole, dropping each variant's forbidden fields as it goes
// (contract 18 by construction).
func (a *dotfilesAdapter) variant() {
	if a.cur >= len(a.targets) {
		return
	}
	k := a.targets[a.cur]
	d := a.draft[k]
	switch {
	case d.Line == "" && d.Block == "" && d.Template == "" && d.Mode != "edit":
		d = profile.Dotfile{Source: d.Source, Line: "set -x"} // whole → edit·line
	case d.Line != "":
		d = profile.Dotfile{Source: d.Source, Block: "content"} // line → block
	case d.Block != "":
		d = profile.Dotfile{Source: d.Source, Template: "tera"} // block → template
	default:
		d = profile.Dotfile{Source: d.Source, Mode: d.Mode} // edit → whole
	}
	a.draft[k] = d
	if a.inForm && a.formKey == k {
		a.loadForm(k)
	}
}

func (a *dotfilesAdapter) HandleKey(key string) {
	if a.prompting {
		if a.prompt.handleEditKey(key) && !a.prompt.edit {
			if target := strings.TrimSpace(a.prompt.String()); target != "" {
				if _, ok := a.draft[target]; !ok {
					a.targets = append(a.targets, target)
					slices.Sort(a.targets)
				}
				if _, ok := a.draft[target]; !ok {
					a.draft[target] = profile.Dotfile{}
				}
				a.cur = slices.Index(a.targets, target)
				a.openForm(target)
			}
			a.prompting = false
		}
		return
	}

	if a.inForm {
		switch key {
		case "j", "down":
			a.formFocus = (a.formFocus + 1) % len(a.fields)
		case "k", "up":
			a.formFocus = (a.formFocus + len(a.fields) - 1) % len(a.fields)
		case "enter":
			a.fields[a.formFocus].beginEdit()
		case "v":
			a.variant()
		case "esc":
			a.inForm = false
		default:
			if a.fields[a.formFocus].edit {
				a.fields[a.formFocus].handleEditKey(key)
				a.commitForm()
			}
		}
		return
	}

	switch key {
	case "j", "down":
		if a.cur < len(a.targets)-1 {
			a.cur++
		}
	case "k", "up":
		if a.cur > 0 {
			a.cur--
		}
	case "n":
		a.prompting = true
		a.prompt = newField("target", "target", "")
		a.prompt.beginEdit()
	case "x":
		if a.cur < len(a.targets) {
			delete(a.draft, a.targets[a.cur])
			a.targets = slices.Delete(a.targets, a.cur, a.cur+1)
			if a.cur >= len(a.targets) && a.cur > 0 {
				a.cur--
			}
		}
	case "enter":
		if a.cur < len(a.targets) {
			a.openForm(a.targets[a.cur])
		}
	case "v":
		a.variant()
	}
}

// sortedKeys lists every draft entry's key in display order — the draft
// map is the truth, the targets list is its view.
func (a *dotfilesAdapter) sortedKeys() []string {
	keys := make([]string, 0, len(a.draft))
	for k := range a.draft {
		keys = append(keys, k)
	}
	slices.Sort(keys)
	return keys
}

func (a *dotfilesAdapter) openForm(key string) {
	a.inForm = true
	a.formKey = key
	a.formFocus = 0
	a.loadForm(key)
	a.fields[0].edit = true
}

func (a *dotfilesAdapter) loadForm(key string) {
	d := a.draft[key]
	for i, spec := range dotfileFieldSpecs {
		a.fields[i] = newField(spec.key, spec.label, spec.pick(d))
	}
}

// commitForm writes the form fields back into the focused entry.
func (a *dotfilesAdapter) commitForm() {
	d := a.draft[a.formKey]
	for i, spec := range dotfileFieldSpecs {
		spec.apply(&d, a.fields[i].String())
	}
	a.draft[a.formKey] = d
}

func (a *dotfilesAdapter) View(p Palette) string {
	if a.prompting {
		return p.Meta("entry target: ") + renderField(p, &a.prompt) + p.Meta("  enter confirm · esc cancel")
	}
	if a.inForm {
		var b strings.Builder
		b.WriteString(p.Label(kindBadge(a.draft[a.formKey])) + p.Meta(" "+a.formKey) + "\n")
		for i, spec := range dotfileFieldSpecs {
			cur := "  "
			if i == a.formFocus {
				cur = "> "
			}
			b.WriteString(cur + spec.label + " · " + renderField(p, &a.fields[i]) + "\n")
		}
		b.WriteString(p.Meta("enter edit · v variant · esc back"))
		return b.String()
	}
	if len(a.targets) == 0 {
		return p.Meta("no dotfiles — n adds one")
	}
	var b strings.Builder
	for i, k := range a.targets {
		cur := "  "
		if i == a.cur {
			cur = "> "
		}
		d := a.draft[k]
		line := cur + "[" + kindBadge(d) + "] " + k
		if d.Source != "" {
			line += " ← " + d.Source
		}
		if i == a.cur {
			line = p.Mark(line)
		}
		b.WriteString(line + "\n")
	}
	return b.String()
}

// The when editor (0065-D9): a text area over the [when] section body with
// live grammar validation — profile's own positioned errors, shifted one
// line up for the synthesized [when] header.

type whenAdapter struct {
	body     string
	baseline string
	edit     bool
	cursor   int // rune offset within body
}

func newWhenAdapter(cfg *profile.ModuleConfig) *whenAdapter {
	body := strings.TrimPrefix(profile.EncodeWhenSection(cfg.When), "[when]\n")
	return &whenAdapter{body: body, baseline: body}
}

func (a *whenAdapter) ID() string     { return "when" }
func (a *whenAdapter) Title() string  { return "when" }
func (a *whenAdapter) Family() string { return service.FamilyWhen }

func (a *whenAdapter) Dirty() bool { return a.body != a.baseline }

func (a *whenAdapter) Block() string {
	if strings.TrimSpace(a.body) == "" {
		return ""
	}
	return "[when]\n" + a.body
}

// Errors validates the draft body with the real grammar: the body is
// strict-decoded as a module.toml behind a synthesized [when] header, and
// the error's position is shifted back one line to point into the body.
func (a *whenAdapter) Errors() []string {
	if strings.TrimSpace(a.body) == "" {
		return nil
	}
	var cfg profile.ModuleConfig
	if err := profile.DecodeModuleTOML("when-editor", []byte("[when]\n"+a.body), &cfg); err != nil {
		return []string{shiftProbeLine(err.Error())}
	}
	if err := profile.ValidateWhen("module", cfg.When); err != nil {
		return []string{err.Error()}
	}
	return nil
}

// shiftProbeLine rewrites the strict decoder's "when-editor:N[:C]: msg"
// bar into "line N-1: msg" — the body starts one line below the
// synthesized header.
func shiftProbeLine(msg string) string {
	const prefix = "when-editor:"
	rest, ok := strings.CutPrefix(msg, prefix)
	if !ok {
		return msg
	}
	line, tail, _ := strings.Cut(rest, ":")
	col := ""
	if l, t, has := strings.Cut(tail, ":"); has && isDigits(l) {
		col, tail = ":"+l, t
	}
	n := 0
	for _, c := range line {
		if c < '0' || c > '9' {
			return strings.TrimSpace(tail)
		}
		n = n*10 + int(c-'0')
	}
	if n < 2 {
		return strings.TrimSpace(tail)
	}
	_ = col
	return fmt.Sprintf("line %d:%s", n-1, tail)
}

func isDigits(s string) bool {
	for _, c := range s {
		if c < '0' || c > '9' {
			return false
		}
	}
	return s != ""
}

func (a *whenAdapter) HandleKey(key string) {
	if !a.edit {
		if key == "enter" || key == "i" {
			a.edit = true
		}
		return
	}
	switch key {
	case "esc":
		a.edit = false
	case "backspace":
		if a.cursor > 0 {
			r := []rune(a.body)
			a.cursor--
			a.body = string(r[:a.cursor]) + string(r[a.cursor+1:])
		}
	case "left":
		if a.cursor > 0 {
			a.cursor--
		}
	case "right":
		if a.cursor < len([]rune(a.body)) {
			a.cursor++
		}
	default:
		if len(key) == 1 {
			r := []rune(a.body)
			a.body = string(r[:a.cursor]) + key + string(r[a.cursor:])
			a.cursor++
		}
	}
}

func (a *whenAdapter) View(p Palette) string {
	var b strings.Builder
	b.WriteString(p.Label("[when]") + p.Meta(" — the expression as TOML; grammar checked live") + "\n")
	body := a.body
	if a.edit {
		r := []rune(body)
		at := a.cursor
		if at > len(r) {
			at = len(r)
		}
		body = string(r[:at]) + "▌" + string(r[at:])
	}
	b.WriteString(body)
	if body == "" || !strings.HasSuffix(body, "\n") {
		b.WriteString("\n")
	}
	b.WriteString(p.Meta("enter/i edit · esc done"))
	if errs := a.Errors(); len(errs) > 0 {
		b.WriteString("\n" + p.Error(errs[0]))
	}
	return b.String()
}
