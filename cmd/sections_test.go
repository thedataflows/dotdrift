package cmd

import (
	"testing"

	"github.com/alecthomas/kong"
	"github.com/stretchr/testify/require"
)

// resolveSections truth table: positives form an allowlist, negatives
// subtract, no flags = everything, empty selection errors.

// mustSections builds a set or fails the test (valid names only).
func mustSections(t *testing.T, names ...string) sectionSet {
	t.Helper()
	set, err := newSectionSet(names...)
	require.NoError(t, err)
	return set
}

func TestResolveSections_noFlagsSelectsAll(t *testing.T) {
	got, err := resolveSections(nil)
	require.NoError(t, err)
	require.Equal(t, mustSections(t, "packages", "tools", "dotfiles", "systemd", "mounts", "smb", "hooks"), got)
}

func TestResolveSections_positiveOnly(t *testing.T) {
	got, err := resolveSections(map[string]bool{"packages": true})
	require.NoError(t, err)
	require.Equal(t, mustSections(t, "packages"), got)
}

func TestResolveSections_multiplePositives(t *testing.T) {
	got, err := resolveSections(map[string]bool{"packages": true, "tools": true})
	require.NoError(t, err)
	require.Equal(t, mustSections(t, "packages", "tools"), got)
}

func TestResolveSections_negatedSubtractsFromAll(t *testing.T) {
	got, err := resolveSections(map[string]bool{"hooks": false})
	require.NoError(t, err)
	require.Equal(t, mustSections(t, "packages", "tools", "dotfiles", "systemd", "mounts", "smb"), got)
}

func TestResolveSections_mixedPositiveAndNegative(t *testing.T) {
	// --packages --no-tools --no-smb: the allowlist is packages, the
	// negatives subtract from it (already absent here, but the mix must
	// not resurrect anything).
	got, err := resolveSections(map[string]bool{"packages": true, "tools": false, "smb": false})
	require.NoError(t, err)
	require.Equal(t, mustSections(t, "packages"), got)
}

func TestResolveSections_emptySelectionErrors(t *testing.T) {
	allNegated := map[string]bool{
		"packages": false, "tools": false, "dotfiles": false, "systemd": false,
		"mounts": false, "smb": false, "hooks": false,
	}
	_, err := resolveSections(allNegated)
	require.Error(t, err)
	require.Contains(t, err.Error(), "no sections selected")
	require.Contains(t, err.Error(), "packages", "error names the valid sections")
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
// state: AfterApply captures the context, and resolveSectionsFromKong
// turns Set flags into the explicit map. Absent flags must not appear.
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
		if !isSectionName(fl.Name) {
			continue
		}
		require.NotNil(t, fl.Group, "section flag --%s must be grouped", fl.Name)
		require.Equal(t, "Section flags", fl.Group.Key)
		grouped[fl.Name] = true
	}
	require.Len(t, grouped, len(sectionNames), "all six section flags carry the group")

	for _, fl := range cli.Apply.kctx.Flags() {
		if isSectionName(fl.Name) {
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
