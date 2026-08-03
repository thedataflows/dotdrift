package tui

import (
	"fmt"
	"strings"
)

// KV is one row of a "key: value" review block. Muted true renders the
// value with valueMutedStyle (e.g. for "(registry preset)") so secondary
// content reads past real values.
type KV struct {
	Key   string
	Value string
	Muted bool
}

// renderKV lays out a key:value block with a single aligned column. The
// column width is the longest key (plus one space) so the colons line up
// regardless of which wizard produced the block. Keys use labelStyle
// (bold indigo), values use valueStyle (or valueMutedStyle when Muted).
//
// This replaces three hand-padded fmt.Fprintf chains in wizard_steps.go
// (13-char column), wizard_smb.go confirmShare (10-char), and
// wizard_smb.go confirmSmbSpec (9-char), which rendered the same kind of
// element three different ways.
func renderKV(pairs []KV) string {
	width := 0
	for _, p := range pairs {
		if len(p.Key) > width {
			width = len(p.Key)
		}
	}
	var b strings.Builder
	for _, p := range pairs {
		key := labelStyle.Render(fmt.Sprintf("%-*s", width, p.Key))
		var val string
		if p.Muted {
			val = valueMutedStyle.Render(p.Value)
		} else {
			val = valueStyle.Render(p.Value)
		}
		fmt.Fprintf(&b, "%s: %s\n", key, val)
	}
	return b.String()
}
