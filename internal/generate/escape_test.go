package generate

import (
	"os/exec"
	"testing"

	"github.com/stretchr/testify/require"
)

// escapeVectors are ground-truth outputs captured from the real
// `systemd-escape --path` binary (systemd 258, CachyOS host).
//
// Known divergence: for a non-normalized path with an interior ".."
// component (e.g. "/a/../b") the real binary hard-fails; EscapePath
// instead normalizes it away ("/a/../b" -> "b") since it cannot
// return an error. That case is intentionally not in the table.
var escapeVectors = []struct {
	name  string
	input string
	want  string
}{
	{name: "root", input: "/", want: "-"},
	{name: "empty", input: "", want: "-"},
	{name: "double slash only", input: "//", want: "-"},
	{name: "plain mount", input: "/mnt/linux2", want: "mnt-linux2"},
	{name: "nested mount", input: "/mnt/win/c", want: "mnt-win-c"},
	{name: "space escaped", input: "/mnt/My Disk", want: `mnt-My\x20Disk`},
	{name: "colon allowed", input: "/mnt/disk:0", want: "mnt-disk:0"},
	{name: "duplicate slash collapsed", input: "/a//b", want: "a-b"},
	{name: "trailing slash dropped", input: "/a/b/", want: "a-b"},
	{name: "leading slashes dropped", input: "//a", want: "a"},
	{name: "utf8 escaped per byte", input: "/mnt/ünicode", want: `mnt-\xc3\xbcnicode`},
	{name: "utf8 multibyte", input: "/mnt/é/a", want: `mnt-\xc3\xa9-a`},
	{name: "percent escaped", input: "/mnt/100%", want: `mnt-100\x25`},
	{name: "underscore allowed", input: "/mnt/a_b", want: "mnt-a_b"},
	{name: "dot allowed", input: "/mnt/a.b", want: "mnt-a.b"},
	{name: "literal dash escaped", input: "/mnt/-leading", want: `mnt-\x2dleading`},
	{name: "uuid dash escaped", input: "/srv/f28e6d9a-uuid", want: `srv-f28e6d9a\x2duuid`},
	{name: "interior dash escaped", input: "/a/-b", want: `a-\x2db`},
	{name: "double dash escaped", input: "/a--b", want: `a\x2d\x2db`},
	{name: "bare dash component", input: "/-", want: `\x2d`},
	{name: "leading dot escaped", input: "/.hidden", want: `\x2ehidden`},
	{name: "dot after separator kept", input: "/a/.b", want: "a-.b"},
	{name: "dot component dropped", input: "/a/./b", want: "a-b"},
	{name: "trailing dot component dropped", input: "/a/.", want: "a"},
	{name: "root dot", input: "/.", want: "-"},
	{name: "leading dotdot dropped", input: "/../a", want: "a"},
	{name: "tilde escaped", input: "/a~b", want: `a\x7eb`},
	{name: "plus escaped", input: "/a+b", want: `a\x2bb`},
	{name: "comma escaped", input: "/a,b", want: `a\x2cb`},
	{name: "dot component mid path", input: "/a.b/c", want: "a.b-c"},
}

func TestEscape_vectors(t *testing.T) {
	for _, tt := range escapeVectors {
		t.Run(tt.name, func(t *testing.T) {
			got := EscapePath(tt.input)
			require.Equal(t, tt.want, got)
			t.Logf("EscapePath(%q) = %q", tt.input, got)
		})
	}
}

// TestEscape_matchesSystemdEscape re-verifies every vector against the
// live systemd-escape binary when available.
func TestEscape_matchesSystemdEscape(t *testing.T) {
	bin, err := exec.LookPath("systemd-escape")
	if err != nil {
		t.Skip("systemd-escape not on PATH")
	}
	for _, tt := range escapeVectors {
		t.Run(tt.name, func(t *testing.T) {
			out, err := exec.Command(bin, "--path", tt.input).Output()
			require.NoError(t, err)
			live := string(out[:len(out)-1]) // strip trailing newline
			require.Equal(t, live, EscapePath(tt.input),
				"EscapePath(%q) diverges from systemd-escape", tt.input)
		})
	}
}
