package resolve_test

import (
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/thedataflows/dotdrift/internal/facts"
)

// The shared resolve fixture declares [hooks] in all three layers of the
// shell module; layer merging appends base → host → user in that order.
func TestMergeHooks_layerAppendOrder(t *testing.T) {
	plan, err := loadAndResolve(t, fixture(t, "resolve"), &facts.Facts{
		Hostname: "myhost",
		Username: "cri",
		OS:       "linux",
	})
	require.NoError(t, err)

	require.Equal(t, []string{"echo base-pre", "echo host-pre", "echo user-pre"}, plan.Hooks.Pre,
		"pre hooks must append base → host → user")
	require.Equal(t, []string{"echo base-post", "echo host-post", "echo user-post"}, plan.Hooks.Post,
		"post hooks must append base → host → user")
}

// Hooks from several selected modules aggregate in selection order (modules
// are discovered sorted by ID, so a runs before b).
func TestMergeHooks_multiModuleSelectionOrder(t *testing.T) {
	root := t.TempDir()
	writeModule(t, root, "a", "[hooks]\npre = [\"a-pre\"]\npost = [\"a-post\"]\n")
	writeModule(t, root, "b", "[hooks]\npre = [\"b-pre-1\", \"b-pre-2\"]\n")

	plan, err := loadAndResolve(t, root, &facts.Facts{Hostname: "h", Username: "u"})
	require.NoError(t, err)

	require.Equal(t, []string{"a-pre", "b-pre-1", "b-pre-2"}, plan.Hooks.Pre,
		"pre hooks must aggregate in module selection order")
	require.Equal(t, []string{"a-post"}, plan.Hooks.Post,
		"post hooks must only contain modules that declare them")
}

// A module without a [hooks] section contributes nothing; Pre/Post stay empty.
func TestMergeHooks_emptyHooks(t *testing.T) {
	root := t.TempDir()
	writeModule(t, root, "plain", "[packages]\npresent = [\"ripgrep\"]\n")

	plan, err := loadAndResolve(t, root, &facts.Facts{Hostname: "h", Username: "u"})
	require.NoError(t, err)

	require.Empty(t, plan.Hooks.Pre)
	require.Empty(t, plan.Hooks.Post)
}

// A module may declare only one of pre/post.
func TestMergeHooks_preOnly(t *testing.T) {
	root := t.TempDir()
	writeModule(t, root, "mod", "[hooks]\npre = [\"only-pre\"]\n")

	plan, err := loadAndResolve(t, root, &facts.Facts{Hostname: "h", Username: "u"})
	require.NoError(t, err)

	require.Equal(t, []string{"only-pre"}, plan.Hooks.Pre)
	require.Empty(t, plan.Hooks.Post)
}
