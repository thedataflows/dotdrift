package tui

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/thedataflows/dotdrift/internal/facts"
	"github.com/thedataflows/dotdrift/internal/profile"
	"github.com/thedataflows/dotdrift/internal/service"
)

// The shell's tree model (0062-D2/D8, T-tui-shell): three root groups, one
// node per module merged across layers (contract 1), overlay origins as
// expandable children, accounts as plain nodes with the superuser
// visibility rule (issue 0029). Built from the modules read model — pure
// data in, pure tree out.

// treeFixture writes the layer directories the tree derivation walks and
// returns the modules read over them. Discovery is simulated by the hand
// built Profile (base-preferred representative paths, the superuser skip),
// matching profile.Load's contract for these directories:
//
//	modules/shell + hosts/myhost/modules/shell + users/cri/modules/shell
//	modules/editor (base only)
//	users/root/modules/vault   — superuser overlay (issue 0029)
//	users/alice/modules/tools  — ordinary other-user overlay (invisible)
//	hosts/other/modules/hidden — other-host overlay (invisible by design)
func treeFixture(t *testing.T) *service.ModulesRead {
	t.Helper()
	dir := t.TempDir()
	write := func(rel string) {
		modDir := filepath.Join(dir, rel)
		require.NoError(t, os.MkdirAll(modDir, 0o755))
		require.NoError(t, os.WriteFile(filepath.Join(modDir, "module.toml"), []byte("id = \"x\"\n"), 0o644))
	}
	write(filepath.Join("modules", "shell"))
	write(filepath.Join("modules", "editor"))
	write(filepath.Join("hosts", "myhost", "modules", "shell"))
	write(filepath.Join("users", "cri", "modules", "shell"))
	write(filepath.Join("users", "root", "modules", "vault"))
	write(filepath.Join("users", "alice", "modules", "tools"))
	write(filepath.Join("hosts", "other", "modules", "hidden"))

	return &service.ModulesRead{
		Facts: &facts.Facts{Hostname: "myhost", Username: "cri", OS: "linux"},
		Profile: &profile.Profile{
			Root: dir,
			Modules: []profile.Module{
				{ID: "editor", Path: filepath.Join(dir, "modules", "editor")},
				{ID: "shell", Path: filepath.Join(dir, "modules", "shell")},
			},
			Skipped: []profile.Skip{
				{
					Module: profile.Module{ID: "vault", Path: filepath.Join(dir, "users", "root", "modules", "vault")},
					Reason: profile.ReasonSuperuserOverlay,
				},
			},
		},
	}
}

func groupByLabel(t *testing.T, bs []branch, label string) branch {
	t.Helper()
	for _, b := range bs {
		if b.item.label == label {
			return b
		}
	}
	t.Fatalf("no group %q in %+v", label, bs)
	return branch{}
}

func TestTree_groupsAndOrdering(t *testing.T) {
	bs := buildTree(treeFixture(t))
	require.Len(t, bs, 3, "three root groups")
	require.Equal(t, []string{"MODULES", "ACCOUNTS", "PROFILE"}, []string{bs[0].item.label, bs[1].item.label, bs[2].item.label})

	mods := groupByLabel(t, bs, "MODULES")
	labels := make([]string, 0, len(mods.kids))
	for _, k := range mods.kids {
		labels = append(labels, k.item.label)
	}
	// One node per module across layers (contract 1): shell exists in base,
	// hosts/myhost, and users/cri — a single node, badge-sized ·3. Sorted
	// by id: editor before shell; the superuser-only module vault surfaces
	// last (issue 0029).
	require.Equal(t, []string{"editor", "shell ·3", "vault"}, labels)
}

func TestTree_overlayOriginsExpand(t *testing.T) {
	bs := buildTree(treeFixture(t))
	mods := groupByLabel(t, bs, "MODULES")

	var shell branch
	for _, k := range mods.kids {
		if k.item.moduleID == "shell" {
			shell = k
		}
	}
	require.Equal(t, kindModule, shell.item.kind)
	// Origins in merge order: base → host → user (contract merge order).
	got := make([]string, 0, len(shell.kids))
	for _, o := range shell.kids {
		require.Equal(t, kindOrigin, o.item.kind)
		require.Equal(t, "shell", o.item.moduleID)
		require.NotEmpty(t, o.item.dir, "origin carries its layer dir")
		got = append(got, o.item.origin)
	}
	require.Equal(t, []string{"base", "hosts/myhost", "users/cri"}, got)

	// The superuser overlay module (issue 0029) surfaces with its
	// requires-root reason and the superuser origin marker.
	var vault *branch
	for i, k := range mods.kids {
		if k.item.moduleID == "vault" {
			vault = &mods.kids[i]
		}
	}
	require.NotNil(t, vault, "superuser-only modules surface in the tree")
	require.Equal(t, profile.ReasonSuperuserOverlay, vault.item.reason)
	require.Len(t, vault.kids, 1)
	require.Equal(t, "superuser", vault.kids[0].item.origin)

	// Invisible overlays stay invisible (merge rules; issue 0029 scope):
	// the other-host module "hidden" and the ordinary other-user module
	// "tools" get no module node and no origin anywhere.
	for _, b := range bs {
		for _, k := range b.kids {
			require.NotEqual(t, "hidden", k.item.moduleID, "other-host overlays are invisible by design")
			require.NotEqual(t, "tools", k.item.moduleID, "ordinary other-user overlays are not selectable modules")
			for _, o := range k.kids {
				require.NotEqual(t, "hosts/other", o.item.origin)
				require.NotEqual(t, "users/alice", o.item.origin)
			}
		}
	}
}

func TestTree_accountsNodes(t *testing.T) {
	bs := buildTree(treeFixture(t))
	accts := groupByLabel(t, bs, "ACCOUNTS")

	type ref struct {
		kind, name string
		current    bool
	}
	var got []ref
	var labels []string
	for _, k := range accts.kids {
		require.Equal(t, kindAccount, k.item.kind)
		require.Empty(t, k.kids, "accounts are plain nodes")
		got = append(got, ref{k.item.acctKind, k.item.acctName, k.item.current})
		labels = append(labels, k.item.label)
	}
	// Hosts (current first, then sorted), users (current first, sorted,
	// superuser owners excluded), superuser nodes last. The superuser
	// overlay is visible per issue 0029's rule; ordinary other-user
	// overlays (alice) show as account nodes carrying their configuration.
	require.Equal(t, []ref{
		{"host", "myhost", true},
		{"host", "other", false},
		{"user", "cri", true},
		{"user", "alice", false},
		{"superuser", "root", false},
	}, got)
	require.Contains(t, labels[4], "superuser", "the superuser node is labeled as such")
}

func TestTree_profileGroup(t *testing.T) {
	bs := buildTree(treeFixture(t))
	prof := groupByLabel(t, bs, "PROFILE")
	require.Len(t, prof.kids, 5, "plan, status, onboard, restore, generate")

	// Plan and status are views; onboard/restore/generate are actions that
	// stay disabled stubs until T-tui-writes lands.
	require.Equal(t, viewPlan, prof.kids[0].item.view)
	require.Equal(t, kindView, prof.kids[0].item.kind)
	require.Equal(t, viewStatus, prof.kids[1].item.view)
	for i, want := range []string{"onboard", "restore", "generate"} {
		require.Equal(t, kindAction, prof.kids[2+i].item.kind)
		require.Equal(t, want, prof.kids[2+i].item.action)
	}
}

func TestTree_emptyAndNilReads(t *testing.T) {
	require.Empty(t, buildTree(nil))
	require.Empty(t, buildTree(&service.ModulesRead{Facts: &facts.Facts{}}), "nil profile builds no groups")

	// Unknown identity (detect failure) degrades cleanly: account nodes
	// come from the profile's layers only — never an empty-named node.
	read := treeFixture(t)
	read.Facts = &facts.Facts{}
	bs := buildTree(read)
	accts := groupByLabel(t, bs, "ACCOUNTS")
	for _, k := range accts.kids {
		require.NotEmpty(t, k.item.acctName, "no blank account nodes")
		require.False(t, k.item.current, "nothing is current when facts are unknown")
	}
}
