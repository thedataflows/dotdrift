package profile_test

import (
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/thedataflows/dotdrift/internal/facts"
	"github.com/thedataflows/dotdrift/internal/profile"
)

// Boolean expression combinators on [when]: fields AND by default (the
// historical behavior), or = [...] any-of, and = [...] all-of grouping,
// not = {...} negation — all recursive, arbitrarily deep.

// The motivating example: kernel >= 7 AND somepackage NOT installed.
func TestWhenCombinators_notWithLeaf(t *testing.T) {
	root := t.TempDir()
	writeModule(t, root, "modules/m", `id = "m"
[when]
kernel = ">= 7"
not = { packages = ["somepackage"] }
`)
	none := &facts.Facts{Kernel: "7.2.0", InstalledPackages: map[string]bool{}}

	p, err := profile.Load(root, none)
	require.NoError(t, err)
	require.Equal(t, []string{"m"}, selectedIDs(p), "kernel matches, package absent")

	p, err = profile.Load(root, &facts.Facts{Kernel: "7.2.0", InstalledPackages: map[string]bool{"somepackage": true}})
	require.NoError(t, err)
	require.Equal(t, []string{"m"}, skippedIDs(p), "negated package present")

	p, err = profile.Load(root, &facts.Facts{Kernel: "6.1.0", InstalledPackages: map[string]bool{}})
	require.NoError(t, err)
	require.Equal(t, []string{"m"}, skippedIDs(p), "kernel fails")
	require.Equal(t, "when filter", p.Skipped[0].Reason)
}

// or = [...] matches when any element matches.
func TestWhenCombinators_or(t *testing.T) {
	root := t.TempDir()
	writeModule(t, root, "modules/m", `id = "m"
[when]
or = [{ gpu = "nvidia" }, { packages = ["nvidia-driver"] }]
`)
	load := func(f *facts.Facts) *profile.Profile {
		p, err := profile.Load(root, f)
		require.NoError(t, err)
		return p
	}
	require.Equal(t, []string{"m"}, selectedIDs(load(&facts.Facts{GPU: "nvidia"})), "first branch")
	require.Equal(t, []string{"m"}, selectedIDs(load(&facts.Facts{InstalledPackages: map[string]bool{"nvidia-driver": true}})), "second branch")
	require.Equal(t, []string{"m"}, skippedIDs(load(&facts.Facts{GPU: "amd"})), "neither branch")
}

// and = [...] groups sub-expressions: (a or b) and (c or d).
func TestWhenCombinators_andOfOrGroups(t *testing.T) {
	root := t.TempDir()
	writeModule(t, root, "modules/m", `id = "m"
[when]
and = [
  { or = [{ os = ["arch"] }, { os = ["cachyos"] }] },
  { or = [{ gpu = "nvidia" }, { kernel = ">= 7" }] },
]
`)
	load := func(f *facts.Facts) []string {
		p, err := profile.Load(root, f)
		require.NoError(t, err)
		return selectedIDs(p)
	}
	require.Equal(t, []string{"m"}, load(&facts.Facts{OS: "arch", GPU: "nvidia"}), "first group via os, second via gpu")
	require.Equal(t, []string{"m"}, load(&facts.Facts{OS: "cachyos", Kernel: "7.2.0"}), "first group via os, second via kernel")
	require.Empty(t, load(&facts.Facts{OS: "ubuntu", GPU: "nvidia"}), "first group fails")
	require.Empty(t, load(&facts.Facts{OS: "arch", Kernel: "6.0.0"}), "second group fails")
}

// not composes with or inside: NOT(a or b) = neither a nor b.
func TestWhenCombinators_notOfOr(t *testing.T) {
	root := t.TempDir()
	writeModule(t, root, "modules/m", `id = "m"
[when]
not = { or = [{ gpu = "nvidia" }, { packages = ["nvidia-driver"] }] }
`)
	pkgs := func(names ...string) map[string]bool {
		m := map[string]bool{}
		for _, n := range names {
			m[n] = true
		}
		return m
	}
	load := func(f *facts.Facts) []string {
		p, err := profile.Load(root, f)
		require.NoError(t, err)
		return selectedIDs(p)
	}
	require.Equal(t, []string{"m"}, load(&facts.Facts{GPU: "amd", InstalledPackages: pkgs()}), "neither branch matches")
	require.Empty(t, load(&facts.Facts{GPU: "amd", InstalledPackages: pkgs("nvidia-driver")}), "or branch matches, negated")
	require.Empty(t, load(&facts.Facts{GPU: "nvidia", InstalledPackages: pkgs()}), "or branch matches, negated")
}

// Deep nesting: not of (or of (and of ...)) — three levels down.
func TestWhenCombinators_deepNesting(t *testing.T) {
	root := t.TempDir()
	writeModule(t, root, "modules/m", `id = "m"
[when]
not = { or = [
  { and = [{ gpu = "nvidia" }, { kernel = "< 7" }] },
  { packages = ["legacy-driver"] },
] }
`)
	load := func(f *facts.Facts) []string {
		p, err := profile.Load(root, f)
		require.NoError(t, err)
		return selectedIDs(p)
	}
	require.Equal(t, []string{"m"}, load(&facts.Facts{GPU: "nvidia", Kernel: "7.2.0"}), "and-group false (kernel), pkg absent")
	require.Empty(t, load(&facts.Facts{GPU: "nvidia", Kernel: "6.1.0"}), "and-group true, negated")
}

// The dotted TOML spellings ([when.not], [[when.or]]) decode to the same
// expression tree as the inline form.
func TestWhenCombinators_dottedSpellings(t *testing.T) {
	root := t.TempDir()
	writeModule(t, root, "modules/m", `id = "m"
[when]
kernel = ">= 7"

[when.not]
packages = ["somepackage"]
`)
	p, err := profile.Load(root, &facts.Facts{Kernel: "7.2.0", InstalledPackages: map[string]bool{}})
	require.NoError(t, err)
	require.Equal(t, []string{"m"}, selectedIDs(p))

	root = t.TempDir()
	writeModule(t, root, "modules/m", `id = "m"
[[when.or]]
gpu = "nvidia"

[[when.or]]
kernel = ">= 7"
`)
	p, err = profile.Load(root, &facts.Facts{Kernel: "6.1.0", GPU: "amd"})
	require.NoError(t, err)
	require.Equal(t, []string{"m"}, skippedIDs(p))
	p, err = profile.Load(root, &facts.Facts{GPU: "nvidia"})
	require.NoError(t, err)
	require.Equal(t, []string{"m"}, selectedIDs(p))
}

// not = {} negates nothing — a footgun that must fail loudly at load,
// naming the module.
func TestLoad_whenEmptyNotErrors(t *testing.T) {
	root := t.TempDir()
	writeModule(t, root, "modules/m", `id = "m"
[when]
not = {}
`)
	_, err := profile.Load(root, &facts.Facts{})
	require.Error(t, err)
	require.Contains(t, err.Error(), "m")
	require.Contains(t, err.Error(), "not")
	t.Logf("error: %v", err)
}

// or = [] can never match; and = [] carries no meaning — both are load
// errors naming the module.
func TestLoad_whenEmptyOrAndErrors(t *testing.T) {
	for _, key := range []string{"or", "and"} {
		root := t.TempDir()
		writeModule(t, root, "modules/m", "id = \"m\"\n[when]\n"+key+" = []\n")
		_, err := profile.Load(root, &facts.Facts{})
		require.Error(t, err, "%s = [] must fail loudly", key)
		require.Contains(t, err.Error(), "m")
		require.Contains(t, err.Error(), key)
		t.Logf("%s error: %v", key, err)
	}
}

// An empty or-element would always match (any-of with a vacuous true) —
// a silent always-select footgun, rejected at load.
func TestLoad_whenEmptyOrElementErrors(t *testing.T) {
	root := t.TempDir()
	writeModule(t, root, "modules/m", `id = "m"
[when]
or = [{ gpu = "nvidia" }, {}]
`)
	_, err := profile.Load(root, &facts.Facts{})
	require.Error(t, err)
	require.Contains(t, err.Error(), "m")
	t.Logf("error: %v", err)
}

// A malformed kernel constraint is a load error even deep inside the
// expression tree — same contract as the flat form.
func TestLoad_whenMalformedKernelInsideNotErrors(t *testing.T) {
	root := t.TempDir()
	writeModule(t, root, "modules/m", `id = "m"
[when]
not = { kernel = "~ 7" }
`)
	_, err := profile.Load(root, &facts.Facts{})
	require.Error(t, err)
	require.Contains(t, err.Error(), "m", "error must name the module")
	require.Contains(t, err.Error(), "~ 7", "error must carry the expression")
	t.Logf("error: %v", err)
}

// Nested when tables decode through LoadModuleConfig with the tree intact.
func TestLoadModuleTOML_whenNestedTree(t *testing.T) {
	root := t.TempDir()
	writeModule(t, root, "modules/m", `id = "m"
[when]
kernel = ">= 7"
not = { or = [{ packages = ["p"] }, { and = [{ tools = ["t"] }] }] }
`)
	cfg, err := profile.LoadModuleConfig(root + "/modules/m")
	require.NoError(t, err)
	require.Equal(t, ">= 7", cfg.When.Kernel)
	require.NotNil(t, cfg.When.Not)
	require.Len(t, cfg.When.Not.Or, 2)
	require.Equal(t, []string{"p"}, cfg.When.Not.Or[0].Packages)
	require.Len(t, cfg.When.Not.Or[1].And, 1)
	require.Equal(t, []string{"t"}, cfg.When.Not.Or[1].And[0].Tools)
}
