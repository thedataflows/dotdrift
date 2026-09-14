package service

// The raw-candidate save path (M15, T-tui-editing): a broken module.toml
// cannot be family-spliced, so the raw-text repair mode sends the whole
// repaired file as the candidate. The splice step swaps out; every other
// stage of the pipeline — strict decode, when-grammar, resolve checks,
// disk-hash recheck, atomic write — guards it unchanged.

import (
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestWriteModuleLayer_rawCandidateRepairsBrokenFile(t *testing.T) {
	root := t.TempDir()
	dir := filepath.Join(root, "modules", "demo")
	require.NoError(t, os.MkdirAll(dir, 0o755))
	broken := "id = \"demo\"\n[broken\n"
	require.NoError(t, os.WriteFile(filepath.Join(dir, "module.toml"), []byte(broken), 0o644))

	area := configArea(root)
	repaired := "id = \"demo\"\ndescription = \"repaired\"\n"
	res, err := area.WriteModuleLayer(SaveRequest{
		Dir:      dir,
		BaseHash: RawHash([]byte(broken)),
		Raw:      &repaired,
	})
	require.NoError(t, err, "a raw candidate that decodes saves through the full pipeline")
	raw, err := os.ReadFile(filepath.Join(dir, "module.toml"))
	require.NoError(t, err)
	require.Equal(t, repaired, string(raw))
	require.Equal(t, RawHash([]byte(repaired)), res.Hash)
}

func TestWriteModuleLayer_rawCandidateStillRefusesConflicts(t *testing.T) {
	root := t.TempDir()
	dir := filepath.Join(root, "modules", "demo")
	require.NoError(t, os.MkdirAll(dir, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "module.toml"), []byte("id = \"demo\"\n[broken\n"), 0o644))

	repaired := "id = \"demo\"\n"
	_, err := configArea(root).WriteModuleLayer(SaveRequest{
		Dir:      dir,
		BaseHash: RawHash([]byte("something else")),
		Raw:      &repaired,
	})
	var conflict *DiskHashConflictError
	require.True(t, errors.As(err, &conflict), "the hash guard applies to raw saves too, got: %v", err)
}

func TestWriteModuleLayer_rawCandidateStillDecodesStrictly(t *testing.T) {
	root := t.TempDir()
	dir := filepath.Join(root, "modules", "demo")
	require.NoError(t, os.MkdirAll(dir, 0o755))
	broken := "id = \"demo\"\n[broken\n"
	require.NoError(t, os.WriteFile(filepath.Join(dir, "module.toml"), []byte(broken), 0o644))

	stillBroken := "id = \"demo\"\n[alsbroken\n"
	_, err := configArea(root).WriteModuleLayer(SaveRequest{
		Dir:      dir,
		BaseHash: RawHash([]byte(broken)),
		Raw:      &stillBroken,
	})
	require.Error(t, err, "a raw candidate that does not decode is refused")
	var schema *SchemaError
	require.True(t, errors.As(err, &schema), "the refusal is the strict decode, got: %v", err)
}
