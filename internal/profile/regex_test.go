package profile_test

import (
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/thedataflows/dotdrift/internal/facts"
	"github.com/thedataflows/dotdrift/internal/profile"
)

// Regex entries in when.packages/when.tools: "apollo.*" must match both
// "apollo" and "apollo-cuda-git" (anchored full-name match). Plain entries
// keep exact semantics.

// The motivating example from the request.
func TestWhenRegex_packages(t *testing.T) {
	root := t.TempDir()
	writeModule(t, root, "modules/m", `id = "m"
[when]
packages = ["apollo.*"]
`)
	load := func(installed ...string) []string {
		m := map[string]bool{}
		for _, n := range installed {
			m[n] = true
		}
		p, err := profile.Load(root, &facts.Facts{InstalledPackages: m})
		require.NoError(t, err)
		return selectedIDs(p)
	}
	require.Equal(t, []string{"m"}, load("apollo"), "regex .* matches empty suffix")
	require.Equal(t, []string{"m"}, load("apollo-cuda-git"), "regex matches suffix")
	require.Equal(t, []string{"m"}, load("apollox"), "any suffix matches")
	require.Empty(t, load("xapollo"), "anchored: no partial/prefix match")
	require.Empty(t, load("other-pkg"))
}

// Anchoring is full-name: "apollo." (a literal dot) matches only "apollo."
// spellings, not "apollo2".
func TestWhenRegex_anchoredFullMatch(t *testing.T) {
	root := t.TempDir()
	writeModule(t, root, "modules/m", `id = "m"
[when]
packages = ["apollo.+"]
`)
	load := func(installed ...string) []string {
		m := map[string]bool{}
		for _, n := range installed {
			m[n] = true
		}
		p, err := profile.Load(root, &facts.Facts{InstalledPackages: m})
		require.NoError(t, err)
		return selectedIDs(p)
	}
	require.Empty(t, load("apollo"), ".+ needs a non-empty suffix")
	require.Equal(t, []string{"m"}, load("apollo-cuda-git"))
}

// Regex entries mix with plain entries and combinators.
func TestWhenRegex_mixedAndNested(t *testing.T) {
	root := t.TempDir()
	writeModule(t, root, "modules/m", `id = "m"
[when]
packages = ["vim", "apollo.*"]
not = { tools = ["cuda.*"] }
`)
	pkgs := func(names ...string) map[string]bool {
		m := map[string]bool{}
		for _, n := range names {
			m[n] = true
		}
		return m
	}
	tools := func(names ...string) map[string]bool {
		m := map[string]bool{}
		for _, n := range names {
			m[n] = true
		}
		return m
	}
	p, err := profile.Load(root, &facts.Facts{InstalledPackages: pkgs("vim", "apollo-cuda-git"), InstalledTools: tools()})
	require.NoError(t, err)
	require.Equal(t, []string{"m"}, selectedIDs(p))

	// Plain entry missing -> whole AND fails.
	p, err = profile.Load(root, &facts.Facts{InstalledPackages: pkgs("apollo-cuda-git"), InstalledTools: tools()})
	require.NoError(t, err)
	require.Empty(t, selectedIDs(p))

	// not = { tools = ["cuda.*"] } negated by cuda-toolkit.
	p, err = profile.Load(root, &facts.Facts{InstalledPackages: pkgs("vim", "apollo"), InstalledTools: tools("cuda-toolkit")})
	require.NoError(t, err)
	require.Empty(t, selectedIDs(p))
}

// An installed name exactly equal to the entry always wins — the entry is
// found by lookup, never re-interpreted as a pattern first. Regex-special
// names that are INVALID regexes (c++, g++) are load errors: write them
// escaped (g\+\+), which anchored-matches the literal name.
func TestWhenRegex_exactHitWins(t *testing.T) {
	root := t.TempDir()
	writeModule(t, root, "modules/m", `id = "m"
[when]
packages = ["apollo.*"]
`)
	// A package literally named "apollo.*": exact lookup wins.
	p, err := profile.Load(root, &facts.Facts{InstalledPackages: map[string]bool{"apollo.*": true}})
	require.NoError(t, err)
	require.Equal(t, []string{"m"}, selectedIDs(p), "exact name hit, no regex reading")
}

func TestWhenRegex_escapedLiteralSpecials(t *testing.T) {
	root := t.TempDir()
	writeModule(t, root, "modules/m", `id = "m"
[when]
packages = ["g\\+\\+"]
`)
	p, err := profile.Load(root, &facts.Facts{InstalledPackages: map[string]bool{"g++": true}})
	require.NoError(t, err)
	require.Equal(t, []string{"m"}, selectedIDs(p), "escaped entry matches the literal name")

	p, err = profile.Load(root, &facts.Facts{InstalledPackages: map[string]bool{"g": true}})
	require.NoError(t, err)
	require.Empty(t, selectedIDs(p), "escaped plus is literal, not repetition")
}

// Tools support regex too: "go.*" matches "go" and "golangci-lint".
func TestWhenRegex_tools(t *testing.T) {
	root := t.TempDir()
	writeModule(t, root, "modules/m", `id = "m"
[when]
tools = ["go.*"]
`)
	p, err := profile.Load(root, &facts.Facts{InstalledTools: map[string]bool{"golangci-lint": true}})
	require.NoError(t, err)
	require.Equal(t, []string{"m"}, selectedIDs(p))

	p, err = profile.Load(root, &facts.Facts{InstalledTools: map[string]bool{"python": true}})
	require.NoError(t, err)
	require.Empty(t, selectedIDs(p))
}

// A syntactically invalid regex is a load-time error naming the module and
// carrying the entry — never a silent never-match. Applies at any depth.
func TestLoad_whenInvalidRegexErrors(t *testing.T) {
	for _, tc := range []struct {
		name string
		toml string
		key  string
		entry string
	}{
		{"packages", `[when]
packages = ["apollo["]
`, "packages", "apollo["},
		{"tools", `[when]
tools = ["(unclosed"]
`, "tools", "(unclosed"},
		{"nested in not", `[when]
not = { packages = ["apollo["] }
`, "packages", "apollo["},
	} {
		t.Run(tc.name, func(t *testing.T) {
			root := t.TempDir()
			writeModule(t, root, "modules/m", "id = \"m\"\n"+tc.toml)
			_, err := profile.Load(root, &facts.Facts{})
			require.Error(t, err)
			require.Contains(t, err.Error(), "m", "error must name the module")
			require.Contains(t, err.Error(), tc.key)
			require.Contains(t, err.Error(), tc.entry, "error must carry the entry")
			t.Logf("error: %v", err)
		})
	}
}
