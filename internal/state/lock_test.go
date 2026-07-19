package state_test

import (
	"fmt"
	"path/filepath"
	"sync"
	"testing"
	"time"

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

func TestFileStore_sidecarLockMutualExclusion(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "state.json")
	fs1 := state.NewFileStore(path)
	fs2 := state.NewFileStore(path)

	require.Equal(t, path+".lock", fs1.LockPath())

	require.NoError(t, fs1.Lock())

	ok, err := fs2.TryLock()
	require.NoError(t, err)
	require.False(t, ok, "second opener must not acquire the sidecar lock while held")

	require.NoError(t, fs1.Unlock())

	ok, err = fs2.TryLock()
	require.NoError(t, err)
	require.True(t, ok, "second opener should acquire the sidecar lock after release")
	require.NoError(t, fs2.Unlock())
}

func TestFileStore_lockSurvivesRename(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "state.json")
	fs1 := state.NewFileStore(path)
	require.NoError(t, fs1.Lock())

	// Save replaces state.json via tmp+rename; the lock must stay effective
	// because it lives on the sidecar, not the renamed inode.
	require.NoError(t, fs1.Save(state.New()))

	fs2 := state.NewFileStore(path)
	ok, err := fs2.TryLock()
	require.NoError(t, err)
	require.False(t, ok, "sidecar lock must survive atomic rename of the state file")

	require.NoError(t, fs1.Unlock())
	ok, err = fs2.TryLock()
	require.NoError(t, err)
	require.True(t, ok)
	require.NoError(t, fs2.Unlock())
}

func TestFileStore_lockBlocksUntilUnlock(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "state.json")
	fs1 := state.NewFileStore(path)
	require.NoError(t, fs1.Lock())

	fs2 := state.NewFileStore(path)
	acquired := make(chan error, 1)
	go func() { acquired <- fs2.Lock() }()

	select {
	case err := <-acquired:
		t.Fatalf("second Lock should block while first is held, got %v", err)
	case <-time.After(100 * time.Millisecond):
		// still blocked: correct
	}

	require.NoError(t, fs1.Unlock())
	require.NoError(t, <-acquired, "second Lock should unblock after first Unlock")
	require.NoError(t, fs2.Unlock())
}
