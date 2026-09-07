package resolve_test

import (
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/thedataflows/dotdrift/internal/facts"
	"github.com/thedataflows/dotdrift/internal/profile"
)

// Secrets merge like tools: unioned across selected modules and layers,
// nearer layer (user > host > base) replacing a same-name entry (issue 0044).
func TestResolve_secretsMergeAcrossLayers(t *testing.T) {
	root := t.TempDir()
	writeModule(t, root, "svc", `
[secrets]
cache_token = "BASE_TOKEN"
db_password = { env = "BASE_DB" }
`)
	writeOverlayModule(t, root, "users", "cri", "svc", `
[secrets]
cache_token = "USER_TOKEN"
`)

	plan, err := loadAndResolve(t, root, &facts.Facts{Hostname: "h", Username: "cri"})
	require.NoError(t, err)

	require.Equal(t, profile.Secret{Env: "USER_TOKEN"}, plan.Secrets["cache_token"],
		"user overlay replaces the base entry for the same name")
	require.Equal(t, profile.Secret{Env: "BASE_DB"}, plan.Secrets["db_password"],
		"untouched base entries survive")
}

// Secrets union across modules.
func TestResolve_secretsUnionAcrossModules(t *testing.T) {
	root := t.TempDir()
	writeModule(t, root, "a", `
[secrets]
tok_a = "A"
`)
	writeModule(t, root, "b", `
[secrets]
tok_b = { env = "B", description = "b" }
`)

	plan, err := loadAndResolve(t, root, &facts.Facts{Hostname: "h", Username: "cri"})
	require.NoError(t, err)
	require.Equal(t, profile.Secret{Env: "A"}, plan.Secrets["tok_a"])
	require.Equal(t, profile.Secret{Env: "B", Description: "b"}, plan.Secrets["tok_b"])
}

// A profile without secrets resolves an empty map, not nil — emission must
// stay a no-section no-op.
func TestResolve_noSecretsEmptyMap(t *testing.T) {
	root := t.TempDir()
	writeModule(t, root, "plain", `
[packages]
present = ["x"]
`)

	plan, err := loadAndResolve(t, root, &facts.Facts{Hostname: "h", Username: "cri"})
	require.NoError(t, err)
	require.Empty(t, plan.Secrets)
}
