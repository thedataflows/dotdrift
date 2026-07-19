package resolve_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/thedataflows/dotdrift/internal/facts"
	"github.com/thedataflows/dotdrift/internal/profile"
	"github.com/thedataflows/dotdrift/internal/resolve"
)

func scopeFacts() *facts.Facts {
	return &facts.Facts{Hostname: "myhost", Username: "cri", OS: "linux"}
}

func entryByTarget(t *testing.T, entries []resolve.DotfileEntry, target string) resolve.DotfileEntry {
	t.Helper()
	for _, e := range entries {
		if e.Target == target {
			return e
		}
	}
	t.Fatalf("dotfile entry %q not found in %v", target, entries)
	return resolve.DotfileEntry{}
}

// Every resolved dotfile entry carries its module's scope.
func TestResolveScope_entriesCarryModuleScope(t *testing.T) {
	p, err := profile.Load(fixture(t, "scope"), scopeFacts())
	require.NoError(t, err)

	plan, err := resolve.Resolve(p, scopeFacts())
	require.NoError(t, err)
	require.Len(t, plan.Dotfiles.Entries, 2)

	demo := entryByTarget(t, plan.Dotfiles.Entries, "/etc/demo.conf")
	require.Equal(t, profile.ScopeSystem, demo.Scope)
	require.Equal(t, "demo", demo.Module)

	bashrc := entryByTarget(t, plan.Dotfiles.Entries, "~/.bashrc")
	require.Equal(t, profile.ScopeUser, bashrc.Scope)
	require.Equal(t, "shell", bashrc.Module)
}

// Mixed user+system modules partition cleanly by scope.
func TestResolveScope_mixedModulesPartition(t *testing.T) {
	p, err := profile.Load(fixture(t, "scope"), scopeFacts())
	require.NoError(t, err)

	plan, err := resolve.Resolve(p, scopeFacts())
	require.NoError(t, err)

	var user, system []resolve.DotfileEntry
	for _, e := range plan.Dotfiles.Entries {
		switch e.Scope {
		case profile.ScopeSystem:
			system = append(system, e)
		case profile.ScopeUser:
			user = append(user, e)
		default:
			t.Fatalf("entry %q has unexpected scope %q", e.Target, e.Scope)
		}
	}
	require.Len(t, user, 1)
	require.Len(t, system, 1)
	require.Equal(t, "~/.bashrc", user[0].Target)
	require.Equal(t, "/etc/demo.conf", system[0].Target)
}

// An unrecognized scope value is a resolve-time error naming the module and
// the value (same fail-loud pattern as dotfile mode validation).
func TestResolveScope_invalidScopeErrors(t *testing.T) {
	root := t.TempDir()
	modDir := filepath.Join(root, "modules", "bad")
	require.NoError(t, os.MkdirAll(modDir, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(modDir, "module.toml"), []byte(
		"id = \"bad\"\nscope = \"bogus\"\n\n[dotfiles]\n\"~/x\" = { source = \"x\", mode = \"link\" }\n"), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(modDir, "x"), []byte("x"), 0o644))

	p, err := profile.Load(root, scopeFacts())
	require.NoError(t, err)

	_, err = resolve.Resolve(p, scopeFacts())
	require.Error(t, err)
	require.Contains(t, err.Error(), "bad")
	require.Contains(t, err.Error(), "bogus")
}

// The invalid-scope error fires even when the module declares no dotfiles:
// scope is module-level metadata and must never pass silently.
func TestResolveScope_invalidScopeErrorsWithoutDotfiles(t *testing.T) {
	root := t.TempDir()
	modDir := filepath.Join(root, "modules", "bad")
	require.NoError(t, os.MkdirAll(modDir, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(modDir, "module.toml"), []byte(
		"id = \"bad\"\nscope = \"bogus\"\n"), 0o644))

	p, err := profile.Load(root, scopeFacts())
	require.NoError(t, err)

	_, err = resolve.Resolve(p, scopeFacts())
	require.Error(t, err)
	require.Contains(t, err.Error(), "bad")
	require.Contains(t, err.Error(), "bogus")
}

// Existing fixtures without a scope key resolve every entry as user scope.
func TestResolveScope_existingFixturesDefaultToUser(t *testing.T) {
	p, err := profile.Load(fixture(t, "resolve"), scopeFacts())
	require.NoError(t, err)

	plan, err := resolve.Resolve(p, scopeFacts())
	require.NoError(t, err)
	require.NotEmpty(t, plan.Dotfiles.Entries)
	for _, e := range plan.Dotfiles.Entries {
		require.Equal(t, profile.ScopeUser, e.Scope, "entry %q must default to user scope", e.Target)
	}
}
