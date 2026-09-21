// Package tomlsplice splices sections of a module.toml text: it splits a
// document into its preamble and table sections, replaces the sections of
// one family (the top-level table path) with re-encoded text, and
// reassembles the document so every untouched byte — comments, position,
// formatting — passes through verbatim. It is text mechanics only: no
// schema knowledge, no decoding (0065-D7's layer 1; extracted and
// generalized from onboard's splicer).
package tomlsplice

import (
	"sort"
	"strconv"
	"strings"
)

// FamilyKeys is the preamble's splice family — the module-level keys
// (id, app, description, …) that live before any table header.
const FamilyKeys = "keys"

// Section is one top-level block of a TOML document: the preamble
// (Header "", Family FamilyKeys — where module-level keys live) or one
// "[...]" table with its body lines, exactly as they appear in the source.
type Section struct {
	Header string // the raw header line, untrimmed; "" for the preamble
	Family string // the header path's top-level key; FamilyKeys for the preamble
	Lines  []string
}

// Split parses text into sections in document order. The header grammar
// is line-based and multi-line-string aware: a line is a table header only
// when, outside any open multi-line string and any single-line string, it
// opens with "[", closes the bracket before any top-level "=", and
// carries nothing but more brackets, whitespace, or a comment afterwards.
// Quoted keys, inline tables, array rows, and the body of a """…""" or
// ”'…”' string are body lines, never headers.
func Split(text string) []Section {
	var secs []Section
	cur := Section{Family: FamilyKeys}
	st := lineState{}
	for _, line := range strings.Split(text, "\n") {
		if step(line, &st) {
			secs = append(secs, cur)
			cur = Section{Header: line, Family: headerFamily(line)}
		} else {
			cur.Lines = append(cur.Lines, line)
		}
	}
	secs = append(secs, cur)
	return secs
}

// Splice returns text with every section whose family is a key of
// replacements replaced by that key's block: the block is inserted where
// the family first appeared, later sections of the same family are
// dropped (both the [dotfiles] inline spelling and [dotfiles."…"]
// sub-table spelling belong to one family), and the block "" removes the
// family outright. A family the file does not have yet is appended, in
// sorted key order. Untouched sections — and the preamble, unless
// FamilyKeys is a replacement key — keep their exact bytes.
func Splice(text string, replacements map[string]string) string {
	secs := Split(text)
	out := make([]string, 0, len(strings.Split(text, "\n"))+8)
	emitted := map[string]bool{}

	addBlock := func(block string, sep bool) {
		lines := trimBlank(block)
		if len(lines) == 0 {
			return
		}
		if sep && len(out) > 0 && out[len(out)-1] != "" {
			out = append(out, "")
		}
		out = append(out, lines...)
	}

	for _, sec := range secs {
		block, replace := replacements[sec.Family]
		switch {
		case replace && !emitted[sec.Family]:
			emitted[sec.Family] = true
			addBlock(block, sec.Family != FamilyKeys && len(out) > 0)
		case replace:
			// A later section of an already-spliced family: dropped with
			// its body (its separator blank line dies with it).
		default:
			if sec.Header != "" {
				out = append(out, sec.Header)
			}
			out = append(out, sec.Lines...)
		}
	}

	// Append fresh families, sorted for determinism.
	var fresh []string
	for fam := range replacements {
		if !emitted[fam] {
			fresh = append(fresh, fam)
		}
	}
	sort.Strings(fresh)
	for _, fam := range fresh {
		addBlock(replacements[fam], true)
	}

	return joinLines(out, strings.HasSuffix(text, "\n"))
}

// The header grammar. One scanner walks each line exactly once — the same
// pass classifies the line and advances the multi-line string state, so
// classification can never double-count or lose string state.

type lineState struct {
	multi byte // 0, or the quote rune of the open """ / ''' string
}

// step advances the multi-line string state across one line and reports
// whether the line is a table header. Called exactly once per line, in
// document order.
func step(line string, st *lineState) (isHeader bool) {
	if st.multi != 0 {
		scanInMulti(line, st)
		return false // inside a """…""" string: always body
	}
	i, n := 0, len(line)
	inStr := byte(0)
	opened, closed, sawEq, tailClean := false, false, false, true
	for i < n {
		c := line[i]
		switch {
		case st.multi != 0:
			if st.multi == '"' && c == '\\' {
				i += 2
				continue
			}
			if strings.HasPrefix(line[i:], trice(st.multi)) {
				st.multi = 0
				i += 3
				continue
			}
			i++
		case inStr != 0:
			if inStr == '"' && c == '\\' {
				i += 2
				continue
			}
			if c == inStr {
				inStr = 0
			}
			i++
		case c == '#':
			return opened && closed && !sawEq && tailClean && st.multi == 0
		case c == '"' || c == '\'':
			if strings.HasPrefix(line[i:], trice(c)) {
				st.multi = c
				i += 3
				continue
			}
			inStr = c
			i++
		case c == '[':
			if !opened {
				opened = true
			}
			i++
		case c == ']':
			if opened && !closed {
				closed = true
			} else if closed && c == ']' {
				// [[array.tables]] — extra closing brackets stay legal.
			} else {
				tailClean = false
			}
			i++
		case c == '=':
			if !closed {
				sawEq = true
			}
			i++
		case c == ' ' || c == '\t':
			i++
		default:
			if closed {
				tailClean = false // content after the header bracket
			}
			i++
		}
	}
	return opened && closed && !sawEq && tailClean && st.multi == 0
}

// scanInMulti advances the state across a line inside an open multi-line
// string (single-line strings and headers cannot occur there).
func scanInMulti(line string, st *lineState) {
	i := 0
	for i < len(line) {
		c := line[i]
		if st.multi == '"' && c == '\\' {
			i += 2
			continue
		}
		if strings.HasPrefix(line[i:], trice(st.multi)) {
			st.multi = 0
			i += 3
			continue
		}
		i++
	}
}

func trice(q byte) string { return strings.Repeat(string(q), 3) }

// headerFamily extracts a header's top-level key: "[smb.shares.media]" →
// "smb"; quoted first keys unquote; trailing comments never reach here
// (the bracket closes first). The empty family never does either — the
// preamble has no header.
func headerFamily(header string) string {
	s := strings.TrimSpace(header)
	// Cut at the header's closing bracket (first top-level one).
	var quote byte
	end := -1
	for i := 0; i < len(s); i++ {
		c := s[i]
		switch {
		case quote != 0:
			if c == '\\' && quote == '"' {
				i++
			} else if c == quote {
				quote = 0
			}
		case c == '"' || c == '\'':
			quote = c
		case c == ']':
			end = i
		default:
		}
		if end >= 0 {
			break
		}
	}
	if end < 0 {
		end = len(s)
	}
	inner := strings.TrimPrefix(s[1:end], "[") // drop the outermost bracket(s)
	// Cut at the first top-level dot (quoted pieces may contain dots).
	quote = 0
	for i := 0; i < len(inner); i++ {
		c := inner[i]
		switch {
		case quote != 0:
			if c == '\\' && quote == '"' {
				i++
			} else if c == quote {
				quote = 0
			}
		case c == '"' || c == '\'':
			quote = c
		case c == '.':
			return unquoteKey(strings.TrimSpace(inner[:i]))
		}
	}
	return unquoteKey(strings.TrimSpace(inner))
}

// unquoteKey strips one layer of quoting from a key piece.
func unquoteKey(k string) string {
	if len(k) >= 2 && (k[0] == '"' && k[len(k)-1] == '"' || k[0] == '\'' && k[len(k)-1] == '\'') {
		inner := k[1 : len(k)-1]
		if k[0] == '\'' {
			return inner // literal strings have no escapes
		}
		if s, err := strconv.Unquote(`"` + inner + `"`); err == nil {
			return s
		}
		return inner
	}
	return k
}

// trimBlank drops a block's leading and trailing blank lines (the splice's
// own separators are added explicitly).
func trimBlank(block string) []string {
	lines := strings.Split(block, "\n")
	for len(lines) > 0 && strings.TrimSpace(lines[0]) == "" {
		lines = lines[1:]
	}
	for len(lines) > 0 && strings.TrimSpace(lines[len(lines)-1]) == "" {
		lines = lines[:len(lines)-1]
	}
	return lines
}

// joinLines reassembles output lines: every original blank run survives
// verbatim; the document ends in a newline exactly when it did before.
func joinLines(lines []string, hadFinalNewline bool) string {
	if !hadFinalNewline {
		for len(lines) > 0 && lines[len(lines)-1] == "" {
			lines = lines[:len(lines)-1]
		}
	}
	if len(lines) == 0 {
		return ""
	}
	return strings.Join(lines, "\n")
}
