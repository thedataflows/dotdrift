package tui

import (
	"errors"
	"fmt"
	"os"
	"strings"

	"github.com/charmbracelet/huh"
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
