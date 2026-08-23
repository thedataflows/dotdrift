// Package palette holds the named color roles for dotdrift's colored CLI
// output and their per-profile overrides.
package palette

import (
	"fmt"
	"sort"
	"strings"
)

// Role is one named color slot. The vocabulary is shared by status,
// modules, plan, and diff output.
type Role string

const (
	OK      Role = "ok"      // all-checks-passed, no-drift
	Missing Role = "missing" // missing/removed items (light red)
	Warn    Role = "warn"    // content/version differs (yellow)
	Error   Role = "error"   // unknown, not-a-symlink (red)
	Orphan  Role = "orphan"  // orphans section (magenta)
	Dim     Role = "dim"     // dimmed module/description text
)

// roles is the valid-role set, used for validation error messages.
var roles = []Role{OK, Missing, Warn, Error, Orphan, Dim}

func init() {
	sort.Slice(roles, func(i, j int) bool { return roles[i] < roles[j] })
}

// Palette maps roles to raw SGR parameter sequences (no ESC/CSI wrapper).
type Palette struct {
	seqs map[Role]string
}

// Default returns the built-in palette. Missing stays orange (38;5;208)
// — the light-red experiment was reverted; orange keeps missing visually
// distinct from hard failures (error red) while reading as "absent, not
// broken".
func Default() *Palette {
	return &Palette{seqs: map[Role]string{
		OK:      "32",
		Missing: "38;5;208",
		Warn:    "33",
		Error:   "31",
		Orphan:  "35",
		Dim:     "90",
	}}
}

// FromConfig builds a palette from [colors] overrides (role → raw SGR
// params). Unknown roles and malformed values are errors — a typo must
// fail loudly at dotdrift.toml load, never silently keep the default.
func FromConfig(cfg map[string]string) (*Palette, error) {
	p := Default()
	valid := make(map[Role]struct{}, len(roles))
	for _, r := range roles {
		valid[r] = struct{}{}
	}
	// Deterministic error for multiple bad keys: check sorted order.
	keys := make([]string, 0, len(cfg))
	for k := range cfg {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		role := Role(k)
		if _, ok := valid[role]; !ok {
			return nil, fmt.Errorf("colors: unknown role %q (valid: %s)",
				k, joinRoles())
		}
		v := cfg[k]
		if !validSGR(v) {
			return nil, fmt.Errorf("colors: invalid value %q for %q (want raw SGR parameters, e.g. \"31\" or \"38;5;208\")", v, k)
		}
		p.seqs[role] = v
	}
	return p, nil
}

// validSGR accepts digit groups separated by semicolons: "31", "1;31",
// "38;5;208". Rejects empty strings, ESC/CSI wrappers, blanks, signs.
func validSGR(v string) bool {
	if v == "" {
		return false
	}
	for _, part := range strings.Split(v, ";") {
		if part == "" || strings.Trim(part, "0123456789") != "" {
			return false
		}
	}
	return true
}

func joinRoles() string {
	parts := make([]string, len(roles))
	for i, r := range roles {
		parts[i] = string(r)
	}
	return strings.Join(parts, ", ")
}

// Seq returns the raw SGR parameters for a role (digits only, no wrapper).
func (p *Palette) Seq(r Role) string {
	if seq, ok := p.seqs[r]; ok {
		return seq
	}
	return ""
}

// BoldSeq returns the bold variant of a role's sequence ("1;" prefix).
func (p *Palette) BoldSeq(r Role) string {
	seq := p.Seq(r)
	if seq == "" {
		return ""
	}
	if strings.HasPrefix(seq, "1;") {
		return seq
	}
	return "1;" + seq
}

// Wrap renders s in the role's hue (full escape sequence).
func (p *Palette) Wrap(r Role, s string) string {
	seq := p.Seq(r)
	if seq == "" {
		return s
	}
	return "\033[" + seq + "m" + s + "\033[0m"
}
