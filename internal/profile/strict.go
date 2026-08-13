package profile

import (
	"errors"
	"fmt"
	"os"
	"strings"

	"github.com/BurntSushi/toml"
)

// DecodeModuleTOMLFile strictly decodes a module.toml into cfg. It is
// DecodeModuleTOML over the file's contents; see its contract.
func DecodeModuleTOMLFile(path string, cfg *ModuleConfig) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	return DecodeModuleTOML(path, data, cfg)
}

// DecodeModuleTOML strictly decodes module.toml contents into cfg: any key
// not in the ModuleConfig schema is an error naming the file, line, and
// key — typos like `preset = [...]` under [packages] must fail loudly at
// load, never decode silently. path is used for error messages only. The
// schema is the ModuleConfig struct tree (toml tags);
// docs/product/profile-layout.md is its human-readable counterpart. Every
// production module.toml read goes through one of these two functions.
func DecodeModuleTOML(path string, data []byte, cfg *ModuleConfig) error {
	// Unknown keys inside [[hooks.pre]]/[[hooks.post]] never reach
	// Undecoded (the custom unmarshaler consumes the subtree), so scan the
	// table-array spelling up front for an exact line. The inline spelling
	// ({ command = "...", bogus = 1 }) is caught by HookCommand.UnmarshalTOML
	// via the decode error, without a line.
	if line, key := findUnknownHookKey(data); line > 0 {
		return fmt.Errorf("%s:%d: unknown key %q in hook entry (valid keys: command, optional)", path, line, key)
	}
	md, err := toml.Decode(string(data), cfg)
	if err != nil {
		return fmt.Errorf("decode %s: %w", path, err)
	}
	if undecoded := md.Undecoded(); len(undecoded) > 0 {
		return unknownKeysError(path, data, undecoded)
	}
	return nil
}

// findUnknownHookKey scans for [[hooks.pre]]/[[hooks.post]] tables and
// returns the 1-based line and name of the first key outside the
// {command, optional} schema. Zero when all hook tables are clean.
func findUnknownHookKey(src []byte) (int, string) {
	inHookTable := false
	for i, raw := range strings.Split(string(src), "\n") {
		line := strings.TrimSpace(raw)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		if strings.HasPrefix(line, "[") {
			pieces := splitKeyPieces(strings.Trim(line, "[]"))
			inHookTable = len(pieces) == 2 && pieces[0] == "hooks" && (pieces[1] == "pre" || pieces[1] == "post")
			continue
		}
		if !inHookTable {
			continue
		}
		pieces, ok := leadingKeyPieces(line)
		if !ok {
			continue
		}
		if len(pieces) != 1 || (pieces[0] != "command" && pieces[0] != "optional") {
			return i + 1, strings.Join(pieces, ".")
		}
	}
	return 0, ""
}

// unknownKeysError renders one "file:line: unknown key" line per undecoded
// key. The key message names the offending leaf key and its parent table
// path when nested (e.g. unknown key "preset" in [packages]).
func unknownKeysError(path string, src []byte, keys []toml.Key) error {
	msgs := make([]string, 0, len(keys))
	for _, key := range keys {
		pieces := []string(key)
		var msg string
		if len(pieces) == 1 {
			msg = fmt.Sprintf("unknown key %q", pieces[0])
		} else {
			parent := toml.Key(pieces[:len(pieces)-1]).String()
			msg = fmt.Sprintf("unknown key %q in [%s]", pieces[len(pieces)-1], parent)
		}
		if line := locateKeyLine(src, pieces); line > 0 {
			msgs = append(msgs, fmt.Sprintf("%s:%d: %s", path, line, msg))
		} else {
			// ponytail: inline-table unknown keys ({ bogus = 1 } on one line)
			// are reported without a line; the dotted-path scanner only tracks
			// table headers and leading key tokens. Upgrade path: full TOML AST.
			msgs = append(msgs, fmt.Sprintf("%s: %s", path, msg))
		}
	}
	return errors.New(strings.Join(msgs, "\n"))
}

// locateKeyLine scans TOML source for the 1-based line where the key path
// pieces are assigned, tracking [table] and [[array-table]] headers. Both
// the header itself (unknown table) and "key = value" lines match. Returns
// 0 when the key is not found (inline-table values, generated content).
func locateKeyLine(src []byte, want []string) int {
	wantStr := strings.Join(want, "\x00")
	var table []string
	for i, raw := range strings.Split(string(src), "\n") {
		line := strings.TrimSpace(raw)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		if strings.HasPrefix(line, "[") {
			table = splitKeyPieces(strings.Trim(line, "[]"))
			if strings.Join(table, "\x00") == wantStr {
				return i + 1
			}
			continue
		}
		pieces, ok := leadingKeyPieces(line)
		if !ok {
			continue
		}
		full := append(append([]string{}, table...), pieces...)
		if strings.Join(full, "\x00") == wantStr {
			return i + 1
		}
	}
	return 0
}

// leadingKeyPieces parses the dotted key before the top-level "=" of a
// "key = value" line (quotes respected). False when the line assigns nothing.
func leadingKeyPieces(line string) ([]string, bool) {
	var quote rune
	for i, r := range line {
		switch {
		case quote != 0:
			if r == quote {
				quote = 0
			}
		case r == '"' || r == '\'':
			quote = r
		case r == '=':
			return splitKeyPieces(strings.TrimSpace(line[:i])), true
		}
	}
	return nil, false
}

// splitKeyPieces splits a dotted TOML key into its unquoted pieces
// (`dotfiles."~/.zshrc"` → ["dotfiles", "~/.zshrc"]). Unquoted pieces are
// space-trimmed; quoted pieces keep their literal contents.
func splitKeyPieces(s string) []string {
	var pieces []string
	var cur strings.Builder
	var quote rune
	quoted := false
	flush := func() {
		piece := cur.String()
		if !quoted {
			piece = strings.TrimSpace(piece)
		}
		pieces = append(pieces, piece)
		cur.Reset()
		quoted = false
	}
	for _, r := range s {
		switch {
		case quote != 0:
			if r == quote {
				quote = 0
			} else {
				cur.WriteRune(r)
			}
		case r == '"' || r == '\'':
			quote = r
			quoted = true
		case r == '.':
			flush()
		default:
			cur.WriteRune(r)
		}
	}
	flush()
	return pieces
}
