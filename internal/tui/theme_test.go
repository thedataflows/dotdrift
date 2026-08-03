package tui

import (
	"bytes"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

// renderKV aligns values to the longest key, padding shorter keys with
// spaces so all colons line up — the three review screens previously used
// three different hand-tuned column widths for the same kind of block.
func TestRenderKV_alignsToLongestKey(t *testing.T) {
	out := renderKV([]KV{
		{Key: "name", Value: "media"},
		{Key: "destination", Value: "/mnt/media"},
		{Key: "type", Value: "nfs"},
	})
	// Strip any ANSI styling before checking alignment.
	plain := stripANSI(out)
	for _, line := range strings.Split(strings.TrimRight(plain, "\n"), "\n") {
		colon := strings.IndexByte(line, ':')
		require.Equal(t, 11, colon, "every key column must end at the longest key (destination=11): %q", line)
	}
}

// renderKV renders muted (parenthesized) values dimmed so "(registry
// preset)" reads as secondary content next to real values.
func TestRenderKV_mutedValue(t *testing.T) {
	out := renderKV([]KV{{Key: "options", Value: "(registry preset)", Muted: true}})
	plain := stripANSI(out)
	require.Contains(t, plain, "(registry preset)")
}

// warnf / errorf / infof prefix the message with a colored ASCII label and
// write to stderr. The body stays plain so the line is greppable; the
// prefix carries the color so the severity reads at a glance.
func TestNotices_prefixAndStream(t *testing.T) {
	var stderr bytes.Buffer
	warnf(&stderr, "no volumes detected; falling back to a manual source input")
	errorf(&stderr, "detecting volumes failed: %v", "no lsblk")
	infof(&stderr, "wrote %d files", 3)

	plain := stripANSI(stderr.String())
	require.Contains(t, plain, "warning: no volumes detected;")
	require.Contains(t, plain, "error: detecting volumes failed: no lsblk")
	require.Contains(t, plain, "info: wrote 3 files")
}

// stripANSI removes SGR escape sequences so tests can assert on the plain
// text content. We use a tiny local regex instead of importing a helper —
// the test scope is the presence/absence of color codes, not their value.
func stripANSI(s string) string {
	var b strings.Builder
	inEscape := false
	for _, r := range s {
		switch {
		case r == '\x1b':
			inEscape = true
		case inEscape && r == 'm':
			inEscape = false
		case !inEscape:
			b.WriteRune(r)
		}
	}
	return b.String()
}
