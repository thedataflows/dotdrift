// Package editor is the TUI's editor suite (0065-D6/D7 layer 4): one
// schema-driven frame over a file-scoped draft, plus one adapter per
// module.toml section and four custom models for the sections whose schema
// is not form-shaped (dotfiles, when, hooks, systemd.units). The frame owns
// dirty tracking, the validation tiers, save gating, and the dirty-confirm
// chrome; it drives the service config area and holds no presentation
// below the View — pure state machines, tested without a terminal.
package editor

import (
	"errors"
	"strings"

	"github.com/thedataflows/dotdrift/internal/profile"
	"github.com/thedataflows/dotdrift/internal/service"
)

// Config is the frame's narrow service surface (ADR-0008's doorway),
// satisfied implicitly by *service.ConfigArea.
type Config interface {
	ReadModuleLayer(dir string) (*service.ModuleLayerRead, error)
	WriteModuleLayer(req service.SaveRequest) (*service.SaveResult, error)
}

// Palette is the chrome's styling surface — internal/tui's registry-backed
// theme implements it (ADR-0003: one color source, no second palette).
type Palette interface {
	Label(s string) string
	Meta(s string) string
	Error(s string) string
	Mark(s string) string
}

// Adapter is one section editor bound to the frame's shared draft
// (0065-D6): fields, validators reusing profile's own checks, the
// canonical re-encode, and a small key vocabulary. Implementations are
// pure state machines.
type Adapter interface {
	ID() string
	Title() string
	Family() string
	// Dirty deep-compares the draft values against the baseline (0065-D8).
	Dirty() bool
	// Block re-encodes the draft section canonically (the staged block).
	Block() string
	// Errors returns the tier-1 live validation findings.
	Errors() []string
	// HandleKey applies the editor-local key vocabulary.
	HandleKey(key string)
	// View renders the section body.
	View(p Palette) string
}

// The editor-local key vocabulary (the shell's 0062-D5 globals stay
// reserved): [ ] switch section, j/k move, enter edit/commit, esc done or
// back, n new row, x delete row, v variant/spelling switch, space toggle,
// < > reorder, s save, r reload.
const (
	keyPrevSection = "["
	keyNextSection = "]"
	keySave        = "s"
	keyReload      = "r"
)

// Frame is the editor over ONE layer's module.toml (0065-D8): the draft
// unit is the file. Every section adapter binds to the shared draft, so
// multiple dirty sections save in one splice and an editor never
// hash-conflicts with itself.
type Frame struct {
	cfg    Config
	pal    Palette
	dir    string
	draft  *service.ModuleLayerRead
	status string

	sections []Adapter
	cur      int

	readOnly  bool
	schemaErr *service.SchemaError

	confirm    bool // dirty-confirm chrome showing
	popRequest bool // esc asked to leave the editor (clean, or confirmed)

	cross []string // tier-2 findings (cross-checks, run on ticks/switch/save)
}

// NewFrame opens the editor for one layer's module.toml. A broken file
// opens read-only with its schema error (0065-D2/D8); a missing file opens
// as a scaffold draft.
func NewFrame(cfg Config, pal Palette, dir string) *Frame {
	f := &Frame{cfg: cfg, pal: pal, dir: dir}
	f.reload()
	return f
}

// reload (re)reads the file and rebuilds the section adapters from the
// decoded config. Used at open and after a save or an explicit reload.
func (f *Frame) reload() {
	r, err := f.cfg.ReadModuleLayer(f.dir)
	switch {
	case err == nil:
		f.draft = r
		f.readOnly = false
		f.schemaErr = nil
		f.sections = buildAdapters(r)
	case errors.As(err, new(*service.SchemaError)):
		var schema *service.SchemaError
		_ = errors.As(err, &schema)
		f.draft = &service.ModuleLayerRead{Dir: f.dir, Config: &profile.ModuleConfig{}}
		f.readOnly = true
		f.schemaErr = schema
		f.sections = nil
		f.status = "broken file — read-only: " + err.Error()
	default:
		f.draft = nil
		f.readOnly = true
		f.status = "cannot open: " + err.Error()
	}
	f.cur = 0
	f.confirm = false
	f.cross = nil
}

// Dir is the module layer directory the frame edits.
func (f *Frame) Dir() string { return f.dir }

// Path is the exact file the editor chrome names (0065-D2).
func (f *Frame) Path() string {
	if f.draft == nil {
		return ""
	}
	return f.draft.Path
}

// ReadOnly reports a broken (or unreadable) file: nothing is editable.
func (f *Frame) ReadOnly() bool { return f.readOnly }

// Confirming reports whether the dirty-confirm chrome is showing.
func (f *Frame) Confirming() bool { return f.confirm }

// Status is the frame's status line.
func (f *Frame) Status() string { return f.status }

// PopRequested reports that esc asked to leave the editor — the shell owns
// the stack and pops on it. Only a clean frame (or a confirmed discard or
// successful save) sets it.
func (f *Frame) PopRequested() bool { return f.popRequest }

// Sections lists the adapter ids in tab order.
func (f *Frame) Sections() []string {
	ids := make([]string, 0, len(f.sections))
	for _, s := range f.sections {
		ids = append(ids, s.ID())
	}
	return ids
}

// Current returns the active section's id.
func (f *Frame) Current() string {
	if f.cur < len(f.sections) {
		return f.sections[f.cur].ID()
	}
	return ""
}

// Dirty reports whether any section of the shared draft carries unsaved
// state.
func (f *Frame) Dirty() bool {
	for _, s := range f.sections {
		if s.Dirty() {
			return true
		}
	}
	return false
}

// Staged returns the family → block map the next save would splice: one
// entry per dirty section, all of them belonging to this one file.
func (f *Frame) Staged() map[string]string {
	staged := map[string]string{}
	for _, s := range f.sections {
		if s.Dirty() {
			staged[s.Family()] = s.Block()
		}
	}
	return staged
}

// Errors collects the tier-1 live findings of every section.
func (f *Frame) Errors() []string {
	var out []string
	for _, s := range f.sections {
		for _, e := range s.Errors() {
			out = append(out, s.Title()+": "+e)
		}
	}
	return out
}

// CrossChecks returns the tier-2 findings — the in-module cross-checks
// (scope vs sections present), refreshed debounced (on Tick, section
// switch, and save).
func (f *Frame) CrossChecks() []string { return f.cross }

// crossCheck runs the in-module cross-checks against the draft values
// (0065-D8): the same scope rules resolve enforces, said early. The
// sections' own values are read from the draft config snapshot.
func (f *Frame) crossCheck() {
	f.cross = nil
	if f.readOnly || f.draft == nil {
		return
	}
	scope := f.draft.Config.ScopeOrDefault()
	hasMounts := sectionDirtyOrSet(f, service.FamilyMounts) || len(f.draft.Config.Mounts) > 0
	hasSmb := sectionDirtyOrSet(f, service.FamilySmb) || len(f.draft.Config.Smb.Shares) > 0 || f.draft.Config.Smb.Group != ""
	hasSystemd := sectionDirtyOrSet(f, service.FamilySystemd) || len(f.draft.Config.Systemd.Units) > 0
	if (hasMounts || hasSmb) && scope != "system" {
		f.cross = append(f.cross, "mounts/smb require scope = \"system\" (got \""+scope+"\")")
	}
	if hasSystemd && scope != "user" {
		f.cross = append(f.cross, "systemd user units require scope = \"user\" (got \""+scope+"\")")
	}
}

// sectionDirtyOrSet reports whether the family's adapter exists, is
// dirty, and its block is non-empty (a section edited INTO existence —
// e.g. mounts added to a module that had none — counts as set).
func sectionDirtyOrSet(f *Frame, family string) bool {
	for _, s := range f.sections {
		if s.Family() == family {
			return s.Dirty() && s.Block() != ""
		}
	}
	return false
}

// Tick is the shell's heartbeat: the debounced point where the tier-2
// cross-checks recompute (they also run on section switches and saves).
func (f *Frame) Tick() {
	f.crossCheck()
}

// HandleKey applies the frame's key vocabulary. Keys the shell reserves
// never reach here.
func (f *Frame) HandleKey(key string) {
	if f.readOnly {
		switch key {
		case keyReload:
			f.reload()
		}
		return
	}

	if f.confirm {
		switch key {
		case keySave: // save, then leave
			f.confirm = false
			if err := f.Save(); err == nil {
				f.popRequest = true
			}
		case "d": // discard, then leave
			f.reload()
			f.popRequest = true
		case "esc", "n": // cancel the confirm
			f.confirm = false
			f.status = ""
		}
		return
	}

	switch key {
	case keyNextSection:
		f.switchSection(1)
		return
	case keyPrevSection:
		f.switchSection(-1)
		return
	case keySave:
		_ = f.Save()
		return
	case keyReload:
		f.reload()
		return
	case "esc":
		if f.Dirty() {
			f.confirm = true // the dirty-confirm guard (0065-D8)
			return
		}
		f.popRequest = true
		return
	}

	if s := f.active(); s != nil {
		s.HandleKey(key)
	}
}

func (f *Frame) active() Adapter {
	if f.cur < len(f.sections) {
		return f.sections[f.cur]
	}
	return nil
}

func (f *Frame) switchSection(delta int) {
	if len(f.sections) == 0 {
		return
	}
	f.cur = ((f.cur+delta)%len(f.sections) + len(f.sections)) % len(f.sections)
	f.crossCheck()
}

// Save runs the authoritative path (0065-D8): tier-1 live findings gate,
// tier-2 cross-checks gate, then the service save pipeline (tier 3) —
// encode → splice → strict-decode → resolve checks → disk-hash → atomic
// write. Success rebases the draft onto the saved file.
func (f *Frame) Save() error {
	if f.readOnly || f.draft == nil {
		return errors.New("editor is read-only")
	}
	if errs := f.Errors(); len(errs) > 0 {
		f.status = "fix " + itoa(len(errs)) + " field error(s) first: " + errs[0]
		return errors.New(f.status)
	}
	f.crossCheck() // tier 2, authoritative at save
	if len(f.cross) > 0 {
		f.status = "cross-check: " + f.cross[0]
		return errors.New(f.status)
	}
	staged := f.Staged()
	if len(staged) == 0 {
		f.status = "nothing to save"
		return nil
	}
	res, err := f.cfg.WriteModuleLayer(service.SaveRequest{
		Dir:          f.dir,
		BaseHash:     f.draft.Hash,
		Replacements: staged,
	})
	if err != nil {
		var conflict *service.DiskHashConflictError
		switch {
		case errors.As(err, &conflict):
			f.status = "changed on disk — r reloads"
		default:
			f.status = firstLine(err.Error())
		}
		return err
	}
	_ = res
	f.reload()
	f.status = "saved"
	return nil
}

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	var b [20]byte
	i := len(b)
	for n > 0 {
		i--
		b[i] = byte('0' + n%10)
		n /= 10
	}
	return string(b[i:])
}

func firstLine(s string) string {
	if i := strings.IndexByte(s, '\n'); i >= 0 {
		return s[:i]
	}
	return s
}

// View renders the editor chrome: a title naming the exact file (0065-D2),
// the section tabs, the active section's body, and the status/confirm
// line.
func (f *Frame) View() string {
	var b strings.Builder
	p := f.pal
	if p == nil {
		p = plainPalette{}
	}
	if f.readOnly {
		b.WriteString(p.Label("EDIT (read-only)") + p.Meta(" — "+f.Path()) + "\n")
		b.WriteString(p.Error(f.status))
		return b.String()
	}
	b.WriteString(p.Label("EDIT") + p.Meta(" — "+f.Path()) + "\n")

	for i, s := range f.sections {
		if i > 0 {
			b.WriteString(" ")
		}
		id := s.ID()
		switch {
		case i == f.cur:
			b.WriteString(p.Label("[" + id + "]"))
		case s.Dirty():
			b.WriteString(p.Mark("{" + id + "}"))
		default:
			b.WriteString(p.Meta(id))
		}
	}
	b.WriteString("\n\n")
	if s := f.active(); s != nil {
		b.WriteString(s.View(p) + "\n")
	}

	if f.confirm {
		b.WriteString(p.Mark("● unsaved changes — (s)ave, (d)iscard, esc cancel") + "\n")
	}
	if f.Dirty() {
		b.WriteString(p.Mark("● unsaved") + p.Meta("  s save · [ ] section · esc back") + "\n")
	}
	if status := f.status; status != "" {
		b.WriteString(p.Error(status) + "\n")
	}
	return b.String()
}

// plainPalette is the nil-Palette fallback: raw text, no styling.
type plainPalette struct{}

func (plainPalette) Label(s string) string { return s }
func (plainPalette) Meta(s string) string  { return s }
func (plainPalette) Error(s string) string { return s }
func (plainPalette) Mark(s string) string  { return s }
