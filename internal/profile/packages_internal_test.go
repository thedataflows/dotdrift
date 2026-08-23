package profile

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/thedataflows/dotdrift/internal/facts"
)

// swapProbe replaces all four installed-state probe seams for one test:
// per-name exact probes and the installed-list probes (regex support).
// The fakes record every probed name and answer from the given sets, so
// tests observe exactly which packages/tools Load queried and via which
// path.
func swapProbe(t *testing.T, answer map[string]bool) *probeRecorder {
	t.Helper()
	rec := &probeRecorder{answer: answer}
	origPkg, origTool := probeInstalledPackages, probeInstalledTools
	origPkgList, origToolList := probeInstalledPackageList, probeInstalledToolList
	probeInstalledPackages = func(backend string, names []string) map[string]bool {
		rec.backend = backend
		rec.pkgNames = append(rec.pkgNames, names...)
		rec.pkgExactCalls++
		return answer
	}
	probeInstalledTools = func(names []string) map[string]bool {
		rec.toolNames = append(rec.toolNames, names...)
		rec.toolExactCalls++
		return answer
	}
	probeInstalledPackageList = func(backend string) map[string]bool {
		rec.backend = backend
		rec.pkgListCalls++
		return rec.pkgListAnswer
	}
	probeInstalledToolList = func() map[string]bool {
		rec.toolListCalls++
		return rec.toolListAnswer
	}
	t.Cleanup(func() {
		probeInstalledPackages, probeInstalledTools = origPkg, origTool
		probeInstalledPackageList, probeInstalledToolList = origPkgList, origToolList
	})
	return rec
}

type probeRecorder struct {
	answer        map[string]bool
	backend       string
	pkgNames      []string
	toolNames     []string
	pkgExactCalls int
	toolExactCalls int
	pkgListCalls  int
	toolListCalls int
	pkgListAnswer map[string]bool
	toolListAnswer map[string]bool
}

// when.packages and when.tools are the only triggers for probing: Load asks
// each seam exactly once for the deduplicated union of referenced names,
// and the answer drives selection.
func TestLoad_probesInstalledPackagesForWhenPackages(t *testing.T) {
	rec := swapProbe(t, map[string]bool{"installed-pkg": true})
	p, err := Load("../../testdata/profiles/whenfilter", &facts.Facts{Backend: "paru"})
	require.NoError(t, err)
	require.Equal(t, "paru", rec.backend)
	require.Equal(t, []string{"installed-pkg"}, rec.pkgNames)
	require.Contains(t, selectedIDsInternal(p), "pkgonly")
}

// when.tools triggers the tool probe (presence only — mise-managed tools).
// The whenfilter fixture also declares when.packages (pkgonly), so both
// seams run; each records only its own names.
func TestLoad_probesInstalledToolsForWhenTools(t *testing.T) {
	rec := swapProbe(t, map[string]bool{"installed-tool": true})
	p, err := Load("../../testdata/profiles/whenfilter", &facts.Facts{})
	require.NoError(t, err)
	require.Equal(t, []string{"installed-tool"}, rec.toolNames)
	require.Equal(t, []string{"installed-pkg"}, rec.pkgNames, "packages seam stays separate")
	require.Contains(t, selectedIDsInternal(p), "toolonly")
}

// A profile declaring no when.packages/when.tools probes nothing — existing
// profiles keep zero extra subprocess cost.
func TestLoad_probesNothingWithoutWhenPackages(t *testing.T) {
	rec := swapProbe(t, map[string]bool{"installed-pkg": true})
	_, err := Load("../../testdata/profiles/simple", &facts.Facts{Backend: "paru"})
	require.NoError(t, err)
	require.Empty(t, rec.pkgNames)
	require.Empty(t, rec.toolNames)
}

// Explicitly provided InstalledPackages/InstalledTools facts win: Load
// never overwrites them with a probe.
func TestLoad_explicitInstalledFactsWin(t *testing.T) {
	rec := swapProbe(t, map[string]bool{"installed-pkg": true, "installed-tool": true})
	p, err := Load("../../testdata/profiles/whenfilter", &facts.Facts{
		InstalledPackages: map[string]bool{"installed-pkg": false},
		InstalledTools:    map[string]bool{"installed-tool": false},
	})
	require.NoError(t, err)
	require.Empty(t, rec.pkgNames, "no probe when facts are provided")
	require.Empty(t, rec.toolNames)
	require.Contains(t, skippedIDsInternal(p), "pkgonly")
	require.Contains(t, skippedIDsInternal(p), "toolonly")
}

// Duplicate names across modules collapse into one probe per seam.
func TestLoad_probesDeduplicatedNames(t *testing.T) {
	root := t.TempDir()
	writeModuleInternal(t, root, "modules/a", "[when]\npackages = [\"vim\"]\n")
	writeModuleInternal(t, root, "modules/b", "[when]\npackages = [\"vim\", \"git\"]\n")
	rec := swapProbe(t, map[string]bool{"vim": true, "git": true})
	p, err := Load(root, &facts.Facts{})
	require.NoError(t, err)
	require.ElementsMatch(t, []string{"vim", "git"}, rec.pkgNames)
	require.Len(t, rec.pkgNames, 2)
	require.Len(t, p.Selected, 2)
}

// when.packages/when.tools leaves anywhere in the expression tree
// (inside or/and/not) are collected for probing like top-level leaves.
// Only p2 answers installed: the not-group's branches are false and the
// top-level or matches, so the module selects.
func TestLoad_probesNestedWhenLeaves(t *testing.T) {
	root := t.TempDir()
	writeModuleInternal(t, root, "modules/m", `id = "m"
[when]
not = { or = [{ packages = ["p1"] }, { and = [{ tools = ["t1"] }] }] }
or = [{ packages = ["p2"] }]
`)
	rec := swapProbe(t, map[string]bool{"p2": true})
	p, err := Load(root, &facts.Facts{})
	require.NoError(t, err)
	require.ElementsMatch(t, []string{"p1", "p2"}, rec.pkgNames)
	require.Equal(t, []string{"t1"}, rec.toolNames)
	require.Len(t, p.Selected, 1)
}

// A regex entry (regex metacharacters present) switches the probe from
// per-name exact queries to ONE installed-list query — a regex cannot be
// answered by IsInstalled(name). Plain-only profiles keep the exact path.
func TestLoad_regexEntriesUseListProbe(t *testing.T) {
	t.Run("packages regex triggers list, not exact", func(t *testing.T) {
		root := t.TempDir()
		writeModuleInternal(t, root, "modules/m", `[when]
packages = ["apollo.*"]
`)
		rec := swapProbe(t, nil)
		rec.pkgListAnswer = map[string]bool{"apollo-cuda-git": true}
		p, err := Load(root, &facts.Facts{Backend: "paru"})
		require.NoError(t, err)
		require.Equal(t, 1, rec.pkgListCalls)
		require.Zero(t, rec.pkgExactCalls)
		require.Equal(t, "paru", rec.backend)
		require.Len(t, p.Selected, 1)
	})

	t.Run("plain-only keeps exact probes", func(t *testing.T) {
		root := t.TempDir()
		writeModuleInternal(t, root, "modules/m", `[when]
packages = ["vim"]
`)
		rec := swapProbe(t, map[string]bool{"vim": true})
		_, err := Load(root, &facts.Facts{Backend: "paru"})
		require.NoError(t, err)
		require.Zero(t, rec.pkgListCalls)
		require.Equal(t, 1, rec.pkgExactCalls)
	})

	t.Run("mixed: one list query answers plain and regex entries", func(t *testing.T) {
		root := t.TempDir()
		writeModuleInternal(t, root, "modules/m", `[when]
packages = ["vim", "apollo.*"]
`)
		rec := swapProbe(t, nil)
		rec.pkgListAnswer = map[string]bool{"vim": true, "apollo": true}
		p, err := Load(root, &facts.Facts{})
		require.NoError(t, err)
		require.Equal(t, 1, rec.pkgListCalls, "list covers plain entries too")
		require.Zero(t, rec.pkgExactCalls, "no per-name queries when the list answered")
		require.Len(t, p.Selected, 1)
	})

	t.Run("list failure falls back to exact probes for plain entries", func(t *testing.T) {
		root := t.TempDir()
		writeModuleInternal(t, root, "modules/m", `[when]
packages = ["vim", "apollo.*"]
`)
		rec := swapProbe(t, map[string]bool{"vim": true})
		rec.pkgListAnswer = nil // e.g. unsupported backend: regex fails open
		p, err := Load(root, &facts.Facts{Backend: "unknown"})
		require.NoError(t, err)
		require.Equal(t, 1, rec.pkgListCalls)
		require.Equal(t, 1, rec.pkgExactCalls, "plain entries still exact-probed")
		require.Empty(t, p.Selected, "regex entry unanswered -> fail open")
	})

	t.Run("tools regex triggers tool list", func(t *testing.T) {
		root := t.TempDir()
		writeModuleInternal(t, root, "modules/m", `[when]
tools = ["go.*"]
`)
		rec := swapProbe(t, nil)
		rec.toolListAnswer = map[string]bool{"golangci-lint": true}
		p, err := Load(root, &facts.Facts{})
		require.NoError(t, err)
		require.Equal(t, 1, rec.toolListCalls)
		require.Zero(t, rec.toolExactCalls)
		require.Len(t, p.Selected, 1)
	})
}

func selectedIDsInternal(p *Profile) []string {
	ids := make([]string, len(p.Selected))
	for i, m := range p.Selected {
		ids[i] = m.ID
	}
	return ids
}

func skippedIDsInternal(p *Profile) []string {
	ids := make([]string, len(p.Skipped))
	for i, s := range p.Skipped {
		ids[i] = s.Module.ID
	}
	return ids
}

func writeModuleInternal(t *testing.T, root, relPath, tomlContent string) {
	t.Helper()
	dir := filepath.Join(root, relPath)
	require.NoError(t, os.MkdirAll(dir, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "module.toml"), []byte(tomlContent), 0o644))
}
