package dotdrift

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/rs/zerolog/log"
)

// runGit executes git in dir; swapped out by tests.
var runGit = func(dir string, args ...string) error {
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

// InitCmd creates or clones a profile.
type InitCmd struct {
	Path string `arg:"" help:"Local path or git URL for the profile" default:"."`
}

// Run creates a new profile directory or clones a git URL.
func (c *InitCmd) Run() error {
	if c.Path == "" {
		c.Path = "."
	}

	if isGitURL(c.Path) {
		return c.clone()
	}

	return c.create()
}

func (c *InitCmd) create() error {
	if err := os.MkdirAll(c.Path, 0o755); err != nil {
		return fmt.Errorf("create profile directory: %w", err)
	}
	tomlPath := filepath.Join(c.Path, "dotdrift.toml")
	if _, err := os.Stat(tomlPath); err == nil {
		return fmt.Errorf("profile already exists at %s", c.Path)
	}
	content := "[modules]\n"
	if err := os.WriteFile(tomlPath, []byte(content), 0o644); err != nil {
		return fmt.Errorf("write dotdrift.toml: %w", err)
	}
	for _, dir := range []string{"modules", "hosts", "users"} {
		if err := os.MkdirAll(filepath.Join(c.Path, dir), 0o755); err != nil {
			return fmt.Errorf("create %s directory: %w", dir, err)
		}
	}
	if err := runGit(c.Path, "init", "-q"); err != nil {
		log.Warn().Err(err).Str("path", c.Path).Msg("git init failed; profile created without git")
	}
	fmt.Printf("Initialized profile at %s\n", c.Path)
	return nil
}

func (c *InitCmd) clone() error {
	// Infer directory name from URL unless the current path is explicitly provided.
	dir := strings.TrimSuffix(filepath.Base(strings.TrimSuffix(c.Path, "/")), ".git")
	if dir == "" || dir == "." {
		return fmt.Errorf("cannot infer clone directory from %s", c.Path)
	}
	target, err := filepath.Abs(dir)
	if err != nil {
		return fmt.Errorf("resolve clone target: %w", err)
	}
	if err := runGit(filepath.Dir(target), "clone", c.Path, target); err != nil {
		return fmt.Errorf("clone profile: %w", err)
	}
	if _, err := os.Stat(filepath.Join(target, "dotdrift.toml")); err != nil {
		return fmt.Errorf("cloned repository at %s is not a dotdrift profile: missing dotdrift.toml", target)
	}
	fmt.Printf("Cloned profile from %s into %s\n", c.Path, target)
	return nil
}

func isGitURL(s string) bool {
	return strings.HasPrefix(s, "http://") ||
		strings.HasPrefix(s, "https://") ||
		strings.HasPrefix(s, "git@") ||
		strings.HasPrefix(s, "file://")
}
