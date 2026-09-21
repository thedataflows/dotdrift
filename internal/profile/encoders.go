package profile

import (
	"maps"
	"slices"
	"strconv"
	"strings"

	"github.com/thedataflows/dotdrift/internal/tomlsplice"
)

// Per-section encoders (0065-D7's layer 2): typed section values →
// canonical TOML text, one block per section family, splice-ready. The
// blocks are deterministic — keyed tables emit sorted keys, ordered arrays
// preserve their order — so encode ∘ decode ∘ encode is a fixed point
// (tested). Editing a section re-encodes it canonically: interior comments
// go; untouched sections never lose a byte, which is tomlsplice's contract.
//
// generate's whole-file encoder (BurntSushi, writer.go) is a different
// tool for a different job: fresh-module creation, where there are no
// comments to preserve (0065-D1).

// Section families — the splice keys the editor adapters and the save
// pipeline stage blocks under. FamilyKeys (the preamble holding the
// module-level keys) comes from tomlsplice.
const (
	FamilyKeys     = tomlsplice.FamilyKeys
	FamilyPackages = "packages"
	FamilyTools    = "tools"
	FamilySecrets  = "secrets"
	FamilyDotfiles = "dotfiles"
	FamilyWhen     = "when"
	FamilyHooks    = "hooks"
	FamilyMounts   = "mounts"
	FamilySmb      = "smb"
	FamilySystemd  = "systemd"
)

// PackageEntry is one declared distro package with its optional cosmetic
// description, rendered as a trailing "# <desc>" comment next to the
// entry. The description is encode-side only: resolve reads back just the
// bare names (the TOML parser ignores comments), so it never enters the
// data model.
type PackageEntry struct {
	Name        string
	Description string
}

// HookRow is one hooks entry with its TOML spelling for encoding. A
// Structured row renders as a table ({ command = …, optional = true });
// an all-plain list keeps the concise array spelling (pre = ["a"]).
type HookRow struct {
	Command    string
	Optional   bool
	Structured bool
}

// HookRows converts decoded hook commands into rows, inferring the
// spelling: an optional command must be structured (a bare string cannot
// carry the flag).
func HookRows(cmds []HookCommand) []HookRow {
	rows := make([]HookRow, 0, len(cmds))
	for _, c := range cmds {
		rows = append(rows, HookRow{Command: c.Command, Optional: c.Optional, Structured: c.Optional})
	}
	return rows
}

// PackageEntries converts bare package names into entries without
// descriptions (the decode side of the packages section).
func PackageEntries(names []string) []PackageEntry {
	out := make([]PackageEntry, 0, len(names))
	for _, n := range names {
		out = append(out, PackageEntry{Name: n})
	}
	return out
}

// EncodeKeysSection renders the module-level keys (the preamble block:
// id, description, disabled, scope). Zero values are omitted — a
// minimal scaffold stays minimal.
func EncodeKeysSection(cfg ModuleConfig) string {
	var b strings.Builder
	if cfg.ID != "" {
		b.WriteString("id = " + tomlBasicString(cfg.ID) + "\n")
	}
	if cfg.Description != "" {
		b.WriteString("description = " + tomlBasicString(cfg.Description) + "\n")
	}
	if cfg.Disabled {
		b.WriteString("disabled = true\n")
	}
	if cfg.Scope != "" {
		b.WriteString("scope = " + tomlBasicString(cfg.Scope) + "\n")
	}
	return b.String()
}

// EncodePackagesSection renders the [packages] block: present (with
// optional description comments) and absent lists, order preserved.
func EncodePackagesSection(present []PackageEntry, absent []string) string {
	if len(present) == 0 && len(absent) == 0 {
		return ""
	}
	var b strings.Builder
	b.WriteString("[packages]\n")
	if len(present) > 0 {
		b.WriteString("present = [\n")
		for _, p := range present {
			if p.Description != "" {
				b.WriteString("  " + tomlBasicString(p.Name) + ", # " + p.Description + "\n")
			} else {
				b.WriteString("  " + tomlBasicString(p.Name) + ",\n")
			}
		}
		b.WriteString("]\n")
	}
	if len(absent) > 0 {
		b.WriteString("absent = [\n")
		for _, name := range absent {
			b.WriteString("  " + tomlBasicString(name) + ",\n")
		}
		b.WriteString("]\n")
	}
	return b.String()
}

// EncodeToolsSection renders the [tools] block, keys sorted.
func EncodeToolsSection(tools map[string]string) string {
	if len(tools) == 0 {
		return ""
	}
	var b strings.Builder
	b.WriteString("[tools]\n")
	for _, k := range slices.Sorted(maps.Keys(tools)) {
		b.WriteString(tomlKey(k) + " = " + tomlBasicString(tools[k]) + "\n")
	}
	return b.String()
}

// EncodeSecretsSection renders the [secrets] block: the short form
// (name = "ENV") when only the env var is set, the table form otherwise.
func EncodeSecretsSection(secrets map[string]Secret) string {
	if len(secrets) == 0 {
		return ""
	}
	var b strings.Builder
	b.WriteString("[secrets]\n")
	for _, name := range slices.Sorted(maps.Keys(secrets)) {
		s := secrets[name]
		if s.Description == "" && !s.AllowEmpty {
			b.WriteString(tomlKey(name) + " = " + tomlBasicString(s.Env) + "\n")
			continue
		}
		parts := []string{"env = " + tomlBasicString(s.Env)}
		if s.Description != "" {
			parts = append(parts, "description = "+tomlBasicString(s.Description))
		}
		if s.AllowEmpty {
			parts = append(parts, "allow_empty = true")
		}
		b.WriteString(tomlKey(name) + " = { " + strings.Join(parts, ", ") + " }\n")
	}
	return b.String()
}

// EncodeDotfilesSection renders the [dotfiles] block: one inline table per
// entry, keys sorted, whole-file and edit fields in schema order.
func EncodeDotfilesSection(dotfiles map[string]Dotfile) string {
	if len(dotfiles) == 0 {
		return ""
	}
	var b strings.Builder
	b.WriteString("[dotfiles]\n")
	for _, k := range slices.Sorted(maps.Keys(dotfiles)) {
		e := dotfiles[k]
		var parts []string
		for _, f := range []struct {
			key string
			val string
		}{
			{"source", e.Source}, {"mode", e.Mode}, {"line", e.Line},
			{"block", e.Block}, {"comment", e.Comment}, {"template", e.Template},
		} {
			if f.val != "" {
				parts = append(parts, f.key+" = "+tomlBasicString(f.val))
			}
		}
		b.WriteString(tomlKey(k) + " = { " + strings.Join(parts, ", ") + " }\n")
	}
	return b.String()
}

// EncodeWhenSection renders the [when] block: leaves in schema order,
// combinators as inline tables.
func EncodeWhenSection(w When) string {
	var b strings.Builder
	first := true
	put := func(key, val string) {
		if first {
			b.WriteString("[when]\n")
		}
		first = false
		b.WriteString(key + " = " + val + "\n")
	}
	if len(w.Hosts) > 0 {
		put("hosts", encodeStringList(w.Hosts))
	}
	if len(w.Users) > 0 {
		put("users", encodeStringList(w.Users))
	}
	if len(w.OS) > 0 {
		put("os", encodeStringList(w.OS))
	}
	if w.GPU != "" {
		put("gpu", tomlBasicString(w.GPU))
	}
	if w.Kernel != "" {
		put("kernel", tomlBasicString(w.Kernel))
	}
	if len(w.Packages) > 0 {
		put("packages", encodeStringList(w.Packages))
	}
	if len(w.Tools) > 0 {
		put("tools", encodeStringList(w.Tools))
	}
	if len(w.And) > 0 {
		items := make([]string, 0, len(w.And))
		for _, sub := range w.And {
			items = append(items, encodeWhenInline(sub))
		}
		put("and", "["+strings.Join(items, ", ")+"]")
	}
	if len(w.Or) > 0 {
		items := make([]string, 0, len(w.Or))
		for _, sub := range w.Or {
			items = append(items, encodeWhenInline(sub))
		}
		put("or", "["+strings.Join(items, ", ")+"]")
	}
	if w.Not != nil {
		put("not", encodeWhenInline(*w.Not))
	}
	if first {
		return ""
	}
	return b.String()
}

// encodeWhenInline renders a when node as one inline table.
func encodeWhenInline(w When) string {
	var parts []string
	if len(w.Hosts) > 0 {
		parts = append(parts, "hosts = "+encodeStringList(w.Hosts))
	}
	if len(w.Users) > 0 {
		parts = append(parts, "users = "+encodeStringList(w.Users))
	}
	if len(w.OS) > 0 {
		parts = append(parts, "os = "+encodeStringList(w.OS))
	}
	if w.GPU != "" {
		parts = append(parts, "gpu = "+tomlBasicString(w.GPU))
	}
	if w.Kernel != "" {
		parts = append(parts, "kernel = "+tomlBasicString(w.Kernel))
	}
	if len(w.Packages) > 0 {
		parts = append(parts, "packages = "+encodeStringList(w.Packages))
	}
	if len(w.Tools) > 0 {
		parts = append(parts, "tools = "+encodeStringList(w.Tools))
	}
	if len(w.And) > 0 {
		items := make([]string, 0, len(w.And))
		for _, sub := range w.And {
			items = append(items, encodeWhenInline(sub))
		}
		parts = append(parts, "and = ["+strings.Join(items, ", ")+"]")
	}
	if len(w.Or) > 0 {
		items := make([]string, 0, len(w.Or))
		for _, sub := range w.Or {
			items = append(items, encodeWhenInline(sub))
		}
		parts = append(parts, "or = ["+strings.Join(items, ", ")+"]")
	}
	if w.Not != nil {
		parts = append(parts, "not = "+encodeWhenInline(*w.Not))
	}
	if len(parts) == 0 {
		return "{}"
	}
	return "{ " + strings.Join(parts, ", ") + " }"
}

func encodeStringList(items []string) string {
	quoted := make([]string, 0, len(items))
	for _, s := range items {
		quoted = append(quoted, tomlBasicString(s))
	}
	return "[" + strings.Join(quoted, ", ") + "]"
}

// EncodeHooksSection renders the hooks block. An all-plain list keeps the
// concise array spelling; any structured row in a list takes the
// [[hooks.pre]]/[[hooks.post]] table spelling, plain rows included.
func EncodeHooksSection(pre, post []HookRow) string {
	if len(pre) == 0 && len(post) == 0 {
		return ""
	}
	var b strings.Builder
	b.WriteString("[hooks]\n")
	if len(pre) > 0 {
		b.WriteString(hookListLine("pre", pre))
	}
	if len(post) > 0 {
		b.WriteString(hookListLine("post", post))
	}
	return b.String()
}

// hookListLine renders one pre/post array line: bare strings for plain
// rows, inline tables for structured ones.
func hookListLine(key string, rows []HookRow) string {
	quoted := make([]string, 0, len(rows))
	for _, r := range rows {
		if r.Structured {
			parts := []string{"command = " + tomlBasicString(r.Command)}
			if r.Optional {
				parts = append(parts, "optional = true")
			}
			quoted = append(quoted, "{ "+strings.Join(parts, ", ")+" }")
			continue
		}
		quoted = append(quoted, tomlBasicString(r.Command))
	}
	return key + " = [" + strings.Join(quoted, ", ") + "]\n"
}

// EncodeMountsSection renders the [mounts.<name>] blocks, names sorted,
// fields in schema order, zeros omitted.
func EncodeMountsSection(mounts map[string]MountSpec) string {
	if len(mounts) == 0 {
		return ""
	}
	var b strings.Builder
	for _, name := range slices.Sorted(maps.Keys(mounts)) {
		m := mounts[name]
		if b.Len() > 0 {
			b.WriteString("\n")
		}
		b.WriteString("[mounts." + tomlKey(name) + "]\n")
		for _, f := range []struct {
			key string
			val string
		}{
			{"source", m.Source}, {"destination", m.Destination}, {"type", m.Type},
		} {
			if f.val != "" {
				b.WriteString(f.key + " = " + tomlBasicString(f.val) + "\n")
			}
		}
		if len(m.Options) > 0 {
			b.WriteString("options = " + encodeStringList(m.Options) + "\n")
		}
		if m.StartAt != "" {
			b.WriteString("startat = " + tomlBasicString(m.StartAt) + "\n")
		}
		if m.State != "" {
			b.WriteString("state = " + tomlBasicString(m.State) + "\n")
		}
	}
	return b.String()
}

// EncodeSmbSection renders the [smb] block plus [smb.shares.<name>]
// sub-tables, share names sorted, zeros omitted. Avahi is emitted only
// when explicitly set (nil keeps the default-on semantics).
func EncodeSmbSection(s SmbSpec) string {
	if s.Group == "" && len(s.Users) == 0 && s.Avahi == nil && len(s.Shares) == 0 {
		return ""
	}
	var b strings.Builder
	b.WriteString("[smb]\n")
	if s.Group != "" {
		b.WriteString("group = " + tomlBasicString(s.Group) + "\n")
	}
	if len(s.Users) > 0 {
		b.WriteString("users = " + encodeStringList(s.Users) + "\n")
	}
	if s.Avahi != nil {
		b.WriteString("avahi = " + strconv.FormatBool(*s.Avahi) + "\n")
	}
	for _, name := range slices.Sorted(maps.Keys(s.Shares)) {
		sh := s.Shares[name]
		b.WriteString("\n[smb.shares." + tomlKey(name) + "]\n")
		for _, f := range []struct {
			key string
			val string
		}{
			{"path", sh.Path}, {"comment", sh.Comment}, {"valid_users", sh.ValidUsers},
		} {
			if f.val != "" {
				b.WriteString(f.key + " = " + tomlBasicString(f.val) + "\n")
			}
		}
		if sh.Writable {
			b.WriteString("writable = true\n")
		}
		if sh.Public {
			b.WriteString("public = true\n")
		}
	}
	return b.String()
}

// EncodeSystemdSection renders [systemd.units."<name>"] blocks: unit names
// quoted (they carry dots), directives free-form passthrough with any TOML
// value type, keys sorted per unit.
func EncodeSystemdSection(units map[string]SystemdUnit) string {
	if len(units) == 0 {
		return ""
	}
	var b strings.Builder
	for _, name := range slices.Sorted(maps.Keys(units)) {
		unit := units[name]
		if b.Len() > 0 {
			b.WriteString("\n")
		}
		b.WriteString("[systemd.units." + tomlBasicString(name) + "]\n")
		for _, directive := range slices.Sorted(maps.Keys(unit)) {
			if v := EncodeTomlValue(unit[directive]); v != "" {
				b.WriteString(tomlKey(directive) + " = " + v + "\n")
			}
		}
	}
	return b.String()
}

// EncodeTomlValue renders any passthrough directive value as TOML text —
// the one value renderer for encoders and editors alike. A nil or
// unsupported value renders empty.
func EncodeTomlValue(v any) string {
	switch val := v.(type) {
	case string:
		return tomlBasicString(val)
	case bool:
		return strconv.FormatBool(val)
	case int64:
		return strconv.FormatInt(val, 10)
	case int:
		return strconv.Itoa(val)
	case float64:
		return strconv.FormatFloat(val, 'f', -1, 64)
	case []any:
		items := make([]string, 0, len(val))
		for _, item := range val {
			if s := EncodeTomlValue(item); s != "" {
				items = append(items, s)
			}
		}
		return "[" + strings.Join(items, ", ") + "]"
	case map[string]any:
		pairs := make([]string, 0, len(val))
		for _, k := range slices.Sorted(maps.Keys(val)) {
			if s := EncodeTomlValue(val[k]); s != "" {
				pairs = append(pairs, tomlKey(k)+" = "+s)
			}
		}
		return "{ " + strings.Join(pairs, ", ") + " }"
	default:
		return ""
	}
}

// tomlBasicString renders s as a TOML basic string with the minimal
// escaping the TOML spec requires (control chars, quote, backslash).
func tomlBasicString(s string) string {
	var b strings.Builder
	b.WriteByte('"')
	for _, r := range s {
		switch r {
		case '"':
			b.WriteString(`\"`)
		case '\\':
			b.WriteString(`\\`)
		case '\n':
			b.WriteString(`\n`)
		case '\t':
			b.WriteString(`\t`)
		case '\r':
			b.WriteString(`\r`)
		default:
			if r < 0x20 {
				b.WriteString(`\u` + strconv.FormatInt(int64(r), 16))
			} else {
				b.WriteRune(r)
			}
		}
	}
	b.WriteByte('"')
	return b.String()
}

// tomlKey renders a key bare when it matches the simple-key charset
// ([A-Za-z0-9_-]+), quoted otherwise.
func tomlKey(k string) string {
	if isBareKey(k) {
		return k
	}
	return tomlBasicString(k)
}

func isBareKey(k string) bool {
	if k == "" {
		return false
	}
	for _, r := range k {
		switch {
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r >= '0' && r <= '9', r == '_' || r == '-':
		default:
			return false
		}
	}
	return true
}
