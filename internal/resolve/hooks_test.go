package resolve_test

import (
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/thedataflows/dotdrift/internal/facts"
	"github.com/thedataflows/dotdrift/internal/profile"
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

	require.Equal(t, []profile.HookCommand{
		{Command: "echo base-pre", Module: "shell"},
		{Command: "echo host-pre", Module: "shell"},
		{Command: "echo user-pre", Module: "shell"},
	}, plan.Hooks.Pre, "pre hooks must append base → host → user and carry the module name")
	require.Equal(t, []profile.HookCommand{
		{Command: "echo base-post", Module: "shell"},
		{Command: "echo host-post", Module: "shell"},
		{Command: "echo user-post", Module: "shell"},
	}, plan.Hooks.Post, "post hooks must append base → host → user and carry the module name")
}

// Hooks from several selected modules aggregate in selection order (modules
// are discovered sorted by ID, so a runs before b).
func TestMergeHooks_multiModuleSelectionOrder(t *testing.T) {
	root := t.TempDir()
	writeModule(t, root, "a", "[hooks]\npre = [\"a-pre\"]\npost = [\"a-post\"]\n")
	writeModule(t, root, "b", "[hooks]\npre = [\"b-pre-1\", \"b-pre-2\"]\n")

	plan, err := loadAndResolve(t, root, &facts.Facts{Hostname: "h", Username: "u"})
	require.NoError(t, err)

	require.Equal(t, []profile.HookCommand{
		{Command: "a-pre", Module: "a"},
		{Command: "b-pre-1", Module: "b"},
		{Command: "b-pre-2", Module: "b"},
	}, plan.Hooks.Pre, "pre hooks must aggregate in module selection order and carry the module name")
	require.Equal(t, []profile.HookCommand{{Command: "a-post", Module: "a"}}, plan.Hooks.Post,
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

	require.Equal(t, []profile.HookCommand{{Command: "only-pre", Module: "mod"}}, plan.Hooks.Pre)
	require.Empty(t, plan.Hooks.Post)
}
