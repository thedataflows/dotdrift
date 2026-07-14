package dotdrift

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

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
	fmt.Printf("Initialized profile at %s\n", c.Path)
	return nil
}

func (c *InitCmd) clone() error {
	// Infer directory name from URL unless the current path is explicitly provided.
	dir := filepath.Base(c.Path)
	if dir == "" || dir == "." {
		return fmt.Errorf("cannot infer clone directory from %s", c.Path)
	}
	cmd := exec.Command("git", "clone", c.Path, dir)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("clone profile: %w", err)
	}
	fmt.Printf("Cloned profile from %s into %s\n", c.Path, dir)
	return nil
}

func isGitURL(s string) bool {
	return strings.HasPrefix(s, "http://") ||
		strings.HasPrefix(s, "https://") ||
		strings.HasPrefix(s, "git@") ||
		strings.HasPrefix(s, "file://")
}
