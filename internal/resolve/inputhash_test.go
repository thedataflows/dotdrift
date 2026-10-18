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

// Issue 0106: the apply input hash is stable for unchanged input and
// changes when a dotfile source file's bytes change in place.
func TestInputHash_changesWithSourceContent(t *testing.T) {
	root := t.TempDir()
	dir := writeModule(t, root, "demo", "[dotfiles]\n\"~/app.conf\" = { source = \"app.conf\", mode = \"copy\" }\n")
	src := filepath.Join(dir, "app.conf")
	require.NoError(t, os.WriteFile(src, []byte("v1\n"), 0o644))

	f := &facts.Facts{Hostname: "h", Username: "u"}
	hash := func() string {
		p, err := profile.Load(root, f)
		require.NoError(t, err)
		plan, err := resolve.Resolve(p, f)
		require.NoError(t, err)
		return resolve.InputHash(p, f, plan)
	}

	require.Equal(t, hash(), hash(), "unchanged input must hash the same")
	before := hash()
	require.NoError(t, os.WriteFile(src, []byte("v2\n"), 0o644))
	require.NotEqual(t, before, hash(), "edited source bytes must change the hash")
}
