package profile_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/thedataflows/dotdrift/internal/facts"
	"github.com/thedataflows/dotdrift/internal/profile"
)

// misplacedProfile builds a profile with a proper base module, a proper
// root-overlay module, and two misplaced module dirs (module.toml directly
// under the account/host dir, missing the modules/ level), plus a module-less
// dir that must NOT be flagged.
func misplacedProfile(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	write := func(rel, content string) {
		p := filepath.Join(root, rel)
		require.NoError(t, os.MkdirAll(filepath.Dir(p), 0o755))
		require.NoError(t, os.WriteFile(p, []byte(content), 0o644))
	}
	write("modules/base/module.toml", "id = \"base\"\n")
	write("users/root/modules/ok/module.toml", "id = \"ok\"\n")
	write("users/root/loose/module.toml", "id = \"loose\"\n")
	write("hosts/h/looseh/module.toml", "id = \"looseh\"\n")
	require.NoError(t, os.MkdirAll(filepath.Join(root, "users", "root", "notamodule"), 0o755))
	return root
}

func misplacedSkipReasons(p *profile.Profile) []string {
	var out []string
	for _, s := range p.Skipped {
		if s.Reason != "" && len(s.Reason) >= len(profile.ReasonMisplacedModule) &&
			s.Reason[:len(profile.ReasonMisplacedModule)] == profile.ReasonMisplacedModule {
			out = append(out, s.Module.ID+": "+s.Reason)
		}
	}
	return out
}

func TestLoad_misplacedModuleDirsSkipped(t *testing.T) {
	p, err := profile.Load(misplacedProfile(t), &facts.Facts{Username: "cri", Hostname: "myhost"})
	require.NoError(t, err)

	reasons := misplacedSkipReasons(p)
	require.Len(t, reasons, 2)
	require.Contains(t, reasons[0]+reasons[1], "loose")
	require.Contains(t, reasons[0]+reasons[1], "users/root/modules/loose",
		"the reason names the expected path")
	require.Contains(t, reasons[0]+reasons[1], "looseh")
	require.Contains(t, reasons[0]+reasons[1], "hosts/h/modules/looseh")

	var selected []string
	for _, m := range p.Selected {
		selected = append(selected, m.ID)
	}
	require.NotContains(t, selected, "loose", "misplaced modules never select")
	require.NotContains(t, selected, "looseh")
	require.NotContains(t, selected, "notamodule", "a dir without module.toml is not a module at all")
}
