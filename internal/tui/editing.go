package tui

// T-tui-editing: per-field inline editing over the 0065 file-scoped
// draft ledger. A draft forks from a layer read (or, for a broken file,
// from the raw text plus its disk hash); every committed field edit
// re-encodes its family through the profile encoders and splices it into
// the draft's working raw — the tomlsplice round-trip proof (strict
// decode) runs on every commit, so a draft can never hold unparseable
// text. Semantic tier-1 errors stage visibly and block the save; the
// save itself is the untouched 0065 pipeline (family replacements, or
// the whole-file raw candidate for a repair). Structural families beyond
// links/writes (systemd units, secrets, mounts, smb) stay read-only rows
// here; their 0065 custom editors remain reachable in the M14 shell
// until T-tui-cleanup.

import (
	"errors"
	"fmt"
	"maps"
	"slices"
	"strconv"
	"strings"

	tea "charm.land/bubbletea/v2"
	"github.com/BurntSushi/toml"

	"github.com/thedataflows/dotdrift/internal/profile"
	"github.com/thedataflows/dotdrift/internal/service"
	"github.com/thedataflows/dotdrift/internal/tomlsplice"
)

// wsDraft is one file's working state (the 0065 ledger entry).
type wsDraft struct {
	cfg      *profile.ModuleConfig // working copy; nil until a raw-mode repair parses
	raw      string                // working raw text
	baseHash string                // disk hash the draft forked from
	rawMode  bool                  // forked from a broken file: save sends the raw candidate
	touched  map[string]bool       // families with committed edits
	edited   map[string]bool       // row keys with committed edits (dirty markers)
	changes  int                   // committed edits/adds/removes (the discard confirm's count)
	errs     map[string]string     // row key → staged tier-1 error (blocks save)
}

// wsEdit is the active field/line input.
type wsEdit struct {
	row     int    // rows index; for add, the section's last row
	input   []rune // the input buffer
	cur     int    // cursor position in the buffer
	add     bool   // a synthetic new-row input
	section string // the section an add commits into
	addPath string // structural adds: the entry the add lands in ("" = the section itself)
	err     string // live tier-1 error, rendered at the field
}

// inputString returns the buffer as a string.
func (e *wsEdit) inputString() string { return string(e.input) }

// rowKey identifies one editable row for dirty markers and staged errors.
func rowKey(family, key string) string { return family + ":" + key }

// pathSep joins a structural row's machine path segments (0074). The
// separator never appears in a rendered row, so paths round-trip through
// the key no matter what punctuation a TOML key carries.
const pathSep = "\x1f"

// pathKey joins a structural row's machine path segments.
func pathKey(parts ...string) string { return strings.Join(parts, pathSep) }

// splitPath splits a structural row key back into its path segments.
func splitPath(key string) []string { return strings.Split(key, pathSep) }

// lastPathSegment returns a structural row key's field segment (the
// validateField switch thinks in fields, not whole paths).
func lastPathSegment(key string) string {
	parts := splitPath(key)
	return parts[len(parts)-1]
}

// parseBoolInput is the two-state bool grammar: "true", "false", or ""
// (unset means false).
func parseBoolInput(input string) (bool, error) {
	switch strings.TrimSpace(input) {
	case "true":
		return true, nil
	case "false", "":
		return false, nil
	default:
		return false, errors.New(`bool fields take "true" or "false"`)
	}
}

// parseAvahiInput is the tri-state avahi grammar: "true", "false", or ""
// (empty unsets to nil — the default-on semantics).
func parseAvahiInput(input string) (*bool, error) {
	switch strings.TrimSpace(input) {
	case "true":
		v := true
		return &v, nil
	case "false":
		v := false
		return &v, nil
	case "":
		return nil, nil
	default:
		return nil, errors.New(`avahi takes "true", "false", or empty to unset`)
	}
}

// isStructuralFamily reports whether the family is edited through the
// 0074 structural row grammar (containers + machine paths) rather than
// mutateField's flat scalar/table grammar.
func isStructuralFamily(family string) bool {
	switch family {
	case profile.FamilySystemd, profile.FamilySecrets, profile.FamilyMounts, profile.FamilySmb, profile.FamilyWhen:
		return true
	}
	return false
}

// editableTomlValue renders a directive value as the text a field input
// seeds from: bare strings (the common case edits unquoted), every other
// shape as its canonical TOML text.
func editableTomlValue(v any) string {
	if s, ok := v.(string); ok {
		return s
	}
	return profile.EncodeTomlValue(v)
}

// parseTomlValue parses input as a TOML value — quoted strings, numbers,
// bools, lists, inline tables. Anything that is not valid TOML stands as
// a plain string, so `ExecStart = /usr/bin/demo` needs no quoting.
func parseTomlValue(input string) any {
	s := strings.TrimSpace(input)
	var doc map[string]any
	if err := toml.Unmarshal([]byte("v = "+s+"\n"), &doc); err == nil {
		if v, ok := doc["v"]; ok {
			return v
		}
	}
	return s
}

// forkDraft starts the draft for the active layer, or returns the
// existing one. Structured fork: from the landed read. Raw fork: from
// the broken file's text (the draft ledger's 0065 base is the disk hash).
func (w *workspaceModel) forkDraft() *wsDraft {
	if w.draft != nil {
		return w.draft
	}
	d := &wsDraft{
		touched: map[string]bool{},
		edited:  map[string]bool{},
		errs:    map[string]string{},
	}
	if w.schemaErr != nil {
		d.raw = w.rawErr
		d.baseHash = service.RawHash([]byte(w.rawErr))
		d.rawMode = true
	} else if w.read != nil {
		d.raw = w.read.Raw
		d.baseHash = w.read.Hash
		d.cfg = decodeModule(w.read.Path, w.read.Raw)
	}
	w.draft = d
	return d
}

// decodeModule re-decodes raw text — the cheap exact clone for drafts.
func decodeModule(path, raw string) *profile.ModuleConfig {
	cfg := &profile.ModuleConfig{}
	if err := profile.DecodeModuleTOML(path, []byte(raw), cfg); err != nil {
		return nil
	}
	return cfg
}

// encodeFamily renders one family's splice block from the working config
// through the profile encoders (the single source of encoding truth).
func encodeFamily(family string, cfg *profile.ModuleConfig) string {
	switch family {
	case profile.FamilyKeys:
		return profile.EncodeKeysSection(*cfg)
	case profile.FamilyPackages:
		return profile.EncodePackagesSection(profile.PackageEntries(cfg.Packages.Present), cfg.Packages.Absent)
	case profile.FamilyTools:
		return profile.EncodeToolsSection(cfg.Tools)
	case profile.FamilyWhen:
		return profile.EncodeWhenSection(cfg.When)
	case profile.FamilyHooks:
		return profile.EncodeHooksSection(profile.HookRows(cfg.Hooks.Pre), profile.HookRows(cfg.Hooks.Post))
	case profile.FamilyDotfiles:
		return profile.EncodeDotfilesSection(cfg.Dotfiles)
	case profile.FamilySystemd:
		return profile.EncodeSystemdSection(cfg.Systemd.Units)
	case profile.FamilySecrets:
		return profile.EncodeSecretsSection(cfg.Secrets)
	case profile.FamilyMounts:
		return profile.EncodeMountsSection(cfg.Mounts)
	case profile.FamilySmb:
		return profile.EncodeSmbSection(cfg.Smb)
	default:
		return ""
	}
}

// validateField is tier-1: the field's own grammar, live at the input.
// An empty string means the value stands.
func validateField(family, key, input string) string {
	switch family {
	case profile.FamilyKeys:
		if key == "scope" && input != "" && input != "user" && input != "system" {
			return `scope must be "user" or "system"`
		}
		if key == "app" && strings.TrimSpace(input) == "" {
			return "app must not be empty"
		}
	case profile.FamilyPackages:
		name := strings.TrimPrefix(strings.TrimSpace(input), "-")
		if name == "" {
			return "package name must not be empty"
		}
		if strings.ContainsAny(name, " \t") {
			return "package name must not contain spaces"
		}
	case profile.FamilyHooks:
		if strings.TrimSpace(input) == "" {
			return "hook command must not be empty"
		}
	case profile.FamilySystemd:
		if input == "" {
			return "directive value must not be empty (d removes the directive)"
		}
	case profile.FamilySecrets:
		switch lastPathSegment(key) {
		case "env":
			if strings.TrimSpace(input) == "" {
				return "env must not be empty"
			}
		case "allow_empty":
			if _, err := parseBoolInput(input); err != nil {
				return err.Error()
			}
		}
	case profile.FamilyMounts:
		switch lastPathSegment(key) {
		case "source":
			if input == "" {
				return "source is required"
			}
		case "destination":
			if input == "" {
				return "destination is required"
			}
		case "type":
			if input == "" {
				return "type is required"
			}
		case "state":
			if input != "" && input != "enabled" && input != "disabled" {
				return `state must be "", "enabled", or "disabled"`
			}
		}
	case profile.FamilySmb:
		switch lastPathSegment(key) {
		case "path":
			if input == "" {
				return "path is required"
			}
		case "avahi":
			if _, err := parseAvahiInput(input); err != nil {
				return err.Error()
			}
		case "writable", "public":
			if _, err := parseBoolInput(input); err != nil {
				return err.Error()
			}
		}
	}
	return ""
}

// validateAdd is tier-1 for the add grammar (the `a` input). addPath
// scopes structural adds: "" adds the section's entry itself, otherwise
// it names the entry the add lands beneath. An error refuses the add —
// a rejected add never enters the draft.
func validateAdd(family, addPath, input string) string {
	switch family {
	case profile.FamilyPackages:
		if strings.TrimSpace(input) == "" {
			return "package name must not be empty"
		}
	case profile.FamilyTools:
		name, _, _ := strings.Cut(input, "=")
		if strings.TrimSpace(name) == "" {
			return "tool name must not be empty"
		}
		if !strings.Contains(input, "=") {
			return `add as "name = constraint"`
		}
	case profile.FamilyHooks:
		if strings.TrimSpace(input) == "" {
			return "hook command must not be empty"
		}
	case profile.FamilyDotfiles:
		if len(strings.Fields(input)) != 2 {
			return `add as "target source"`
		}
	case profile.FamilySystemd:
		if addPath == "" {
			return validUnitName(input)
		}
		if !strings.Contains(input, "=") {
			return `add as "Name = value"`
		}
	case profile.FamilySecrets:
		if addPath == "" {
			name, env, _ := strings.Cut(input, "=")
			if strings.TrimSpace(name) == "" || strings.TrimSpace(env) == "" {
				return `add as "name = ENV"`
			}
			return ""
		}
		if !okFieldValue(input, "env", "description", "allow_empty") {
			return `add as "field = value"`
		}
	case profile.FamilyMounts:
		if addPath == "" {
			return validEntryName(input, "mount")
		}
		if !okFieldValue(input, "source", "destination", "type", "options", "startat", "state") {
			return `add as "field = value"`
		}
	case profile.FamilySmb:
		if addPath == "" {
			// the section's root scope: a share name or a scalar
			if okFieldValue(input, "group", "users", "avahi") {
				return ""
			}
			return validEntryName(input, "share")
		}
		if !okFieldValue(input, "path", "comment", "valid_users", "writable", "public") {
			return `add as "field = value"`
		}
	case profile.FamilyWhen:
		switch strings.TrimSpace(input) {
		case "and", "or", "not":
		default:
			if !okFieldValue(input, "hosts", "users", "os", "gpu", "kernel", "packages", "tools") {
				return `add "and", "or", "not" or "field = value"`
			}
		}
	}
	return ""
}

// validateDotLine is the writes form's line-grammar tier-1: a non-empty
// space-free target and a non-empty line, joined by pathSep.
func validateDotLine(input string) string {
	target, line, _ := strings.Cut(input, pathSep)
	target, line = strings.TrimSpace(target), strings.TrimSpace(line)
	switch {
	case target == "", strings.ContainsAny(target, " \t"), line == "":
		return `add as "target = line text"`
	}
	return ""
}

// addDotLine creates a writes entry in line mode: the entry ensures the
// exact line exists in the target, no mode.
func addDotLine(cfg *profile.ModuleConfig, target, line string) error {
	target, line = strings.TrimSpace(target), strings.TrimSpace(line)
	if target == "" || line == "" {
		return errors.New(`add as "target = line text"`)
	}
	if cfg.Dotfiles == nil {
		cfg.Dotfiles = map[string]profile.Dotfile{}
	}
	cfg.Dotfiles[target] = profile.Dotfile{Line: line}
	return nil
}

// okFieldValue checks `field = value` input against a field set: the
// field must be known and the value must not be empty (an add that sets
// nothing is refused — clearing happens by editing the row).
func okFieldValue(input string, fields ...string) bool {
	field, value, ok := strings.Cut(input, "=")
	return ok && strings.TrimSpace(value) != "" && slices.Contains(fields, strings.TrimSpace(field))
}

// validEntryName is the bare-name tier-1 for sections whose entries grow
// their fields after the add (mounts, smb shares).
func validEntryName(input, what string) string {
	name := strings.TrimSpace(input)
	if name == "" {
		return what + " name must not be empty"
	}
	if strings.ContainsAny(name, " \t") {
		return what + " name must not contain spaces"
	}
	return ""
}

// validUnitName mirrors resolve's structural unit-name contract: non-empty
// and only letters, numbers, '.', '_', '-', '@' (resolve stays the
// authority — kind derivation runs at apply).
func validUnitName(input string) string {
	name := strings.TrimSpace(input)
	if name == "" {
		return "unit name must not be empty"
	}
	for _, r := range name {
		switch {
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r >= '0' && r <= '9',
			r == '.', r == '_', r == '-', r == '@':
		default:
			return `unit name must contain only letters, numbers, '.', '_', '-', or '@'`
		}
	}
	return ""
}

// crossCheck is tier-2, authoritative at save: relationships and
// structural contracts between fields (the workspace can stage an entry
// whose required fields are still empty). Returns the first violation,
// or "".
func crossCheck(cfg *profile.ModuleConfig) string {
	if cfg == nil {
		return ""
	}
	absent := map[string]bool{}
	for _, p := range cfg.Packages.Absent {
		absent[p] = true
	}
	for _, p := range cfg.Packages.Present {
		if absent[p] {
			return fmt.Sprintf("packages: %q is both present and absent", p)
		}
	}
	for _, name := range slices.Sorted(maps.Keys(cfg.Mounts)) {
		m := cfg.Mounts[name]
		switch {
		case m.Source == "":
			return fmt.Sprintf("mounts: %q: source is required", name)
		case m.Destination == "":
			return fmt.Sprintf("mounts: %q: destination is required", name)
		case m.Type == "":
			return fmt.Sprintf("mounts: %q: type is required", name)
		}
	}
	for _, name := range slices.Sorted(maps.Keys(cfg.Smb.Shares)) {
		if cfg.Smb.Shares[name].Path == "" {
			return fmt.Sprintf("smb: share %q: path is required", name)
		}
	}
	if e := checkWhenGroups("", cfg.When); e != "" {
		return e
	}
	return ""
}

// applyEdit commits the input onto the row's field. It mutates a clone,
// re-encodes the family, splices, and strict-decodes the result — a
// structural failure refuses the commit (the field stays open); a
// semantic tier-1 error stages visibly and blocks the save instead.
// Returns false while the edit stays open.
func (w *workspaceModel) applyEdit() bool {
	e := w.editing
	input := e.inputString()
	row := wsRow{}
	if !e.add && e.row < len(w.rows) {
		row = w.rows[e.row]
	}
	family, key := row.family, row.key
	section := row.section
	addPath := ""
	if e.add {
		section = e.section
		family = addFamily(section)
		key = "new"
		addPath = e.addPath
	}

	// Raw mode: the row is a text line; the commit splices the line back
	// and re-parses. A file that parses again unlocks structured editing.
	if family == "raw" {
		return w.applyRawLine(e)
	}

	if e.add {
		// The writes form's line kind travels as "target<pathSep>line":
		// a line may hold spaces and '=', which the link grammar cannot
		// express unambiguously.
		if strings.Contains(input, pathSep) {
			e.err = validateDotLine(input)
		} else {
			e.err = validateAdd(family, addPath, input)
		}
		if e.err != "" {
			return false // an add that fails tier-1 never enters the draft
		}
	} else {
		e.err = validateField(family, key, input)
	}

	d := w.forkDraft()
	candidate := decodeModule(w.readPath(), d.raw)
	if candidate == nil {
		e.err = "the draft no longer parses — discard it"
		return false
	}
	var err error
	newKey := ""
	if target, line, isLine := strings.Cut(input, pathSep); e.add && isLine {
		err = addDotLine(candidate, target, line)
		newKey = strings.TrimSpace(target)
	} else if isStructuralFamily(family) {
		newKey, err = mutateStructural(candidate, family, addPath, key, input, e.add)
	} else {
		err = mutateField(candidate, section, family, key, row.value, input, e.add)
	}
	if err != nil {
		e.err = err.Error()
		return false
	}

	block := encodeFamily(family, candidate)
	spliced := tomlsplice.Splice(d.raw, map[string]string{family: block})
	if decodeModule(w.read.Path, spliced) == nil {
		e.err = "the edit does not produce a valid module.toml"
		return false
	}

	// The row's post-commit identity: adds land on a predicted key, edits
	// may rename (packages, hooks), structural adds report their key.
	want := renamedKey(section, family, key, input)
	if e.add {
		want = newRowKey(section, input)
		if newKey != "" {
			want = newKey
		}
	}

	d.cfg = candidate
	d.raw = spliced
	d.touched[family] = true
	d.changes++
	rk := rowKey(family, want)
	d.edited[rk] = true
	if e.err != "" {
		d.errs[rk] = e.err
	} else {
		delete(d.errs, rk)
	}
	w.rows = wsRows(d.cfg, w.needsRoot)
	// The cursor follows the row's NEW identity (a rename moves it).
	w.cursor = w.rowIndexOf(section, want, input)
	return true
}

// structuralEntryKey lives in structural.go with the mutation it
// mirrors.

// renamedKey computes a row's key after an edit committed input over it.
func renamedKey(section, family, key, input string) string {
	switch family {
	case profile.FamilyPackages:
		name := strings.TrimPrefix(strings.TrimSpace(input), "-")
		if strings.HasPrefix(strings.TrimSpace(input), "-") {
			return "absent:" + name
		}
		return "present:" + name
	case profile.FamilyHooks:
		phase, _, _ := strings.Cut(key, ":")
		return phase + ":" + input
	}
	return key
}

// applyRawLine splices the edited line back into the raw text. Returns
// true when the line edit closes (always — a raw line carries no
// validation; the re-parse decides the mode).
func (w *workspaceModel) applyRawLine(e *wsEdit) bool {
	row := w.rows[e.row]
	d := w.forkDraft()
	lines := strings.Split(strings.TrimRight(d.raw, "\n"), "\n")
	idx := rawLineIndex(row.key)
	if idx < 0 || idx >= len(lines) {
		return true
	}
	lines[idx] = e.inputString()
	d.raw = strings.Join(lines, "\n") + "\n"
	d.changes++
	d.edited[row.key] = true
	if cfg := decodeModule(w.readPath(), d.raw); cfg != nil {
		d.cfg = cfg // parses again: structured editing unlocked
		w.rows = wsRows(cfg, w.needsRoot)
		w.schemaErr = nil
		w.rawErr = ""
		w.cursor = 0
	} else {
		w.rows = rawRows(d.raw)
		w.cursor = min(idx, len(w.rows)-1)
	}
	return true
}

// readPath returns the path of the landed read, or the active layer
// dir's module.toml when the read is a SchemaError (no read landed).
func (w *workspaceModel) readPath() string {
	if w.read != nil {
		return w.read.Path
	}
	return w.activeDir() + "/module.toml"
}

// rawLineIndex parses the "line:N" row key.
func rawLineIndex(key string) int {
	var n int
	if _, err := fmt.Sscanf(key, "line:%d", &n); err != nil {
		return -1
	}
	return n
}

// rawRows builds selectable rows from raw text lines.
func rawRows(raw string) []wsRow {
	var rows []wsRow
	for i, l := range strings.Split(strings.TrimRight(raw, "\n"), "\n") {
		rows = append(rows, wsRow{
			section: "raw",
			text:    l,
			key:     fmt.Sprintf("line:%d", i),
			value:   l,
			family:  "raw",
		})
	}
	return rows
}

// addFamily maps an addable section to its splice family.
func addFamily(section string) string {
	switch section {
	case "packages":
		return profile.FamilyPackages
	case "tools":
		return profile.FamilyTools
	case "hooks":
		return profile.FamilyHooks
	case "links":
		return profile.FamilyDotfiles
	case "systemd.units":
		return profile.FamilySystemd
	case "secrets":
		return profile.FamilySecrets
	case "mounts":
		return profile.FamilyMounts
	case "smb":
		return profile.FamilySmb
	case "when":
		return profile.FamilyWhen
	case "writes":
		// 0077: a line- or link-write entry is creatable from the TUI.
		return profile.FamilyDotfiles
	}
	return ""
}

// addable reports whether a section grows rows with `a`.
func addable(section string) bool { return addFamily(section) != "" }

// mutateField applies the committed input to the candidate config. The
// mutations are deliberately plain field writes — the encoders and the
// splice round-trip carry the correctness proof.
func mutateField(cfg *profile.ModuleConfig, section, family, key, oldValue, input string, add bool) error {
	switch family {
	case profile.FamilyKeys:
		switch key {
		case "description":
			cfg.Description = input
		case "app":
			cfg.App = input
		case "scope":
			cfg.Scope = input
		}
	case profile.FamilyPackages:
		name := strings.TrimSpace(input)
		absent := strings.HasPrefix(name, "-")
		name = strings.TrimPrefix(name, "-")
		if add {
			if absent {
				cfg.Packages.Absent = append(cfg.Packages.Absent, name)
			} else {
				cfg.Packages.Present = append(cfg.Packages.Present, name)
			}
			return nil
		}
		removePackage(cfg, oldValue)
		if absent {
			cfg.Packages.Absent = append(cfg.Packages.Absent, name)
		} else {
			cfg.Packages.Present = append(cfg.Packages.Present, name)
		}
	case profile.FamilyTools:
		if add {
			name, constraint, ok := strings.Cut(input, "=")
			if !ok {
				return fmt.Errorf(`add as "name = constraint"`)
			}
			if cfg.Tools == nil {
				cfg.Tools = map[string]string{}
			}
			cfg.Tools[strings.TrimSpace(name)] = strings.TrimSpace(constraint)
			return nil
		}
		cfg.Tools[key] = input
	case profile.FamilyHooks:
		if add {
			phase, cmd := splitPhase(input)
			if phase == "post" {
				cfg.Hooks.Post = append(cfg.Hooks.Post, profile.HookCommand{Command: cmd})
			} else {
				cfg.Hooks.Pre = append(cfg.Hooks.Pre, profile.HookCommand{Command: cmd})
			}
			return nil
		}
		phase, _, _ := strings.Cut(key, ":")
		replaceHook(cfg, phase, oldValue, input)
	case profile.FamilyDotfiles:
		if add {
			parts := strings.Fields(input)
			if len(parts) != 2 {
				return fmt.Errorf(`add as "target source"`)
			}
			if cfg.Dotfiles == nil {
				cfg.Dotfiles = map[string]profile.Dotfile{}
			}
			cfg.Dotfiles[parts[0]] = profile.Dotfile{Source: parts[1], Mode: "symlink"}
			return nil
		}
		d := cfg.Dotfiles[key]
		if d.Line != "" {
			d.Line = input // a writes row edits its line text
		} else {
			d.Source = input
		}
		cfg.Dotfiles[key] = d
	}
	return nil
}

func removePackage(cfg *profile.ModuleConfig, value string) {
	name := strings.TrimPrefix(value, "-")
	cfg.Packages.Present = slices.DeleteFunc(cfg.Packages.Present, func(p string) bool { return p == name })
	cfg.Packages.Absent = slices.DeleteFunc(cfg.Packages.Absent, func(p string) bool { return p == name })
}

func splitComma(s string) []string {
	var out []string
	for part := range strings.SplitSeq(s, ",") {
		if p := strings.TrimSpace(part); p != "" {
			out = append(out, p)
		}
	}
	return out
}

func splitPhase(input string) (phase, cmd string) {
	if p, c, ok := strings.Cut(input, ":"); ok && (p == "pre" || p == "post") {
		return p, strings.TrimSpace(c)
	}
	return "pre", input
}

func replaceHook(cfg *profile.ModuleConfig, phase, old, new string) {
	rows := cfg.Hooks.Pre
	if phase == "post" {
		rows = cfg.Hooks.Post
	}
	for i, h := range rows {
		if h.Command == old {
			rows[i].Command = new
		}
	}
}

// removeRow deletes the cursor row's entry from the draft (the `d`
// confirm's yes branch).
func (w *workspaceModel) removeRow() {
	row := w.rows[w.cursor]
	d := w.forkDraft()
	candidate := decodeModule(w.readPath(), d.raw)
	if candidate == nil {
		return
	}
	switch row.family {
	case profile.FamilyPackages:
		removePackage(candidate, row.value)
	case profile.FamilyTools:
		delete(candidate.Tools, row.key)
	case profile.FamilyHooks:
		phase, _, _ := strings.Cut(row.key, ":")
		rows := candidate.Hooks.Pre
		if phase == "post" {
			rows = candidate.Hooks.Post
		}
		rows = slices.DeleteFunc(rows, func(h profile.HookCommand) bool { return h.Command == row.value })
		if phase == "post" {
			candidate.Hooks.Post = rows
		} else {
			candidate.Hooks.Pre = rows
		}
	case profile.FamilyDotfiles:
		delete(candidate.Dotfiles, row.key)
	case profile.FamilySystemd:
		if row.container {
			delete(candidate.Systemd.Units, row.key)
		} else {
			parts := splitPath(row.key)
			if len(parts) != 2 {
				return
			}
			unit := candidate.Systemd.Units[parts[0]]
			if unit != nil {
				delete(unit, parts[1])
				candidate.Systemd.Units[parts[0]] = unit
			}
		}
	case profile.FamilySecrets:
		if row.container {
			delete(candidate.Secrets, row.key)
		}
	case profile.FamilyMounts:
		if row.container {
			delete(candidate.Mounts, row.key)
		}
	case profile.FamilySmb:
		if row.container {
			delete(candidate.Smb.Shares, row.key)
		}
	case profile.FamilyWhen:
		if !row.container {
			return // leaves clear by editing them to empty
		}
		parts := splitPath(row.key)
		if parts[len(parts)-1] == "not" {
			if parent := whenNode(&candidate.When, parts[:len(parts)-1]); parent != nil {
				parent.Not = nil
			}
		} else {
			parent := whenNode(&candidate.When, parts[:len(parts)-2])
			if parent == nil {
				return
			}
			idx, err := strconv.Atoi(parts[len(parts)-1])
			if err != nil {
				return
			}
			switch parts[len(parts)-2] {
			case "and":
				if idx < len(parent.And) {
					parent.And = slices.Delete(parent.And, idx, idx+1)
				}
			case "or":
				if idx < len(parent.Or) {
					parent.Or = slices.Delete(parent.Or, idx, idx+1)
				}
			}
		}
	default:
		return
	}
	block := encodeFamily(row.family, candidate)
	d.raw = tomlsplice.Splice(d.raw, map[string]string{row.family: block})
	d.cfg = candidate
	d.touched[row.family] = true
	d.changes++
	delete(d.errs, rowKey(row.family, row.key))
	w.rows = wsRows(candidate, w.needsRoot)
	if w.cursor >= len(w.rows) {
		w.cursor = max(0, len(w.rows)-1)
	}
}

// rowIndexOf lands the cursor on the just-committed row after a rebuild.
func (w *workspaceModel) rowIndexOf(section, key, input string) int {
	want := key
	if key == "new" {
		want = newRowKey(section, input)
	}
	for i, r := range w.rows {
		if r.section == section && r.key == want {
			return i
		}
	}
	return w.firstEntry(0)
}

// newRowKey predicts a committed add row's key.
func newRowKey(section, input string) string {
	switch section {
	case "packages":
		name := strings.TrimSpace(input)
		if strings.HasPrefix(name, "-") {
			return "absent:" + strings.TrimPrefix(name, "-")
		}
		return "present:" + name
	case "tools":
		name, _, _ := strings.Cut(input, "=")
		return strings.TrimSpace(name)
	case "hooks":
		phase, cmd := splitPhase(input)
		return phase + ":" + cmd
	case "links":
		parts := strings.Fields(input)
		if len(parts) == 2 {
			return parts[0]
		}
	}
	return ""
}

// --- Compositor-side editing actions ---

// startEdit opens the field input on the cursor row (enter/e). Read-only
// rows and rows without metadata refuse silently — the row already shows
// everything it can. A closed-set field (0076) opens the choice picker
// instead of the text input. A row that owns no field — a section header
// or a structural container (0078) — grows rows instead: enter falls
// back to the add form, so the empty section under the cursor is
// reachable without knowing a second key; startAdd refuses what is not
// addable.
func (m *Compositor) startEdit() {
	if m.ws.cursor >= len(m.ws.rows) {
		return
	}
	row := m.ws.rows[m.ws.cursor]
	if row.family == "" || row.container {
		m.startAdd()
		return
	}
	if set := choiceSet(row); set != nil {
		m.openChoice(row, set)
		return
	}
	m.ws.editing = &wsEdit{row: m.ws.cursor, input: []rune(row.value), cur: len([]rune(row.value))}
}

// openChoice pushes the row's picker. The seam commits through the same
// path as the text input's enter; picking the effective value — editing
// an unset scope to its own default, say — stages nothing.
func (m *Compositor) openChoice(row wsRow, set []string) {
	current := effectiveChoice(row)
	rowIdx := m.ws.cursor
	m.modals = append(m.modals, newChoiceModel(m.th, choiceTitle(row), set, current, func(value string) {
		value = commitValue(value)
		if value == current {
			return
		}
		m.commitFieldAt(rowIdx, value)
	}))
}

// commitFieldAt commits value onto row i as if the text input had
// received it: applyEdit validates, splices, and refreshes the surface.
// A picker never leaves an input open behind the modal.
func (m *Compositor) commitFieldAt(row int, value string) {
	m.ws.editing = &wsEdit{row: row, input: []rune(value), cur: len([]rune(value))}
	if m.ws.applyEdit() {
		m.finishFieldCommit()
	} else {
		m.ws.editing = nil
	}
}

// startAdd opens the add form for the cursor row (a). The row decides
// the scope (addScope): a header adds at the section's entry level (an
// empty section's only way in; the when/smb headers are those sections'
// root scope), a container adds `field = value` INTO its entry, systemd
// directives and when leaves add into their scope, and every other row
// adds the section's next entry. The form's commit runs the pipeline's
// add path; the section's spec owns the rows.
func (m *Compositor) startAdd() {
	if m.ws.cursor >= len(m.ws.rows) {
		return
	}
	row := m.ws.rows[m.ws.cursor]
	if !addable(row.section) || m.ws.schemaErr != nil {
		return
	}
	section, addPath := addScope(row)
	title, rows, build, relabel := addFormSpec(m.ws.moduleID, section, addPath)
	if rows == nil {
		return
	}
	form := newAddForm(m.th, title, rows, build, func(input string) string {
		return m.commitAddAt(m.ws.cursor, section, addPath, input)
	})
	form.relabel = relabel
	m.modals = append(m.modals, form)
}

// commitAddAt commits a form's synthesized input through the pipeline's
// add path — the same wsEdit an inline input would have produced. A
// refusal returns the tier-1 error; nothing stages.
func (m *Compositor) commitAddAt(row int, section, addPath, input string) string {
	m.ws.editing = &wsEdit{row: row, add: true, section: section, addPath: addPath, input: []rune(input), cur: len([]rune(input))}
	if m.ws.applyEdit() {
		m.finishFieldCommit()
		return ""
	}
	err := m.ws.editing.err
	m.ws.editing = nil
	return err
}

// editKey routes one key to the active field input: runes insert at the
// cursor, arrows move, backspace deletes, enter commits, esc cancels.
// Everything is swallowed — no key falls through to the base while a
// field is open.
func (m *Compositor) editKey(k tea.KeyPressMsg) tea.Cmd {
	e := m.ws.editing
	switch k.String() {
	case "esc":
		m.ws.editing = nil
		return nil
	case "enter":
		if m.ws.applyEdit() {
			m.finishFieldCommit()
		}
		return nil
	case "left":
		if e.cur > 0 {
			e.cur--
		}
	case "right":
		if e.cur < len(e.input) {
			e.cur++
		}
	case "backspace":
		if e.cur > 0 {
			e.input = append(e.input[:e.cur-1], e.input[e.cur:]...)
			e.cur--
		}
	default:
		if k.Text != "" {
			runes := []rune(k.Text)
			e.input = append(e.input[:e.cur], append(runes, e.input[e.cur:]...)...)
			e.cur += len(runes)
		}
	}
	// Live tier-1 at the field while typing.
	if !e.add {
		row := m.ws.rows[e.row]
		e.err = validateField(row.family, row.key, e.inputString())
	} else {
		e.err = validateAdd(addFamily(e.section), e.addPath, e.inputString())
	}
	return nil
}

// finishFieldCommit closes a committed field edit: the ledger takes the
// draft, and a raw-mode file that parses again says so once.
func (m *Compositor) finishFieldCommit() {
	m.ws.editing = nil
	m.store[m.ws.activeDir()] = m.ws.draft
	if m.ws.draft != nil && m.ws.draft.cfg != nil && m.ws.draft.rawMode && m.ws.schemaErr == nil {
		m.message, m.msgErr = "parses again — structured editing unlocked", false
	}
}

// confirmDiscard pushes the discard-draft confirm (D): the question, the
// full target identity, and the staged-change count.
func (m *Compositor) confirmDiscard() {
	d := m.ws.draft
	if d == nil {
		return
	}
	dir := m.ws.activeDir()
	changes := "1 change"
	if d.changes != 1 {
		changes = fmt.Sprintf("%d changes", d.changes)
	}
	m.modals = append(m.modals, &confirmModel{
		th:    m.th,
		title: "discard draft?",
		body: []string{
			m.ws.moduleID + " · " + m.ws.tabs[m.ws.active].label,
			changes + " — this cannot be undone",
		},
		onAnswer: func(ok bool) {
			if !ok {
				return
			}
			delete(m.store, dir)
			m.ws.draft = nil
			if m.ws.read != nil {
				m.ws.rows = wsRows(m.ws.read.Config, m.ws.needsRoot)
			} else if m.ws.schemaErr != nil {
				m.ws.rows = rawRows(m.ws.rawErr)
			}
			if m.ws.cursor >= len(m.ws.rows) {
				m.ws.cursor = max(0, len(m.ws.rows)-1)
			}
		},
	})
}

// confirmRemoveRow pushes the remove-row confirm (d) for table entry
// rows.
func (m *Compositor) confirmRemoveRow() {
	if m.ws.cursor >= len(m.ws.rows) || m.ws.schemaErr != nil {
		return
	}
	row := m.ws.rows[m.ws.cursor]
	switch row.family {
	case profile.FamilyPackages, profile.FamilyTools, profile.FamilyHooks, profile.FamilyDotfiles, profile.FamilySystemd:
	case profile.FamilySecrets, profile.FamilyMounts, profile.FamilySmb, profile.FamilyWhen:
		if !row.container {
			return // d removes entries and groups, not their fields
		}
	default:
		return
	}
	m.modals = append(m.modals, &confirmModel{
		th:    m.th,
		title: "remove row?",
		body: []string{
			m.ws.moduleID + " · " + m.ws.tabs[m.ws.active].label,
			row.text + "  (" + row.section + ")",
		},
		onAnswer: func(ok bool) {
			if !ok {
				return
			}
			m.ws.removeRow()
			m.store[m.ws.activeDir()] = m.ws.draft
		},
	})
}

// saveFinishedMsg carries the save pipeline's outcome.
type saveFinishedMsg struct {
	dir string
	err error
}

// saveDraft runs ctrl+s: tier-1 staged errors and the tier-2 cross-check
// block in place; the save itself is the untouched 0065 pipeline, run
// async with the footer spinner.
func (m *Compositor) saveDraft() tea.Cmd {
	d := m.ws.draft
	if d == nil {
		m.message, m.msgErr = "no changes", false
		return nil
	}
	if len(d.errs) > 0 {
		var first string
		for _, k := range slices.Sorted(maps.Keys(d.errs)) {
			first = d.errs[k]
			break
		}
		m.message = fmt.Sprintf("fix %d field error(s) first: %s", len(d.errs), first)
		m.msgErr = true
		return nil
	}
	if cc := crossCheck(d.cfg); cc != "" {
		m.message, m.msgErr = cc, true
		return nil
	}
	w, ok := m.reader.(LayerWriter)
	if !ok {
		m.message, m.msgErr = "no writer for this layer", true
		return nil
	}
	req := service.SaveRequest{Dir: m.ws.activeDir(), BaseHash: d.baseHash}
	if d.rawMode {
		raw := d.raw
		req.Raw = &raw
	} else {
		req.Replacements = map[string]string{}
		for f := range d.touched {
			req.Replacements[f] = encodeFamily(f, d.cfg)
		}
	}
	// The op announces itself directly and the save runs as one plain cmd:
	// tea.Sequence/Batch produce runtime-internal messages the compositor
	// never sees, so they cannot drive the chrome here.
	m.op = "saving " + m.ws.moduleID
	m.message, m.msgErr = "", false
	m.msgToken++
	return func() tea.Msg {
		_, err := w.WriteModuleLayer(req)
		return saveFinishedMsg{dir: req.Dir, err: err}
	}
}

// saveFinished applies the outcome: the conflict modal, the failure note,
// or the cleared draft plus a re-read of the saved file.
func (m *Compositor) saveFinished(msg saveFinishedMsg) tea.Cmd {
	m.op = ""
	m.msgToken++
	var conflict *service.DiskHashConflictError
	switch {
	case errors.As(msg.err, &conflict):
		m.pushConflictModal(msg.dir)
		return nil
	case msg.err != nil:
		m.message, m.msgErr = firstLineOf(msg.err.Error()), true
		return nil
	}
	delete(m.store, msg.dir)
	if m.ws.activeDir() == msg.dir {
		m.ws.draft = nil
		m.ws.loadedDir = "" // force a re-read of the saved file
	}
	// The success note persists until the next op or user action (no fade
	// tick: a tick is a real-time wait the message-driven tests cannot
	// settle, and a standing "saved" note is harmless).
	m.message, m.msgErr = "saved "+m.ws.moduleID, false
	return m.loadLayer()
}

// pushConflictModal surfaces the disk-hash refusal: reload (draft
// discarded) or keep editing (0065-D8: no merge, no clobber).
func (m *Compositor) pushConflictModal(dir string) {
	m.modals = append(m.modals, &confirmModel{
		th:    m.th,
		title: "file changed on disk",
		body: []string{
			m.ws.moduleID + " · " + m.ws.tabs[m.ws.active].label,
			"y reloads the file (draft discarded)",
			"n keeps editing",
		},
		onAnswer: func(ok bool) {
			if !ok {
				return
			}
			delete(m.store, dir)
			m.ws.draft = nil
			m.ws.loadedDir = ""
			m.afterPop = func() tea.Cmd { return m.loadLayer() }
		},
	})
}

// confirmModel is the one confirm component: the question as title, the
// consequence as body, y confirms, n/esc cancels. It pops itself by
// reporting finished() after an answer.
type confirmModel struct {
	th       theme
	title    string
	body     []string
	onAnswer func(ok bool)
	done     bool
}

func (c *confirmModel) finished() bool { return c.done }

func (c *confirmModel) update(msg tea.Msg) tea.Cmd {
	k, ok := msg.(tea.KeyPressMsg)
	if !ok {
		return nil
	}
	switch k.String() {
	case "y":
		c.done = true
		c.onAnswer(true)
	default: // y alone confirms; every other key cancels
		c.done = true
		if c.onAnswer != nil {
			c.onAnswer(false)
		}
	}
	return nil
}

func (c *confirmModel) view(w, h int) string {
	lines := []string{c.th.modalTitle.Render(c.title), ""}
	lines = append(lines, c.body...)
	lines = append(lines, "", c.th.meta.Render("y confirm · n/esc cancel"))
	return c.th.modalBorder.Padding(1, 2).Render(strings.Join(lines, "\n"))
}
