package state_test

import (
	"fmt"
	"path/filepath"
	"sync"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/thedataflows/dotdrift/internal/state"
)

func TestFileStore_saveLoadWithLock(t *testing.T) {
	dir := t.TempDir()
	fs := state.NewFileStore(filepath.Join(dir, "state.json"))

	s := state.New()
	s.Selection = "abc"
	s.MarkComplete("packages")
	require.NoError(t, fs.Save(s))

	loaded, err := fs.Load()
	require.NoError(t, err)
	require.Equal(t, "abc", loaded.Selection)
	require.True(t, loaded.IsCompleted("packages"))
}

func TestFileStore_concurrentSaveLoad(t *testing.T) {
	dir := t.TempDir()
	fs := state.NewFileStore(filepath.Join(dir, "state.json"))

	var wg sync.WaitGroup
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func(n int) {
			defer wg.Done()
			for j := 0; j < 20; j++ {
				s := state.New()
				s.Selection = fmt.Sprintf("worker-%d-%d", n, j)
				require.NoError(t, fs.Save(s))
				_, err := fs.Load()
				require.NoError(t, err)
			}
		}(i)
	}
	wg.Wait()
}
