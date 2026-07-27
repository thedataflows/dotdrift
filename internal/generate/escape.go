// Package generate renders systemd units from dotdrift configuration.
package generate

import (
	"fmt"
	"strings"
)

// EscapePath mirrors `systemd-escape --path` for mount-unit naming.
//
// Behavior (verified against systemd 258):
//   - The path is normalized: empty and "." components are dropped,
//     ".." pops the previous component (or is dropped at the root).
//   - The resulting components are joined with "-".
//   - An empty result (e.g. "/" or "") yields "-".
//   - Bytes outside [A-Za-z0-9:_.] are escaped as \xNN (UTF-8 bytes);
//     a literal "-" becomes \x2d, and a "." as the very first
//     character of the result is escaped as \x2e.
//
// Unlike the real binary, EscapePath cannot fail: a non-normalized
// path such as "/a/../b" (which systemd-escape rejects) is normalized
// to "b".
func EscapePath(s string) string {
	var comps []string
	for _, p := range strings.Split(s, "/") {
		switch p {
		case "", ".":
			// dropped: collapses duplicate/leading/trailing slashes
		case "..":
			if len(comps) > 0 {
				comps = comps[:len(comps)-1]
			}
		default:
			comps = append(comps, p)
		}
	}
	if len(comps) == 0 {
		return "-"
	}

	var b strings.Builder
	for i, c := range comps {
		if i > 0 {
			b.WriteByte('-')
		}
		for j := 0; j < len(c); j++ {
			ch := c[j]
			switch {
			case ch >= 'a' && ch <= 'z',
				ch >= 'A' && ch <= 'Z',
				ch >= '0' && ch <= '9',
				ch == ':', ch == '_',
				ch == '.' && (i != 0 || j != 0):
				b.WriteByte(ch)
			default:
				fmt.Fprintf(&b, `\x%02x`, ch)
			}
		}
	}
	return b.String()
}
