package cmd

import (
	"bytes"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/thedataflows/dotdrift/internal/facts"
	"github.com/thedataflows/dotdrift/internal/profile"
)

// Multi-account status (issue 0038, ADR-0006): no per-account drift sections,
// no probing of other accounts — just a notice naming each existing account
// with configuration and its apply command.

func stubOtherAccounts(t *testing.T, accts []profile.Account) {
	t.Helper()
	orig := otherAccounts
	t.Cleanup(func() { otherAccounts = orig })
	otherAccounts = func(string, *facts.Facts) ([]profile.Account, error) { return accts, nil }
}

func TestStatus_otherAccountsNotice(t *testing.T) {
	f := &facts.Facts{Hostname: "myhost", Username: "cri", OS: "linux", Backend: "paru"}
	stubStatusDeps(t, f, allInstalledBackend{}, fakeMiseNoOp)
	stubOtherAccounts(t, []profile.Account{
		{Name: "alice", Uid: "1000", Home: "/home/alice"},
		{Name: "root", Uid: "0", Home: "/root"},
	})
	dir := statusMinimalProfile(t)

	var buf bytes.Buffer
	require.NoError(t, (&StatusCmd{Profile: dir, State: filepath.Join(t.TempDir(), "state.json"), out: &buf}).Run())
	out := buf.String()
	t.Log(out)

	require.Contains(t, out, "configuration exists for other accounts")
	require.Contains(t, out, "users/root")
	require.Contains(t, out, "apply with: sudo dotdrift apply",
		"the uid-0 account gets the plain sudo form")
	require.Contains(t, out, "users/alice")
	require.Contains(t, out, "apply with: sudo -iu alice dotdrift apply",
		"other accounts get a login-shell sudo so their own PATH applies")
	require.NotContains(t, out, "users/root:\n", "no per-account drift sections anymore")
	require.NotContains(t, out, "could not confirm", "no visibility probing noise")
}

func TestStatus_noOtherAccountsNoNotice(t *testing.T) {
	f := &facts.Facts{Hostname: "myhost", Username: "cri", OS: "linux", Backend: "paru"}
	stubStatusDeps(t, f, allInstalledBackend{}, fakeMiseNoOp)
	stubOtherAccounts(t, nil)
	dir := statusMinimalProfile(t)

	var buf bytes.Buffer
	require.NoError(t, (&StatusCmd{Profile: dir, State: filepath.Join(t.TempDir(), "state.json"), out: &buf}).Run())
	require.NotContains(t, buf.String(), "configuration exists for other accounts")
}
