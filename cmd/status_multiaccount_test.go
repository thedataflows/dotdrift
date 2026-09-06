package cmd

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/thedataflows/dotdrift/internal/facts"
	"github.com/thedataflows/dotdrift/internal/profile"
)

// multiAccountProfile writes a temp profile with one base module (package
// demo-pkg) and one users/root overlay module (package root-pkg), so shared
// and per-account drift are distinguishable in the report.
func multiAccountProfile(t *testing.T) string {
	t.Helper()
	dir := statusMinimalProfile(t) // modules/demo → demo-pkg
	modDir := filepath.Join(dir, "users", "root", "modules", "rootsvc")
	require.NoError(t, os.MkdirAll(modDir, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(modDir, "module.toml"), []byte(`id = "rootsvc"

[packages]
present = ["root-pkg"]
`), 0o644))
	return dir
}

func stubAccountSeams(t *testing.T, accts []profile.Account, runErr error) {
	t.Helper()
	origOA, origRA := otherAccounts, runAsAccount
	t.Cleanup(func() { otherAccounts, runAsAccount = origOA, origRA })
	otherAccounts = func(string, *facts.Facts) ([]profile.Account, error) { return accts, nil }
	runAsAccount = func(string, ...string) ([]byte, error) { return nil, runErr }
}

func TestStatus_otherAccountSectionDedupAndNote(t *testing.T) {
	f := &facts.Facts{Hostname: "myhost", Username: "cri", OS: "linux", Backend: "paru"}
	// recordingBackend.IsInstalled reports everything absent → drift in both views.
	stubStatusDeps(t, f, &recordingBackend{events: &[]string{}}, fakeMiseNoOp)
	// root exists as an "account"; every sudo-as-account call fails (no
	// credentials): the install note must print, and status still exits 0.
	stubAccountSeams(t, []profile.Account{{Name: "root", Home: "/root"}},
		errors.New("sudo: a terminal is required"))
	dir := multiAccountProfile(t)

	var buf bytes.Buffer
	require.NoError(t, (&StatusCmd{Profile: dir, State: filepath.Join(t.TempDir(), "state.json"), out: &buf}).Run())
	out := buf.String()
	t.Log(out)

	require.Contains(t, out, "users/root:")
	require.Contains(t, out, "root-pkg", "the other account's own drift must surface")
	require.Equal(t, 1, strings.Count(out, "demo-pkg"),
		"shared-layer drift must not repeat under the account section")
	require.Contains(t, out, "system-wide",
		"an account that cannot run dotdrift gets the install recommendation")
}

func TestStatus_otherAccountToolsUnknownWithoutCreds(t *testing.T) {
	f := &facts.Facts{Hostname: "myhost", Username: "cri", OS: "linux", Backend: "paru"}
	stubStatusDeps(t, f, &recordingBackend{events: &[]string{}}, fakeMiseNoOp)
	stubAccountSeams(t, []profile.Account{{Name: "root", Home: "/root"}},
		errors.New("sudo: a terminal is required"))
	dir := multiAccountProfile(t)
	// root's overlay also declares a tool; with every sudo call failing it
	// must report unknown, never fail the run.
	modToml := filepath.Join(dir, "users", "root", "modules", "rootsvc", "module.toml")
	b, err := os.ReadFile(modToml)
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(modToml, append(b, []byte("\n[tools]\ngo = \"1.24\"\n")...), 0o644))

	var buf bytes.Buffer
	require.NoError(t, (&StatusCmd{Profile: dir, State: filepath.Join(t.TempDir(), "state.json"), out: &buf}).Run())
	out := buf.String()
	t.Log(out)

	require.Contains(t, out, "users/root:")
	require.Contains(t, out, "go")
	require.Contains(t, out, "(?)", "unprobeable per-account tool reports unknown")
}
