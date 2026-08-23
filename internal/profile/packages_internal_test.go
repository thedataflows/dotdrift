package profile

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/thedataflows/dotdrift/internal/facts"
)

// swapProbe replaces both installed-state probe seams for one test and
// restores them afterwards. The fakes record every probed name and answer
// from the given sets, so tests observe exactly which packages/tools Load
// queried.
func swapProbe(t *testing.T, answer map[string]bool) *probeRecorder {
	t.Helper()
	rec := &probeRecorder{answer: answer}
	origPkg, origTool := probeInstalledPackages, probeInstalledTools
	probeInstalledPackages = func(backend string, names []string) map[string]bool {
		rec.backend = backend
		rec.pkgNames = append(rec.pkgNames, names...)
		return answer
	}
	probeInstalledTools = func(names []string) map[string]bool {
		rec.toolNames = append(rec.toolNames, names...)
		return answer
	}
	t.Cleanup(func() { probeInstalledPackages, probeInstalledTools = origPkg, origTool })
	return rec
}

type probeRecorder struct {
	answer    map[string]bool
	backend   string
	pkgNames  []string
	toolNames []string
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
