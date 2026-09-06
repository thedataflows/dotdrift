package profile

import (
	"os"
	"os/user"
	"path/filepath"
	"sort"

	"github.com/thedataflows/dotdrift/internal/facts"
)

// ReasonSuperuserOverlay is the Skip.Reason stamped on modules that exist only
// under a superuser-owned user layer (users/<uid-0 account>/modules/). An
// unprivileged run cannot select them — user layers key on the current OS
// account — so they surface as skipped with an actionable reason instead of
// vanishing (issue 0029).
const ReasonSuperuserOverlay = "requires root (run with sudo)"

// lookupUID resolves an OS account name to its uid; ok=false when the account
// does not exist. A test seam.
var lookupUID = func(name string) (string, bool) {
	u, err := user.Lookup(name)
	if err != nil {
		return "", false
	}
	return u.Uid, true
}

// markSuperuserOverlays appends a skip entry for every module under a
// superuser-owned user layer that is not the current account's. Those modules
// are undiscoverable by the layer scan (which only reads users/<username>/),
// and without an entry the profile looks empty for them. Running as the uid-0
// account itself selects normally, so nothing is appended then. A directory
// naming no OS account, or an account whose uid is not 0, is an ordinary
// other-user overlay and stays invisible. The superuser test is uid == 0 —
// never gid (root-group membership is not superuser). Selection is untouched:
// skipped modules never reach resolve.
func (p *Profile) markSuperuserOverlays(root string, f *facts.Facts) error {
	usersDir := filepath.Join(root, "users")
	entries, err := os.ReadDir(usersDir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}
	var skips []Skip
	for _, entry := range entries {
		name := entry.Name()
		if !entry.IsDir() || name == f.Username {
			continue
		}
		if uid, ok := lookupUID(name); !ok || uid != "0" {
			continue
		}
		mods, err := os.ReadDir(filepath.Join(usersDir, name, "modules"))
		if err != nil {
			if os.IsNotExist(err) {
				continue
			}
			return err
		}
		for _, m := range mods {
			if !m.IsDir() {
				continue
			}
			mod, err := loadModule(filepath.Join(usersDir, name, "modules", m.Name()), m.Name())
			if err != nil {
				return err
			}
			if mod == nil {
				continue
			}
			skips = append(skips, Skip{Module: *mod, Reason: ReasonSuperuserOverlay})
		}
	}
	sort.Slice(skips, func(i, j int) bool { return skips[i].Module.ID < skips[j].Module.ID })
	p.Skipped = append(p.Skipped, skips...)
	return nil
}
