package profile

import (
	"testing"

	"github.com/BurntSushi/toml"
	"github.com/stretchr/testify/require"
)

// The legacy spelling — pre/post as a plain string array — must keep working
// (every existing profile and fixture uses it), decoding each string into a
// required HookCommand.
func TestHooks_stringArrayBackwardCompat(t *testing.T) {
	var s struct {
		Hooks Hooks `toml:"hooks"`
	}
	_, err := toml.Decode(`
[hooks]
pre = ["echo a", "echo b"]
post = ["echo c"]
`, &s)
	require.NoError(t, err)
	require.Equal(t, []HookCommand{{Command: "echo a"}, {Command: "echo b"}}, s.Hooks.Pre)
	require.Equal(t, []HookCommand{{Command: "echo c"}}, s.Hooks.Post)
}

// The structured spelling lets a single hook opt out of fail-fast:
// optional = true means a non-zero exit is logged, not fatal.
func TestHooks_structuredOptional(t *testing.T) {
	var s struct {
		Hooks Hooks `toml:"hooks"`
	}
	_, err := toml.Decode(`
[[hooks.pre]]
command = "echo required"
[[hooks.pre]]
command = "echo flaky"
optional = true
`, &s)
	require.NoError(t, err)
	require.Equal(t, []HookCommand{
		{Command: "echo required"},
		{Command: "echo flaky", Optional: true},
	}, s.Hooks.Pre)
}

// The compact, inline spelling — an inline table inside the string array — is
// the recommended way to flag a single hook optional while keeping the array
// form and the hook's position (interleaving). Same UnmarshalTOML path as the
// table-array form, so it works with no extra decoding machinery.
func TestHooks_inlineTableOptional(t *testing.T) {
	var s struct {
		Hooks Hooks `toml:"hooks"`
	}
	_, err := toml.Decode(`
[hooks]
pre = ["echo setup", { command = "echo flaky", optional = true }, "echo cleanup"]
`, &s)
	require.NoError(t, err)
	require.Equal(t, []HookCommand{
		{Command: "echo setup"},
		{Command: "echo flaky", Optional: true},
		{Command: "echo cleanup"},
	}, s.Hooks.Pre)
}
