package profile

import (
	"strings"
	"testing"

	"github.com/BurntSushi/toml"
	"github.com/stretchr/testify/require"
)

// [secrets] accepts two spellings per entry: the short form maps a logical
// name straight to an environment variable; the table form adds description
// and allow_empty (issue 0044).
func TestSecrets_shortAndTableForms(t *testing.T) {
	var s struct {
		Secrets map[string]Secret `toml:"secrets"`
	}
	_, err := toml.Decode(`
[secrets]
cache_token = "MISE_CACHE_TOKEN"
database_password = { env = "DB_PASSWORD", description = "prod db", allow_empty = true }
`, &s)
	require.NoError(t, err)
	require.Equal(t, Secret{Env: "MISE_CACHE_TOKEN"}, s.Secrets["cache_token"])
	require.Equal(t, Secret{Env: "DB_PASSWORD", Description: "prod db", AllowEmpty: true}, s.Secrets["database_password"])
}

// The sub-table spelling decodes identically.
func TestSecrets_subTableForm(t *testing.T) {
	var s struct {
		Secrets map[string]Secret `toml:"secrets"`
	}
	_, err := toml.Decode(`
[secrets.cache_token]
env = "MISE_CACHE_TOKEN"
description = "cache"
`, &s)
	require.NoError(t, err)
	require.Equal(t, Secret{Env: "MISE_CACHE_TOKEN", Description: "cache"}, s.Secrets["cache_token"])
}

// Unknown keys in a secret table are loud errors listing the valid keys —
// the custom unmarshaler consumes the subtree, so Undecoded never sees them
// (same contract as structured hooks).
func TestSecrets_unknownKeyRejected(t *testing.T) {
	var s struct {
		Secrets map[string]Secret `toml:"secrets"`
	}
	_, err := toml.Decode(`
[secrets]
tok = { env = "X", vaule = "oops" }
`, &s)
	require.Error(t, err)
	require.Contains(t, err.Error(), `unknown key "vaule"`)
	require.Contains(t, err.Error(), "env, description, allow_empty")
}

// A secret without an env var is meaningless — error in both spellings.
func TestSecrets_missingEnvRejected(t *testing.T) {
	for name, src := range map[string]string{
		"table": `[secrets]
tok = { description = "no env" }
`,
		"empty short": `[secrets]
tok = ""
`,
	} {
		t.Run(name, func(t *testing.T) {
			var s struct {
				Secrets map[string]Secret `toml:"secrets"`
			}
			_, err := toml.Decode(src, &s)
			require.Error(t, err)
			require.True(t, strings.Contains(err.Error(), "env"), "error must name env: %v", err)
		})
	}
}

// A non-string/non-table value is a type error, not a zero value.
func TestSecrets_scalarTypeError(t *testing.T) {
	var s struct {
		Secrets map[string]Secret `toml:"secrets"`
	}
	_, err := toml.Decode(`
[secrets]
tok = 42
`, &s)
	require.Error(t, err)
}
