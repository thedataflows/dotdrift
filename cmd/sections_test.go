package cmd

import (
	"slices"
	"testing"

	"github.com/alecthomas/kong"
	"github.com/stretchr/testify/require"
	"github.com/thedataflows/dotdrift/internal/service"
)

// Section-resolution semantics (positives-allowlist, negatives-subtract,
// empty-errors, unknown-errors) are pinned once, in the service layer
// (internal/service/sections_test.go) — the single source this flag layer
// feeds. The tests below cover only the flag layer's own concerns: kong
// parsing, flag-presence detection, the env kill-switch, and the
// programmatic override path.

// mustSections builds the expected selection (valid names only).
func mustSections(t *testing.T, names ...string) service.SectionSet {
	t.Helper()
	set := service.SectionSet{}
	for _, n := range names {
		require.Contains(t, service.SectionNames, n)
		set[n] = true
	}
	return set
}

// The section flags parse positive and negated onto ApplyCmd, including
// the legacy --no-hooks spelling.
func TestKong_applySectionFlags(t *testing.T) {
	var cli CLI
	parser, err := kong.New(&cli)
	require.NoError(t, err)
	_, err = parser.Parse([]string{"apply", "--packages", "--no-tools", "--no-smb"})
	require.NoError(t, err)
	require.True(t, cli.Apply.Packages)
	require.False(t, cli.Apply.Tools)
	require.False(t, cli.Apply.Smb)
}

func TestKong_applyNoHooksLegacySpelling(t *testing.T) {
	var cli CLI
	parser, err := kong.New(&cli)
	require.NoError(t, err)
	_, err = parser.Parse([]string{"apply", "--no-hooks"})
	require.NoError(t, err)
	require.False(t, cli.Apply.Hooks)
}

// Flag PRESENCE (positive vs negated vs absent) is read from kong's parse
// state: AfterApply captures the context, and ApplyCmd.resolveSections
// turns Set flags into the explicit map before the service resolves it.
// Absent flags must not appear.
func TestApplyCmd_resolveSectionsFromKong(t *testing.T) {
	var cli CLI
	parser, err := kong.New(&cli)
	require.NoError(t, err)
	_, err = parser.Parse([]string{"apply", "--packages", "--no-hooks"})
	require.NoError(t, err)

	got, err := cli.Apply.resolveSections()
	require.NoError(t, err)
	require.Equal(t, mustSections(t, "packages"), got)

	// No flags at all: everything runs.
	cli = CLI{}
	parser, err = kong.New(&cli)
	require.NoError(t, err)
	_, err = parser.Parse([]string{"apply"})
	require.NoError(t, err)
	got, err = cli.Apply.resolveSections()
	require.NoError(t, err)
	require.Equal(t, mustSections(t, "packages", "tools", "dotfiles", "systemd", "mounts", "smb", "hooks"), got)
}

// DOTDRIFT_NO_HOOKS=1 subtracts hooks exactly like --no-hooks, on top of
// whatever the flags selected.
func TestApplyCmd_noHooksEnvSubtracts(t *testing.T) {
	t.Setenv("DOTDRIFT_NO_HOOKS", "1")
	var cli CLI
	parser, err := kong.New(&cli)
	require.NoError(t, err)
	_, err = parser.Parse([]string{"apply", "--packages", "--tools"})
	require.NoError(t, err)
	got, err := cli.Apply.resolveSections()
	require.NoError(t, err)
	require.Equal(t, mustSections(t, "packages", "tools"), got)
}

// The six section flags render under one "Section flags" group in --help;
// the other apply flags stay ungrouped.
func TestKong_applySectionFlagsGrouped(t *testing.T) {
	var cli CLI
	parser, err := kong.New(&cli)
	require.NoError(t, err)
	_, err = parser.Parse([]string{"apply"})
	require.NoError(t, err)

	grouped := map[string]bool{}
	for _, fl := range cli.Apply.kctx.Flags() {
		if !slices.Contains(service.SectionNames, fl.Name) {
			continue
		}
		require.NotNil(t, fl.Group, "section flag --%s must be grouped", fl.Name)
		require.Equal(t, "Section flags", fl.Group.Key)
		grouped[fl.Name] = true
	}
	require.Len(t, grouped, len(service.SectionNames), "all six section flags carry the group")

	for _, fl := range cli.Apply.kctx.Flags() {
		if slices.Contains(service.SectionNames, fl.Name) {
			continue
		}
		require.Nil(t, fl.Group, "non-section flag --%s must stay ungrouped", fl.Name)
	}
}

// The programmatic override (tests, future callers) rejects unknown
// section names, listing the valid ones.
func TestApplyCmd_unknownOverrideSectionErrors(t *testing.T) {
	cmd := &ApplyCmd{onlySections: []string{"packages", "bogus"}}
	_, err := cmd.resolveSections()
	require.Error(t, err)
	require.Contains(t, err.Error(), "bogus")
	require.Contains(t, err.Error(), "packages", "error lists valid sections")
}
