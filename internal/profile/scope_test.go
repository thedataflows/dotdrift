package profile_test

import (
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/thedataflows/dotdrift/internal/facts"
	"github.com/thedataflows/dotdrift/internal/profile"
)

// scope = "system" parses from module.toml.
func TestModuleScope_systemParses(t *testing.T) {
	p, err := profile.Load(fixture(t, "scope"), &facts.Facts{})
	require.NoError(t, err)
	m := findModule(t, p, "demo")
	require.Equal(t, profile.ScopeSystem, m.Config.Scope)
	require.Equal(t, profile.ScopeSystem, m.Config.ScopeOrDefault())
}

// scope = "user" parses explicitly.
func TestModuleScope_userParses(t *testing.T) {
	p, err := profile.Load(fixture(t, "scope"), &facts.Facts{})
	require.NoError(t, err)
	m := findModule(t, p, "shell")
	require.Equal(t, profile.ScopeUser, m.Config.Scope)
	require.Equal(t, profile.ScopeUser, m.Config.ScopeOrDefault())
}

// An omitted scope defaults to user.
func TestModuleScope_omittedDefaultsToUser(t *testing.T) {
	p, err := profile.Load(fixture(t, "simple"), &facts.Facts{})
	require.NoError(t, err)
	m := findModule(t, p, "b")
	require.Empty(t, m.Config.Scope, "raw field stays empty when omitted")
	require.Equal(t, profile.ScopeUser, m.Config.ScopeOrDefault())
}
