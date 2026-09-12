package tui

import (
	"maps"
	"path/filepath"
	"slices"
	"strconv"
	"strings"

	"github.com/thedataflows/dotdrift/internal/drift"
	"github.com/thedataflows/dotdrift/internal/profile"
	"github.com/thedataflows/dotdrift/internal/service"
)

// The shell's tree model (0062-D2/D8): three root groups — Modules,
// Accounts, Profile — derived from the modules read model. One node per
// module merged across layers (contract 1) with an overlay-count badge and
// its overlay origins as children; accounts as plain nodes carrying
// account facts, with the superuser overlay visible per issue 0029; plan
// and status as views, onboard/restore/generate as (for now, disabled)
// actions. Pure derivation: read model in, branches out — the shell maps
// them onto the bubbles tree.

type viewKind int

const (
	viewModule viewKind = iota
	viewOrigin
	viewEditor
	viewAccount
	viewStatus
	viewPlan
	viewAction
	viewGroup
)

type nodeKind int

const (
	kindGroup nodeKind = iota
	kindModule
	kindOrigin
	kindAccount
	kindView
	kindAction
)

// treeItem is one tree node's payload; label is what renders.
type treeItem struct {
	kind     nodeKind
	label    string
	moduleID string // module + origin: the declared id (plan filter input)
	origin   string // origin marker: base, hosts/<h>, users/<u>, superuser
	dir      string // module/origin: the layer dir (raw declaration input)
	view     viewKind
	action   string // kindAction: onboard | restore | generate
	acctKind string // kindAccount: host | user | superuser
	acctName string // kindAccount
	current  bool   // kindAccount: this machine's account
	reason   string // skip reason (requires root, …)
}

// String is what the bubbles tree renders: the label.
func (i treeItem) String() string { return i.label }

// branch is a tree node with its children; the shell flattens these onto
// the bubbles tree.
type branch struct {
	item treeItem
	kids []branch
}

// buildTree derives the tree model from the modules read. Nil-safe: no
// profile, no groups (the shell shows its loading state until the read
// lands).
func buildTree(read *service.ModulesRead) []branch {
	if read == nil || read.Profile == nil {
		return nil
	}
	var host, user string
	if f := read.Facts; f != nil {
		host, user = f.Hostname, f.Username
	}
	layers := service.ModuleLayers(read.Profile)
	superOwners := superuserOwners(read.Profile.Skipped)

	modules := branch{item: treeItem{kind: kindGroup, label: "MODULES"}}
	for _, m := range read.Profile.Modules {
		origins := moduleOrigins(layers, filepath.Base(m.Path), host, user, superOwners)
		modules.kids = append(modules.kids, moduleBranch(m, origins))
	}
	// Superuser-only modules never enter Profile.Modules (issue 0029) —
	// they surface from the skip list so the profile doesn't look empty.
	known := map[string]bool{}
	for _, m := range read.Profile.Modules {
		known[m.ID] = true
	}
	for _, s := range read.Profile.Skipped {
		if s.Reason != profile.ReasonSuperuserOverlay || known[s.Module.ID] {
			continue
		}
		b := moduleBranch(s.Module, moduleOrigins(layers, filepath.Base(s.Module.Path), host, user, superOwners))
		b.item.reason = s.Reason
		modules.kids = append(modules.kids, b)
	}

	accounts := branch{item: treeItem{kind: kindGroup, label: "ACCOUNTS"}}
	accounts.kids = append(accounts.kids, accountBranches("host", hostOwners(layers, host), host)...)
	accounts.kids = append(accounts.kids, accountBranches("user", userOwners(layers, user, superOwners), user)...)
	for _, name := range sortedOwnerSet(superOwners) {
		accounts.kids = append(accounts.kids, branch{item: treeItem{
			kind: kindAccount, acctKind: "superuser", acctName: name,
			label:  "user " + name + " (superuser)",
			reason: profile.ReasonSuperuserOverlay,
		}})
	}

	profileGroup := branch{item: treeItem{kind: kindGroup, label: "PROFILE"}}
	profileGroup.kids = append(profileGroup.kids,
		branch{item: treeItem{kind: kindView, view: viewPlan, label: "plan"}},
		branch{item: treeItem{kind: kindView, view: viewStatus, label: "status"}},
	)
	for _, a := range []string{"onboard", "restore", "generate"} {
		profileGroup.kids = append(profileGroup.kids, branch{item: treeItem{
			kind: kindAction, action: a, label: a,
		}})
	}

	return []branch{
		modules,
		accounts,
		profileGroup,
	}
}

// moduleBranch builds one module node (id + overlay-count badge + marks)
// with its origins as children.
func moduleBranch(m profile.Module, origins []origin) branch {
	b := branch{item: treeItem{
		kind:     kindModule,
		label:    m.ID,
		moduleID: m.ID,
		dir:      m.Path,
	}}
	if len(origins) > 1 {
		b.item.label += " ·" + strconv.Itoa(len(origins))
	}
	if m.Config.Disabled {
		b.item.label += " (disabled)"
	}
	for _, o := range origins {
		b.kids = append(b.kids, branch{item: treeItem{
			kind: kindOrigin, label: o.label, origin: o.label,
			moduleID: b.item.moduleID, dir: o.dir,
			view: viewOrigin,
		}})
	}
	return b
}

// origin is one contributing layer of a module: marker label + dir.
type origin struct {
	label, dir string
}

// moduleOrigins collects the module's contributing origins in merge order
// (base → host → user → superuser). Only ACTIVE layers contribute — other
// hosts' overlays are invisible by design, ordinary other-user overlays
// are not selectable, and superuser-owned ones appear under the
// "superuser" marker (issue 0029).
func moduleOrigins(layers []drift.ModuleLayer, dirName, host, user string, superOwners map[string]bool) []origin {
	var out []origin
	for _, l := range layers {
		if l.Dir != dirName {
			continue
		}
		switch {
		case l.Layer == "base":
			out = append(out, origin{label: "base", dir: l.Path})
		case l.Layer == "host" && l.Owner == host && host != "":
			out = append(out, origin{label: "hosts/" + l.Owner, dir: l.Path})
		case l.Layer == "user" && user != "" && l.Owner == user && !superOwners[l.Owner]:
			out = append(out, origin{label: "users/" + l.Owner, dir: l.Path})
		case l.Layer == "user" && superOwners[l.Owner]:
			out = append(out, origin{label: "superuser", dir: l.Path})
		}
	}
	return out
}

// superuserOwners extracts the uid-0 overlay owners the profile loader
// marked (issue 0029): users/<owner>/modules/… paths on skip entries with
// the superuser reason. The uid lookup itself stays in profile — the tree
// only reads the verdict.
func superuserOwners(skips []profile.Skip) map[string]bool {
	owners := map[string]bool{}
	for _, s := range skips {
		if s.Reason != profile.ReasonSuperuserOverlay {
			continue
		}
		if owner, ok := userOverlayOwner(s.Module.Path); ok {
			owners[owner] = true
		}
	}
	return owners
}

// userOverlayOwner pulls the users/<owner> segment out of a module path.
func userOverlayOwner(path string) (string, bool) {
	parts := strings.Split(filepath.ToSlash(path), "/")
	for i, part := range parts {
		if part == "users" && i+1 < len(parts) {
			return parts[i+1], true
		}
	}
	return "", false
}

// accountBranches builds host or user account nodes: the current account
// first, remaining owners sorted. The header already names this machine's
// host/user context, so account labels stay bare — long labels wrap the
// bubbles tree and knock its cursor marker off-row.
func accountBranches(kind string, owners []string, currentName string) []branch {
	out := make([]branch, 0, len(owners))
	for _, name := range owners {
		current := name == currentName && currentName != ""
		out = append(out, branch{item: treeItem{
			kind: kindAccount, acctKind: kind, acctName: name,
			label: kind + " " + name, current: current,
		}})
	}
	return out
}

// hostOwners lists the profile's host-layer owners, current host first.
func hostOwners(layers []drift.ModuleLayer, current string) []string {
	owners := ownerSet(layers, "host", nil)
	if current != "" {
		owners[current] = true // always show this machine, module-less or not
	}
	return orderOwners(owners, current)
}

// userOwners lists user-layer owners minus superuser ones, current user
// first. Superuser owners appear as their own nodes instead (0029).
func userOwners(layers []drift.ModuleLayer, current string, superOwners map[string]bool) []string {
	owners := ownerSet(layers, "user", superOwners)
	if current != "" {
		owners[current] = true
	}
	return orderOwners(owners, current)
}

func ownerSet(layers []drift.ModuleLayer, layer string, exclude map[string]bool) map[string]bool {
	owners := map[string]bool{}
	for _, l := range layers {
		if l.Layer == layer && l.Owner != "" && !exclude[l.Owner] {
			owners[l.Owner] = true
		}
	}
	return owners
}

// orderOwners orders current first, the rest sorted.
func orderOwners(owners map[string]bool, current string) []string {
	names := slices.Sorted(maps.Keys(owners))
	if current != "" && owners[current] {
		names = append([]string{current}, slices.DeleteFunc(names, func(n string) bool { return n == current })...)
	}
	return names
}

func sortedOwnerSet(m map[string]bool) []string { return slices.Sorted(maps.Keys(m)) }
