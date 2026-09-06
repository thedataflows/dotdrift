package profile

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/thedataflows/dotdrift/internal/facts"
)

func stubAccountLookup(t *testing.T, homes map[string]string) {
	t.Helper()
	orig := lookupAccount
	lookupAccount = func(name string) (string, bool) {
		home, ok := homes[name]
		return home, ok
	}
	t.Cleanup(func() { lookupAccount = orig })
}

func TestOtherAccounts_listsExistingAccountsWithModules(t *testing.T) {
	// ghost: account lookup fails; cri: the current account; empty: a modules
	// dir with no module.toml-bearing module inside.
	root := superuserProfile(t, "alice", "root", "ghost", "cri")
	require.NoError(t, os.MkdirAll(filepath.Join(root, "users", "empty", "modules", "nothing"), 0o755))
	stubAccountLookup(t, map[string]string{
		"alice": "/home/alice",
		"root":  "/root",
		"cri":   "/home/cri",
		"empty": "/home/empty",
	})

	accts, err := OtherAccounts(root, &facts.Facts{Username: "cri"})
	require.NoError(t, err)
	require.Equal(t, []Account{
		{Name: "alice", Home: "/home/alice"},
		{Name: "root", Home: "/root"},
	}, accts, "sorted by name; current account, unresolvable accounts, and module-less layers excluded")
}

func TestOtherAccounts_noUsersDir(t *testing.T) {
	root := t.TempDir()
	require.NoError(t, os.MkdirAll(filepath.Join(root, "modules"), 0o755))

	accts, err := OtherAccounts(root, &facts.Facts{Username: "cri"})
	require.NoError(t, err)
	require.Empty(t, accts)
}
