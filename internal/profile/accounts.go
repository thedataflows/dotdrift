package profile

import (
	"os"
	"os/user"
	"path/filepath"
	"sort"

	"github.com/thedataflows/dotdrift/internal/facts"
)

// Account is another OS account that owns configuration in the profile: the
// account exists on this system and users/<name>/ holds at least one module
// or a dotdrift.toml (a config-only overlay is still configuration for that
// account). Used by status's multi-account notice (issue 0038); selection
// never consults it — user layers are selected only for the current account.
type Account struct {
	Name string
	//nolint:staticcheck
	Uid  string
	Home string
}

// lookupAccount resolves an OS account name to its uid and home directory;
// ok=false when the account does not exist. A test seam.
var lookupAccount = func(name string) (string, string, bool) {
	u, err := user.Lookup(name)
	if err != nil {
		return "", "", false
	}
	return u.Uid, u.HomeDir, true
}

// OtherAccounts lists the OS accounts owning user layers, for multi-account
// status reporting: every users/<name>/ directory where <name> resolves to an
// existing OS account other than the current one and holds at least one
// module. Unresolvable names stay invisible (the same rule other-user
// overlays follow in selection), as does the current account — its view is
// the main report. Sorted by name.
func OtherAccounts(root string, f *facts.Facts) ([]Account, error) {
	entries, err := os.ReadDir(filepath.Join(root, "users"))
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	var accts []Account
	for _, entry := range entries {
		name := entry.Name()
		if !entry.IsDir() || name == f.Username {
			continue
		}
		uid, home, ok := lookupAccount(name)
		if !ok {
			continue
		}
		accountDir := filepath.Join(root, "users", name)
		has, err := hasModules(filepath.Join(accountDir, "modules"))
		if err != nil {
			return nil, err
		}
		if !has {
			// A config-only overlay (dotdrift.toml, no modules) is still
			// configuration for the account (issue 0038).
			if _, err := os.Stat(filepath.Join(accountDir, "dotdrift.toml")); err == nil {
				has = true
			}
		}
		if has {
			accts = append(accts, Account{Name: name, Uid: uid, Home: home})
		}
	}
	sort.Slice(accts, func(i, j int) bool { return accts[i].Name < accts[j].Name })
	return accts, nil
}

// hasModules reports whether dir holds at least one subdirectory containing a
// module.toml (a module). A missing dir is not an error.
func hasModules(dir string) (bool, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return false, nil
		}
		return false, err
	}
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		if _, err := os.Stat(filepath.Join(dir, entry.Name(), "module.toml")); err == nil {
			return true, nil
		}
	}
	return false, nil
}
