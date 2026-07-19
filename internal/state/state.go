// Package state persists resume-only state for dotdrift apply.
package state

import (
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"syscall"
)

// Status values for the apply pipeline.
const (
	StatusFresh      = "fresh"
	StatusInProgress = "in-progress"
	StatusComplete   = "complete"
	StatusFailed     = "failed"
)

// State holds the resume cursor and last error.
type State struct {
	Selection string          `json:"selection"`
	Completed map[string]bool `json:"completed"`
	Current   string          `json:"current"`
	Status    string          `json:"status"`
	Error     string          `json:"error"`
}

// New returns a fresh state with initialized maps.
func New() *State {
	return &State{
		Completed: make(map[string]bool),
		Status:    StatusFresh,
	}
}

// IsCompleted reports whether a step has been completed.
func (s *State) IsCompleted(step string) bool {
	if s.Completed == nil {
		return false
	}
	return s.Completed[step]
}

// MarkComplete records a step as completed and clears the current step/error.
func (s *State) MarkComplete(step string) {
	if s.Completed == nil {
		s.Completed = make(map[string]bool)
	}
	s.Completed[step] = true
	s.Current = ""
	s.Error = ""
	s.Status = StatusInProgress
}

// MarkFailed records a step as the current failed step and stores the error.
func (s *State) MarkFailed(step string, err error) {
	s.Current = step
	s.Status = StatusFailed
	if err != nil {
		s.Error = err.Error()
	} else {
		s.Error = ""
	}
}

// MarkCompletePipeline marks the pipeline as fully complete.
func (s *State) MarkCompletePipeline() {
	s.Current = ""
	s.Error = ""
	s.Status = StatusComplete
}

// ResetForSelection clears runtime progress when the selection fingerprint changes.
func (s *State) ResetForSelection() {
	s.Completed = make(map[string]bool)
	s.Current = ""
	s.Error = ""
	s.Status = StatusFresh
}

// Store persists state.
type Store interface {
	Load() (*State, error)
	Save(s *State) error
}

// FileStore saves state to a JSON file.
//
// Concurrency: mutual exclusion between processes is provided by an
// flock(LOCK_EX) on the sidecar file <Path>.lock (see Lock), never on the
// state file itself. Save replaces the state file via tmp+rename, so a lock
// taken on the state file's inode would stay behind on the unlinked inode
// while later openers lock the new inode — the sidecar path is stable across
// renames. Load and Save do NOT lock internally; callers that need a
// load→modify→save critical section must hold Lock across it (cmd/apply
// does). Lock-free readers are still safe: rename is atomic, so Load never
// observes a torn write.
type FileStore struct {
	Path string

	lockFile *os.File
}

// DefaultPath returns the default state file path when no profile is known.
// Prefer ProfileStatePath for normal CLI use; DefaultPath is a fallback for
// direct API callers that do not have a profile root. It returns an error
// when neither XDG_STATE_HOME nor a user home directory is available so the
// empty string never flows into MkdirAll.
func DefaultPath() (string, error) {
	root := os.Getenv("XDG_STATE_HOME")
	if root == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			return "", fmt.Errorf("state: XDG_STATE_HOME unset and home directory unavailable: %w", err)
		}
		if home == "" {
			return "", errors.New("state: XDG_STATE_HOME unset and home directory is empty")
		}
		root = filepath.Join(home, ".local", "state")
	}
	return filepath.Join(root, "dotdrift", "state.json"), nil
}

// NewFileStore returns a FileStore using the given path. An empty path falls
// back to DefaultPath; if no default can be determined the store's Path stays
// empty and Load/Save/Lock return an explicit error.
func NewFileStore(path string) *FileStore {
	if path == "" {
		path, _ = DefaultPath()
	}
	return &FileStore{Path: path}
}

var errNoPath = errors.New("state: no state path; set XDG_STATE_HOME or HOME")

// LockPath returns the sidecar lock file path. The sidecar is never renamed,
// so a lock held on it survives Save's atomic tmp+rename of the state file.
func (fs *FileStore) LockPath() string { return fs.Path + ".lock" }

// Lock acquires the exclusive sidecar lock, blocking until it is available.
// Hold it across the entire load→pipeline→save window; it is idempotent on a
// FileStore that already holds it.
func (fs *FileStore) Lock() error {
	if fs.lockFile != nil {
		return nil
	}
	f, err := fs.openLock()
	if err != nil {
		return err
	}
	if err := syscall.Flock(int(f.Fd()), syscall.LOCK_EX); err != nil {
		_ = f.Close()
		return fmt.Errorf("lock state: %w", err)
	}
	fs.lockFile = f
	return nil
}

// TryLock attempts the exclusive sidecar lock without blocking and reports
// whether it was acquired.
func (fs *FileStore) TryLock() (bool, error) {
	if fs.lockFile != nil {
		return true, nil
	}
	f, err := fs.openLock()
	if err != nil {
		return false, err
	}
	if err := syscall.Flock(int(f.Fd()), syscall.LOCK_EX|syscall.LOCK_NB); err != nil {
		_ = f.Close()
		if errors.Is(err, syscall.EWOULDBLOCK) {
			return false, nil
		}
		return false, fmt.Errorf("try lock state: %w", err)
	}
	fs.lockFile = f
	return true, nil
}

// Unlock releases the sidecar lock. It is a no-op when not held.
func (fs *FileStore) Unlock() error {
	if fs.lockFile == nil {
		return nil
	}
	f := fs.lockFile
	fs.lockFile = nil
	_ = syscall.Flock(int(f.Fd()), syscall.LOCK_UN)
	if err := f.Close(); err != nil {
		return fmt.Errorf("close state lock: %w", err)
	}
	return nil
}

func (fs *FileStore) openLock() (*os.File, error) {
	if fs.Path == "" {
		return nil, errNoPath
	}
	if err := os.MkdirAll(filepath.Dir(fs.Path), 0o755); err != nil {
		return nil, fmt.Errorf("create state dir: %w", err)
	}
	f, err := os.OpenFile(fs.LockPath(), os.O_RDWR|os.O_CREATE, 0o600)
	if err != nil {
		return nil, fmt.Errorf("open state lock: %w", err)
	}
	return f, nil
}

// Load reads the state file, returning a fresh state if it does not exist.
func (fs *FileStore) Load() (*State, error) {
	if fs.Path == "" {
		return nil, errNoPath
	}
	f, err := os.Open(fs.Path)
	if err != nil {
		if os.IsNotExist(err) {
			return New(), nil
		}
		return nil, fmt.Errorf("open state: %w", err)
	}
	defer f.Close()

	var s State
	if err := json.NewDecoder(f).Decode(&s); err != nil {
		return nil, fmt.Errorf("decode state: %w (remove or move aside %s to start fresh)", err, fs.Path)
	}
	if s.Completed == nil {
		s.Completed = make(map[string]bool)
	}
	if s.Status == "" {
		s.Status = StatusFresh
	}
	return &s, nil
}

// Save writes the state file atomically (tmp file, fsync, rename).
func (fs *FileStore) Save(s *State) error {
	if fs.Path == "" {
		return errNoPath
	}
	if err := os.MkdirAll(filepath.Dir(fs.Path), 0o755); err != nil {
		return fmt.Errorf("create state dir: %w", err)
	}

	data, err := json.MarshalIndent(s, "", "  ")
	if err != nil {
		return fmt.Errorf("encode state: %w", err)
	}

	tmp, err := os.CreateTemp(filepath.Dir(fs.Path), filepath.Base(fs.Path)+".tmp.")
	if err != nil {
		return fmt.Errorf("create state tmp: %w", err)
	}
	if _, err := tmp.Write(data); err != nil {
		_ = tmp.Close()
		_ = os.Remove(tmp.Name())
		return fmt.Errorf("write state tmp: %w", err)
	}
	if err := tmp.Sync(); err != nil {
		_ = tmp.Close()
		_ = os.Remove(tmp.Name())
		return fmt.Errorf("sync state tmp: %w", err)
	}
	if err := tmp.Close(); err != nil {
		_ = os.Remove(tmp.Name())
		return fmt.Errorf("close state tmp: %w", err)
	}
	if err := os.Rename(tmp.Name(), fs.Path); err != nil {
		_ = os.Remove(tmp.Name())
		return fmt.Errorf("rename state tmp: %w", err)
	}
	return nil
}

// ProfileStatePath returns the default state file path for a profile root.
// The path is located under the XDG state directory so the profile directory
// is not polluted with runtime state.
func ProfileStatePath(profileRoot string) string {
	root := os.Getenv("XDG_STATE_HOME")
	if root == "" {
		home, err := os.UserHomeDir()
		if err != nil || home == "" {
			root = "."
		} else {
			root = filepath.Join(home, ".local", "state")
		}
	}
	if profileRoot == "" {
		profileRoot = "."
	}
	abs, err := filepath.Abs(profileRoot)
	if err != nil {
		abs = profileRoot
	}
	// Canonicalize so a profile reached via different symlinks hashes to one
	// state file.
	if resolved, err := filepath.EvalSymlinks(abs); err == nil {
		abs = resolved
	}
	id := profileHash(abs)
	return filepath.Join(root, "dotdrift", "profiles", id, "state.json")
}

// profileHash returns a stable identifier for a profile path.
func profileHash(path string) string {
	h := sha256.Sum256([]byte(path))
	return fmt.Sprintf("%x", h)
}
