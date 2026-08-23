package palette_test

import (
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/thedataflows/dotdrift/internal/palette"
)

// Defaults: every role wraps with its documented SGR sequence. Missing
// stays orange (the light-red default was reverted by request).
func TestDefaultPalette(t *testing.T) {
	p := palette.Default()
	require.Equal(t, "\033[38;5;208mx\033[0m", p.Wrap(palette.Missing, "x"), "missing stays orange")
	require.Equal(t, "\033[32mx\033[0m", p.Wrap(palette.OK, "x"))
	require.Equal(t, "\033[33mx\033[0m", p.Wrap(palette.Warn, "x"))
	require.Equal(t, "\033[31mx\033[0m", p.Wrap(palette.Error, "x"))
	require.Equal(t, "\033[35mx\033[0m", p.Wrap(palette.Orphan, "x"))
	require.Equal(t, "\033[90mx\033[0m", p.Wrap(palette.Dim, "x"))
}

// Seq returns the raw SGR parameters, BoldSeq the bold variant.
func TestSeqHelpers(t *testing.T) {
	p := palette.Default()
	require.Equal(t, "38;5;208", p.Seq(palette.Missing))
	require.Equal(t, "1;38;5;208", p.BoldSeq(palette.Missing), "bold shade prefixes 1;")
	p2, err := palette.FromConfig(map[string]string{"missing": "38;5;75"})
	require.NoError(t, err)
	require.Equal(t, "1;38;5;75", p2.BoldSeq(palette.Missing))
}

// FromConfig applies per-role overrides; unspecified roles keep defaults.
func TestFromConfig_overrides(t *testing.T) {
	p, err := palette.FromConfig(map[string]string{
		"missing": "38;5;75",
		"orphan":  "94",
	})
	require.NoError(t, err)
	require.Equal(t, "\033[38;5;75mx\033[0m", p.Wrap(palette.Missing, "x"))
	require.Equal(t, "\033[94mx\033[0m", p.Wrap(palette.Orphan, "x"))
	require.Equal(t, "\033[32mx\033[0m", p.Wrap(palette.OK, "x"), "unspecified role keeps its default")
}

// Unknown role names are errors listing the valid roles.
func TestFromConfig_unknownRole(t *testing.T) {
	_, err := palette.FromConfig(map[string]string{"misssing": "31"})
	require.Error(t, err)
	require.Contains(t, err.Error(), "misssing")
	require.Contains(t, err.Error(), "missing", "error lists valid roles")
}

// Malformed values are rejected: empty, ESC/CSI wrappers, non-SGR control
// bytes, anything that is not digits/semicolons.
func TestFromConfig_invalidValues(t *testing.T) {
	for _, v := range []string{"", "\033[31m", "[31m", "31m", "red", "31; ", "-1"} {
		_, err := palette.FromConfig(map[string]string{"missing": v})
		require.Error(t, err, "value %q must be rejected", v)
	}
}

// Combined SGR params pass (bold, 256-color, underline combos).
func TestFromConfig_validCompoundValues(t *testing.T) {
	for _, v := range []string{"31", "1;31", "38;5;208", "4;38;5;75", "91"} {
		_, err := palette.FromConfig(map[string]string{"missing": v})
		require.NoError(t, err, "value %q must be accepted", v)
	}
}

// Profile [colors] config flows into the palette with layer precedence:
// a host layer override beats base, base applies when host is silent.
func TestProfileColors_precedence(t *testing.T) {
	t.Skip("wired in profile tests; kept here as the documented contract pointer")
}
