package mise_test

import (
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/thedataflows/dotdrift/internal/mise"
	"github.com/thedataflows/dotdrift/internal/profile"
)

// [bootstrap.secrets] emission (issue 0044): short form when only env is
// set, table form when description or allow_empty is present; sorted by
// logical name; empty when no secrets are declared.
func TestGenerateBootstrapSecrets_empty(t *testing.T) {
	require.Empty(t, mise.GenerateBootstrapSecrets(nil))
	require.Empty(t, mise.GenerateBootstrapSecrets(map[string]profile.Secret{}))
}

func TestGenerateBootstrapSecrets_shortForm(t *testing.T) {
	got := mise.GenerateBootstrapSecrets(map[string]profile.Secret{
		"cache_token": {Env: "MISE_CACHE_TOKEN"},
	})
	require.Equal(t, "[bootstrap.secrets]\ncache_token = \"MISE_CACHE_TOKEN\"\n", got)
}

func TestGenerateBootstrapSecrets_tableForm(t *testing.T) {
	got := mise.GenerateBootstrapSecrets(map[string]profile.Secret{
		"db": {Env: "DB_PASSWORD", Description: "prod db", AllowEmpty: true},
	})
	require.Equal(t, "[bootstrap.secrets]\ndb = { env = \"DB_PASSWORD\", description = \"prod db\", allow_empty = true }\n", got)
}

func TestGenerateBootstrapSecrets_sortedAndEscaped(t *testing.T) {
	got := mise.GenerateBootstrapSecrets(map[string]profile.Secret{
		"zoe": {Env: "Z"},
		"amy": {Env: "A\"B"},
	})
	require.Less(t, indexOfStr(got, `amy =`), indexOfStr(got, `zoe =`))
	require.Contains(t, got, `"A\"B"`, "env values are TOML-escaped")
}

func indexOfStr(s, sub string) int {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return i
		}
	}
	return -1
}
