// Package state persists resume-only state for dotdrift apply.
package state

import (
	"crypto/sha256"
	"encoding/json"
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
type FileStore struct {
	Path string
}

// DefaultPath returns the default state file path when no profile is known.
// Prefer ProfileStatePath for normal CLI use; DefaultPath is a fallback for
// direct API callers that do not have a profile root.
func DefaultPath() string {
	root := os.Getenv("XDG_STATE_HOME")
	if root == "" {
		home, err := os.UserHomeDir()
		if err != nil || home == "" {
			return ""
		}
		root = filepath.Join(home, ".local", "state")
	}
	return filepath.Join(root, "dotdrift", "state.json")
}

// NewFileStore returns a FileStore using the given path.

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
	id := profileHash(abs)
	return filepath.Join(root, "dotdrift", "profiles", id, "state.json")
}

// profileHash returns a stable identifier for a profile path.
func profileHash(path string) string {
	h := sha256.Sum256([]byte(path))
	return fmt.Sprintf("%x", h)
}
func NewFileStore(path string) *FileStore {
	if path == "" {
		path = DefaultPath()
	}
	return &FileStore{Path: path}
}

// Load reads the state file, returning a fresh state if it does not exist.
func (fs *FileStore) Load() (*State, error) {
	f, err := os.Open(fs.Path)
	if err != nil {
		if os.IsNotExist(err) {
			return New(), nil
		}
		return nil, fmt.Errorf("open state: %w", err)
	}
	defer f.Close()

	if err := syscall.Flock(int(f.Fd()), syscall.LOCK_SH); err != nil {
		return nil, fmt.Errorf("lock state for read: %w", err)
	}
	defer func() { _ = syscall.Flock(int(f.Fd()), syscall.LOCK_UN) }()

	var s State
	if err := json.NewDecoder(f).Decode(&s); err != nil {
		return nil, fmt.Errorf("decode state: %w", err)
	}
	if s.Completed == nil {
		s.Completed = make(map[string]bool)
	}
	if s.Status == "" {
		s.Status = StatusFresh
	}
	return &s, nil
}

// Save writes the state file atomically with an exclusive lock.
func (fs *FileStore) Save(s *State) error {
	if err := os.MkdirAll(filepath.Dir(fs.Path), 0o755); err != nil {
		return fmt.Errorf("create state dir: %w", err)
	}

	f, err := os.OpenFile(fs.Path, os.O_RDWR|os.O_CREATE, 0o600)
	if err != nil {
		return fmt.Errorf("open state for write: %w", err)
	}
	if err := syscall.Flock(int(f.Fd()), syscall.LOCK_EX); err != nil {
		_ = f.Close()
		return fmt.Errorf("lock state for write: %w", err)
	}
	defer func() { _ = syscall.Flock(int(f.Fd()), syscall.LOCK_UN) }()
	defer f.Close()

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

// Load reads state from the given path. If the file does not exist, it returns a default state.
func Load(path string) (State, error) {
	fs := NewFileStore(path)
	s, err := fs.Load()
	if err != nil {
		return State{}, err
	}
	return *s, nil
}

// Save writes state to the given path.
func Save(path string, s State) error {
	fs := NewFileStore(path)
	return fs.Save(&s)
}
