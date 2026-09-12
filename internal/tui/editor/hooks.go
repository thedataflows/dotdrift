package editor

import (
	"slices"
	"strings"

	"github.com/thedataflows/dotdrift/internal/profile"
	"github.com/thedataflows/dotdrift/internal/service"
)

// The hooks editor (0065-D9): ordered rows for the pre/post arrays, the
// plain-string vs inline-table spelling per row, the optional checkbox.
// ONE layer's array only — append-across-layers is resolution, never
// editing.

type hooksAdapter struct {
	pre, post []hookRow
	baseline  [2][]hookRow
	cur       int // index into pre++post
	prompting bool
	prompt    field
}

type hookRow struct {
	cmd        field
	optional   bool
	structured bool
}

func newHookRow(cmd string) hookRow {
	return hookRow{cmd: newField("command", "command", cmd)}
}

func toHookRows(rows []hookRow) []profile.HookRow {
	out := make([]profile.HookRow, 0, len(rows))
	for _, r := range rows {
		out = append(out, profile.HookRow{Command: r.cmd.String(), Optional: r.optional, Structured: r.structured})
	}
	return out
}

func fromHookRows(cmds []profile.HookCommand) []hookRow {
	rows := make([]hookRow, 0, len(cmds))
	for _, c := range profile.HookRows(cmds) {
		r := newHookRow(c.Command)
		r.optional = c.Optional
		r.structured = c.Structured
		rows = append(rows, r)
	}
	return rows
}

func newHooksAdapter(cfg *profile.ModuleConfig) *hooksAdapter {
	a := &hooksAdapter{
		pre:  fromHookRows(cfg.Hooks.Pre),
		post: fromHookRows(cfg.Hooks.Post),
	}
	a.baseline = [2][]hookRow{cloneHookRows(a.pre), cloneHookRows(a.post)}
	return a
}

func cloneHookRows(rows []hookRow) []hookRow {
	out := make([]hookRow, len(rows))
	for i, r := range rows {
		out[i] = hookRow{cmd: newField("command", "command", r.cmd.baseline), optional: r.optional, structured: r.structured}
	}
	return out
}

func (a *hooksAdapter) ID() string     { return "hooks" }
func (a *hooksAdapter) Title() string  { return "hooks" }
func (a *hooksAdapter) Family() string { return service.FamilyHooks }

func hookRowsEqual(x, y []hookRow) bool {
	if len(x) != len(y) {
		return false
	}
	for i := range x {
		if x[i].cmd.String() != y[i].cmd.String() || x[i].optional != y[i].optional || x[i].structured != y[i].structured {
			return false
		}
	}
	return true
}

func (a *hooksAdapter) Dirty() bool {
	return !hookRowsEqual(a.pre, a.baseline[0]) || !hookRowsEqual(a.post, a.baseline[1])
}

func (a *hooksAdapter) Block() string {
	return profile.EncodeHooksSection(toHookRows(a.pre), toHookRows(a.post))
}

// Errors: a hook entry needs a command (the strict decoder's own rule,
// applied live per row).
func (a *hooksAdapter) Errors() []string {
	var out []string
	for _, list := range [][]hookRow{a.pre, a.post} {
		for i, r := range list {
			if strings.TrimSpace(r.cmd.String()) == "" {
				out = append(out, "hook row needs a command")
			}
			_ = i
		}
	}
	return out
}

// rowAt resolves the cursor to its list and index. With no rows at all,
// new rows land in pre (the natural first list).
func (a *hooksAdapter) rowAt() (*[]hookRow, int) {
	if a.cur < len(a.pre) || len(a.pre)+len(a.post) == 0 {
		return &a.pre, a.cur
	}
	return &a.post, a.cur - len(a.pre)
}

func (a *hooksAdapter) HandleKey(key string) {
	if a.prompting {
		if a.prompt.handleEditKey(key) && !a.prompt.edit {
			if cmd := strings.TrimSpace(a.prompt.String()); cmd != "" {
				list, _ := a.rowAt()
				*list = append(*list, newHookRow(cmd))
				if list == &a.pre {
					a.cur = len(a.pre) - 1
				} else {
					a.cur = len(a.pre) + len(a.post) - 1
				}
			}
			a.prompting = false
		}
		return
	}

	// Command editing wins when a row's field is open.
	if a.cur < len(a.pre)+len(a.post) {
		list, i := a.rowAt()
		r := &(*list)[i]
		if r.cmd.edit {
			r.cmd.handleEditKey(key)
			return
		}
	}

	switch {
	case key == "j" || key == "down":
		if a.cur < len(a.pre)+len(a.post)-1 {
			a.cur++
		}
	case key == "k" || key == "up":
		if a.cur > 0 {
			a.cur--
		}
	case key == "enter":
		list, i := a.rowAt()
		(*list)[i].cmd.beginEdit()
	case key == "n":
		a.prompting = true
		a.prompt = newField("command", "command", "")
		a.prompt.beginEdit()
	case key == "x":
		list, i := a.rowAt()
		*list = slices.Delete(*list, i, i+1)
		if a.cur >= len(a.pre)+len(a.post) && a.cur > 0 {
			a.cur--
		}
	case key == " ":
		list, i := a.rowAt()
		(*list)[i].optional = !(*list)[i].optional
	case key == "v":
		list, i := a.rowAt()
		(*list)[i].structured = !(*list)[i].structured
	case key == "<":
		list, i := a.rowAt()
		if i > 0 {
			(*list)[i-1], (*list)[i] = (*list)[i], (*list)[i-1]
			a.cur--
		}
	case key == ">":
		list, i := a.rowAt()
		if i < len(*list)-1 {
			(*list)[i+1], (*list)[i] = (*list)[i], (*list)[i+1]
			a.cur++
		}
	case key == "esc":
		list, i := a.rowAt()
		(*list)[i].cmd.endEdit()
	}
}

func (a *hooksAdapter) View(p Palette) string {
	if a.prompting {
		return p.Meta("hook command: ") + renderField(p, &a.prompt) + p.Meta("  enter confirm · esc cancel")
	}
	var b strings.Builder
	row := func(label string, list []hookRow, offset int) {
		for i, r := range list {
			cur := "  "
			if offset+i == a.cur {
				cur = "> "
			}
			line := cur + label + " " + r.cmd.String()
			if r.optional {
				line += " (optional)"
			}
			if r.structured {
				line += p.Meta(" {table}")
			}
			if offset+i == a.cur {
				line = p.Mark(line)
			}
			b.WriteString(line + "\n")
		}
	}
	row("pre ", a.pre, 0)
	row("post", a.post, len(a.pre))
	if len(a.pre)+len(a.post) == 0 {
		b.WriteString(p.Meta("no hooks — n adds one"))
	} else {
		b.WriteString(p.Meta("space optional · v spelling · < > order · n add · x delete"))
	}
	return b.String()
}

// The systemd.units editor (0065-D9): named key-value rows with a kind
// badge derived from the unit suffix; directives are free-form passthrough
// (values keep their TOML types; dotdrift validates structure only).

type systemdAdapter struct {
	units       []string // sorted unit names
	draft       map[string]profile.SystemdUnit
	baseline    map[string]profile.SystemdUnit
	cur         int
	inUnit      bool
	curDir      int
	promptUnit  bool
	promptU     field
	promptDir   bool
	promptDKey  field
	promptDVal  field
	promptStep  int // 0 = key, 1 = value
	editVal     field
	editing     bool
	raw         map[string]map[string]string // unit → directive → raw TOML text
	baselineRaw map[string]map[string]string
}

func newSystemdAdapter(cfg *profile.ModuleConfig) *systemdAdapter {
	a := &systemdAdapter{
		draft:       map[string]profile.SystemdUnit{},
		baseline:    map[string]profile.SystemdUnit{},
		raw:         map[string]map[string]string{},
		baselineRaw: map[string]map[string]string{},
	}
	for name, unit := range cfg.Systemd.Units {
		a.draft[name] = unit
		a.baseline[name] = unit
		a.units = append(a.units, name)
		a.raw[name] = map[string]string{}
		a.baselineRaw[name] = map[string]string{}
		for k, v := range unit {
			a.raw[name][k] = rawValue(v)
			a.baselineRaw[name][k] = rawValue(v)
		}
	}
	slices.Sort(a.units)
	return a
}

// kindBadge derives a unit's kind from its suffix (resolve derives the
// emitted unit kind the same way).
func unitKindBadge(name string) string {
	switch {
	case strings.HasSuffix(name, ".service"):
		return "service"
	case strings.HasSuffix(name, ".timer"):
		return "timer"
	default:
		return "unit"
	}
}

func (a *systemdAdapter) ID() string     { return "systemd" }
func (a *systemdAdapter) Title() string  { return "systemd.units" }
func (a *systemdAdapter) Family() string { return service.FamilySystemd }

func (a *systemdAdapter) parseDraft() error {
	for unit, dirs := range a.raw {
		parsed := profile.SystemdUnit{}
		for k, rawText := range dirs {
			v, err := tomlDecodeAny(rawText)
			if err != nil {
				return err
			}
			parsed[k] = v
		}
		a.draft[unit] = parsed
	}
	return nil
}

func (a *systemdAdapter) Dirty() bool {
	if len(a.draft) != len(a.baseline) {
		return true
	}
	for u, dirs := range a.raw {
		base, ok := a.baselineRaw[u]
		if !ok || len(dirs) != len(base) {
			return true
		}
		for k, v := range dirs {
			if base[k] != v {
				return true
			}
		}
	}
	return false
}

func (a *systemdAdapter) Block() string {
	if err := a.parseDraft(); err != nil {
		return profile.EncodeSystemdSection(a.draft) // last good parse
	}
	return profile.EncodeSystemdSection(a.draft)
}

// Errors: directive values must parse as TOML values (passthrough values
// keep their types); unit names need a kind suffix.
func (a *systemdAdapter) Errors() []string {
	var out []string
	for _, u := range a.units {
		for k, rawText := range a.raw[u] {
			if _, err := tomlDecodeAny(rawText); err != nil {
				out = append(out, u+" · "+k+": "+err.Error())
			}
		}
	}
	return out
}

func (a *systemdAdapter) HandleKey(key string) {
	// Prompt flows: new unit name, then directive key/value.
	if a.promptUnit {
		if a.promptU.handleEditKey(key) && !a.promptU.edit {
			if name := strings.TrimSpace(a.promptU.String()); name != "" {
				if _, ok := a.draft[name]; !ok {
					a.draft[name] = profile.SystemdUnit{}
					a.raw[name] = map[string]string{}
					a.units = append(a.units, name)
					slices.Sort(a.units)
				}
				a.cur = slices.Index(a.units, name)
				a.inUnit = true
			}
			a.promptUnit = false
		}
		return
	}
	if a.promptDir {
		switch a.promptStep {
		case 0:
			if a.promptDKey.handleEditKey(key) && !a.promptDKey.edit {
				if strings.TrimSpace(a.promptDKey.String()) == "" {
					a.promptDir = false
					return
				}
				a.promptStep = 1
				a.promptDVal.beginEdit()
			}
			return
		case 1:
			if a.promptDVal.handleEditKey(key) && !a.promptDVal.edit {
				unit := a.units[a.cur]
				a.raw[unit][a.promptDKey.String()] = a.promptDVal.String()
				a.promptDir = false
				a.promptStep = 0
			}
			return
		}
	}
	if a.editing {
		a.editVal.handleEditKey(key)
		if !a.editVal.edit {
			a.editing = false
			unit := a.units[a.cur]
			dirs := a.directiveNames(unit)
			if a.curDir < len(dirs) {
				a.raw[unit][dirs[a.curDir]] = a.editVal.String()
			}
		}
		return
	}

	if a.inUnit {
		unit := a.units[a.cur]
		dirs := a.directiveNames(unit)
		switch {
		case key == "j" || key == "down":
			if a.curDir < len(dirs)-1 {
				a.curDir++
			}
		case key == "k" || key == "up":
			if a.curDir > 0 {
				a.curDir--
			}
		case key == "enter":
			if a.curDir < len(dirs) {
				a.editVal = newField("value", "value", a.raw[unit][dirs[a.curDir]])
				a.editVal.beginEdit()
				a.editing = true
			}
		case key == "n":
			a.promptDir = true
			a.promptStep = 0
			a.promptDKey = newField("key", "directive", "")
			a.promptDVal = newField("value", "value", "")
			a.promptDKey.beginEdit()
		case key == "x":
			if a.curDir < len(dirs) {
				delete(a.raw[unit], dirs[a.curDir])
				if a.curDir >= len(a.raw[unit]) && a.curDir > 0 {
					a.curDir--
				}
			}
		case key == "esc":
			a.inUnit = false
			a.curDir = 0
		}
		return
	}

	switch {
	case key == "j" || key == "down":
		if a.cur < len(a.units)-1 {
			a.cur++
		}
	case key == "k" || key == "up":
		if a.cur > 0 {
			a.cur--
		}
	case key == "enter":
		if a.cur < len(a.units) {
			a.inUnit = true
		}
	case key == "n":
		a.promptUnit = true
		a.promptU = newField("unit", "unit", "")
		a.promptU.beginEdit()
	case key == "x":
		if a.cur < len(a.units) {
			delete(a.draft, a.units[a.cur])
			delete(a.raw, a.units[a.cur])
			a.units = slices.Delete(a.units, a.cur, a.cur+1)
			if a.cur >= len(a.units) && a.cur > 0 {
				a.cur--
			}
		}
	}
}

func (a *systemdAdapter) directiveNames(unit string) []string {
	keys := make([]string, 0, len(a.raw[unit]))
	for k := range a.raw[unit] {
		keys = append(keys, k)
	}
	slices.Sort(keys)
	return keys
}

func (a *systemdAdapter) View(p Palette) string {
	if a.promptUnit {
		return p.Meta("unit name: ") + renderField(p, &a.promptU) + p.Meta("  enter confirm · esc cancel")
	}
	if a.promptDir {
		if a.promptStep == 0 {
			return p.Meta("directive: ") + renderField(p, &a.promptDKey)
		}
		return p.Meta(a.promptDKey.String()+" = ") + renderField(p, &a.promptDVal) + p.Meta("  raw TOML value")
	}
	if len(a.units) == 0 {
		return p.Meta("no units — n adds one")
	}
	var b strings.Builder
	if !a.inUnit {
		for i, u := range a.units {
			cur := "  "
			if i == a.cur {
				cur = "> "
			}
			line := cur + "[" + unitKindBadge(u) + "] " + u
			if i == a.cur {
				line = p.Mark(line)
			}
			b.WriteString(line + "\n")
		}
		return b.String()
	}
	unit := a.units[a.cur]
	b.WriteString(p.Label(unitKindBadge(unit)) + p.Meta(" "+unit) + "\n")
	dirs := a.directiveNames(unit)
	for i, k := range dirs {
		cur := "  "
		if i == a.curDir {
			cur = "> "
		}
		line := cur + k + " = " + a.raw[unit][k]
		if i == a.curDir {
			line = p.Mark(line)
		}
		b.WriteString(line + "\n")
	}
	if a.editing {
		b.WriteString(p.Mark("▌"+a.editVal.String()) + "\n")
	}
	b.WriteString(p.Meta("directives pass through untouched — n add · enter edit · esc back"))
	return b.String()
}
