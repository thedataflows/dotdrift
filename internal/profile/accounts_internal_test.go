package profile

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/thedataflows/dotdrift/internal/facts"
)

// stubAccountLookup maps account name → [uid, home]; absent = does not exist.
func stubAccountLookup(t *testing.T, accts map[string][2]string) {
	t.Helper()
	orig := lookupAccount
	lookupAccount = func(name string) (string, string, bool) {
		pair, ok := accts[name]
		return pair[0], pair[1], ok
	}
	t.Cleanup(func() { lookupAccount = orig })
}

func TestOtherAccounts_listsExistingAccountsWithModules(t *testing.T) {
	// ghost: account lookup fails; cri: the current account; empty: a modules
	// dir with no module.toml-bearing module inside.
	root := superuserProfile(t, "alice", "root", "ghost", "cri")
	require.NoError(t, os.MkdirAll(filepath.Join(root, "users", "empty", "modules", "nothing"), 0o755))
	stubAccountLookup(t, map[string][2]string{
		"alice": {"1000", "/home/alice"},
		"root":  {"0", "/root"},
		"cri":   {"1000", "/home/cri"},
		"empty": {"1000", "/home/empty"},
	})

	accts, err := OtherAccounts(root, &facts.Facts{Username: "cri"})
	require.NoError(t, err)
	require.Equal(t, []Account{
		{Name: "alice", Uid: "1000", Home: "/home/alice"},
		{Name: "root", Uid: "0", Home: "/root"},
	}, accts, "sorted by name; current account, unresolvable accounts, and module-less layers excluded")
}

func TestOtherAccounts_configOnlyOverlayCounts(t *testing.T) {
	// A user layer holding only dotdrift.toml (no modules) is still
	// configuration for that account (issue 0038).
	root := superuserProfile(t)
	require.NoError(t, os.MkdirAll(filepath.Join(root, "users", "alice"), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(root, "users", "alice", "dotdrift.toml"), []byte("[modules]\n"), 0o644))
	stubAccountLookup(t, map[string][2]string{"alice": {"1000", "/home/alice"}})

	accts, err := OtherAccounts(root, &facts.Facts{Username: "cri"})
	require.NoError(t, err)
	require.Equal(t, []Account{{Name: "alice", Uid: "1000", Home: "/home/alice"}}, accts)
}

func TestOtherAccounts_noUsersDir(t *testing.T) {
	root := t.TempDir()
	require.NoError(t, os.MkdirAll(filepath.Join(root, "modules"), 0o755))

	accts, err := OtherAccounts(root, &facts.Facts{Username: "cri"})
	require.NoError(t, err)
	require.Empty(t, accts)
}
