package service

import (
	"testing"

	"github.com/stretchr/testify/require"
)

// ResolveSections truth table at the service boundary (the flag layer's
// former package-local copy moved here in 0069; 0070 makes it the single
// source, including the unknown-name rejection).

func TestResolveSections_unknownNameErrors(t *testing.T) {
	// An unknown section name must fail loudly listing the valid ones —
	// a typo (onlySections override, programmatic callers) must never
	// silently select nothing (byte-parity with the deleted cmd copy).
	_, err := ResolveSections(map[string]bool{"packages": true, "bogus": true})
	require.Error(t, err)
	require.Contains(t, err.Error(), "bogus")
	require.Contains(t, err.Error(), "packages", "error lists the valid sections")
	require.Contains(t, err.Error(), "unknown section(s)")

	// Unknown negative names are rejected too: the name is still a typo.
	_, err = ResolveSections(map[string]bool{"nope": false})
	require.Error(t, err)
	require.Contains(t, err.Error(), "nope")
}
