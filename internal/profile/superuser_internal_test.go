package profile

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/thedataflows/dotdrift/internal/facts"
)

// superuserProfile builds a minimal profile: one base module plus one module
// per named user overlay under users/<name>/modules/<name>mod/.
func superuserProfile(t *testing.T, users ...string) string {
	t.Helper()
	root := t.TempDir()
	writeTestModule(t, filepath.Join(root, "modules", "base"), "base")
	for _, u := range users {
		writeTestModule(t, filepath.Join(root, "users", u, "modules", u+"mod"), u+"mod")
	}
	return root
}

func writeTestModule(t *testing.T, dir, id string) {
	t.Helper()
	require.NoError(t, os.MkdirAll(dir, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "module.toml"),
		[]byte("id = \""+id+"\"\n"), 0o644))
}

func stubUIDLookup(t *testing.T, uids map[string]string) {
	t.Helper()
	orig := lookupUID
	lookupUID = func(name string) (string, bool) {
		uid, ok := uids[name]
		return uid, ok
	}
	t.Cleanup(func() { lookupUID = orig })
}

func superuserSkips(p *Profile) []Skip {
	var out []Skip
	for _, s := range p.Skipped {
		if s.Reason == ReasonSuperuserOverlay {
			out = append(out, s)
		}
	}
	return out
}

func selectedIDsOf(p *Profile) []string {
	var out []string
	for _, m := range p.Selected {
		out = append(out, m.ID)
	}
	return out
}

func TestSuperuserOverlay_skippedWhenUnprivileged(t *testing.T) {
	stubUIDLookup(t, map[string]string{"root": "0"})
	p, err := Load(superuserProfile(t, "root"), &facts.Facts{Username: "cri"})
	require.NoError(t, err)

	skips := superuserSkips(p)
	require.Len(t, skips, 1)
	require.Equal(t, "rootmod", skips[0].Module.ID)
	require.Contains(t, skips[0].Reason, "sudo")
	require.NotContains(t, selectedIDsOf(p), "rootmod",
		"superuser overlay must never be selected by an unprivileged run")
}

func TestSuperuserOverlay_selectedWhenRunningAsRoot(t *testing.T) {
	stubUIDLookup(t, map[string]string{"root": "0"})
	p, err := Load(superuserProfile(t, "root"), &facts.Facts{Username: "root"})
	require.NoError(t, err)

	require.Contains(t, selectedIDsOf(p), "rootmod")
	require.Empty(t, superuserSkips(p),
		"running as the uid-0 account selects normally — no skip entries")
}

func TestSuperuserOverlay_otherUid0AccountAlsoSurfaced(t *testing.T) {
	stubUIDLookup(t, map[string]string{"toor": "0"})
	p, err := Load(superuserProfile(t, "toor"), &facts.Facts{Username: "cri"})
	require.NoError(t, err)

	require.Len(t, superuserSkips(p), 1,
		"uid-0 accounts named something other than root must surface too")
}

func TestSuperuserOverlay_nonSuperuserOwnerStaysInvisible(t *testing.T) {
	stubUIDLookup(t, map[string]string{"alice": "1000"})
	p, err := Load(superuserProfile(t, "alice"), &facts.Facts{Username: "cri"})
	require.NoError(t, err)

	require.Empty(t, superuserSkips(p),
		"another user's overlay is invisible by design, not reported")
}

func TestSuperuserOverlay_unknownAccountStaysInvisible(t *testing.T) {
	stubUIDLookup(t, map[string]string{}) // lookup fails for every name
	p, err := Load(superuserProfile(t, "ghost"), &facts.Facts{Username: "cri"})
	require.NoError(t, err)

	require.Empty(t, superuserSkips(p),
		"an overlay dir naming no OS account is not a superuser overlay")
}
