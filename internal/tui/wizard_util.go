package tui

import (
	"errors"
	"fmt"
	"maps"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"github.com/BurntSushi/toml"
	"github.com/charmbracelet/huh"
	"github.com/thedataflows/dotdrift/internal/generate"
	"github.com/thedataflows/dotdrift/internal/profile"
)

// Small helpers shared by the wizard flows.

// confirm runs a single yes/no question.
func confirm(title string, def bool) (bool, error) {
	v := def
	if err := huh.NewForm(huh.NewGroup(
		huh.NewConfirm().Title(title).Value(&v),
	)).Run(); err != nil {
		return false, err
	}
	return v, nil
}

// nonEmpty validates that a required input is not blank.
func nonEmpty(what string) func(string) error {
	return func(s string) error {
		if strings.TrimSpace(s) == "" {
			return fmt.Errorf("%s is required", what)
		}
		return nil
	}
}

// friendlyAbort maps a user abort to a clean, silent exit.
func friendlyAbort(err error) error {
	if errors.Is(err, huh.ErrUserAborted) {
		fmt.Fprintln(os.Stderr, "aborted; nothing written")
		return nil
	}
	return err
}

// existingSources returns the mount sources the target module already
// manages (for the MANAGED volume marking); any failure yields nil.
func existingSources(root string, sel generate.Selection) []string {
	dir, err := generate.ModuleDir(root, sel)
	if err != nil {
		return nil
	}
	var cfg profile.ModuleConfig
	if _, err := toml.DecodeFile(filepath.Join(dir, "module.toml"), &cfg); err != nil {
		return nil
	}
	sources := make([]string, 0, len(cfg.Mounts))
	for _, name := range slices.Sorted(maps.Keys(cfg.Mounts)) {
		sources = append(sources, cfg.Mounts[name].Source)
	}
	return sources
}

// printSummary lists the materialized module dir and its files.
func printSummary(root string, sel generate.Selection) error {
	dir, err := generate.ModuleDir(root, sel)
	if err != nil {
		return err
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		return fmt.Errorf("read module dir %s: %w", dir, err)
	}
	fmt.Fprintf(os.Stderr, "wrote %s:\n", dir)
	for _, e := range entries {
		if !e.IsDir() {
			fmt.Fprintf(os.Stderr, "  %s\n", e.Name())
		}
	}
	return nil
}
