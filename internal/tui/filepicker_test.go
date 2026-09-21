package tui

// T-tui-typed-inputs (0094): the file-picker modal's unit tests. The
// picker is the desktop-style path input: a directory listing (dirs
// first, case-insensitive, symlinks resolved), arrows/pgup/pgdn/
// home/end navigation, / filtering the current listing, a ctrl+l
// location bar for typed or pasted paths, and per-mode enter semantics
// (dirs mode picks directories, files mode descends into them, either
// does both). esc backs out of filter/location first, then closes.

import (
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/stretchr/testify/require"
)

// pickerFixture builds a listing with every shape the picker meets:
// dirs in mixed case, files, a hidden file, a dir symlink, and a broken
// symlink.
func pickerFixture(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	require.NoError(t, os.MkdirAll(filepath.Join(root, "alpha", "nested"), 0o755))
	require.NoError(t, os.MkdirAll(filepath.Join(root, "Beta"), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(root, "gamma.txt"), []byte("x"), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(root, "delta.sh"), []byte("x"), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(root, ".hidden"), []byte("x"), 0o644))
	require.NoError(t, os.Symlink("alpha", filepath.Join(root, "linkdir")))
	require.NoError(t, os.Symlink("nonexistent", filepath.Join(root, "broken")))
	return root
}

func pkey(p *filePicker, s string) { p.update(keyPress(s)) }

// ptype sends each rune as a key with Text set — what a real terminal
// delivers while a text input is active.
func ptype(p *filePicker, s string) {
	for _, r := range s {
		p.update(tea.KeyPressMsg{Code: r, Text: string(r)})
	}
}

func pickNames(entries []pickEntry) []string {
	var out []string
	for _, e := range entries {
		out = append(out, e.name)
	}
	return out
}

func TestPicker_dirsFirstCaseInsensitive(t *testing.T) {
	root := pickerFixture(t)
	p := newFilePicker(newTheme(true), kindEither, root, func(string) string { return "" })
	require.Equal(t, root, p.cwd)
	require.Equal(t,
		[]string{"alpha", "Beta", "linkdir", "broken", "delta.sh", "gamma.txt"},
		pickNames(p.visible()),
		"dirs first, then files, case-insensitive; symlinks resolve (linkdir is a dir, broken is not); hidden stays out",
	)
	for _, e := range p.visible()[:3] {
		require.True(t, e.dir, "%s resolves as a directory", e.name)
	}
}

func TestPicker_dirsModeListsDirsOnly(t *testing.T) {
	root := pickerFixture(t)
	p := newFilePicker(newTheme(true), kindDirs, root, func(string) string { return "" })
	require.Equal(t, []string{"alpha", "Beta", "linkdir"}, pickNames(p.visible()),
		"a directory pick lists directories only — a file is never a valid answer")
}

func TestPicker_hiddenToggle(t *testing.T) {
	root := pickerFixture(t)
	p := newFilePicker(newTheme(true), kindEither, root, func(string) string { return "" })
	require.NotContains(t, pickNames(p.visible()), ".hidden")
	pkey(p, ".")
	require.True(t, p.showHidden)
	require.Contains(t, pickNames(p.visible()), ".hidden", ". reveals dotfiles")
	pkey(p, ".")
	require.NotContains(t, pickNames(p.visible()), ".hidden", ". again hides them")
}

func TestPicker_filterNarrowsAndEscClears(t *testing.T) {
	root := pickerFixture(t)
	p := newFilePicker(newTheme(true), kindEither, root, func(string) string { return "" })
	pkey(p, "/")
	require.True(t, p.filtering)
	ptype(p, "ga")
	require.Equal(t, []string{"gamma.txt"}, pickNames(p.visible()), "the filter narrows the current listing")
	require.True(t, p.onEsc(), "esc in the filter is handled inside the picker — it must not close the modal")
	require.False(t, p.filtering)
	require.Empty(t, string(p.filter), "esc clears the query")
	require.Len(t, p.visible(), 6, "the full listing is back")
}

func TestPicker_filterKeepsSelectionInBounds(t *testing.T) {
	root := pickerFixture(t)
	p := newFilePicker(newTheme(true), kindEither, root, func(string) string { return "" })
	p.sel = 5 // gamma.txt
	pkey(p, "/")
	ptype(p, "al") // alpha only
	require.Less(t, p.sel, len(p.visible()), "a narrowing filter clamps the selection")
}

func TestPicker_enterSelectsDirInDirsMode(t *testing.T) {
	root := pickerFixture(t)
	p := newFilePicker(newTheme(true), kindDirs, root, func(string) string { return "" })
	pkey(p, "enter") // alpha is first
	require.True(t, p.finished())
	require.Equal(t, filepath.Join(root, "alpha"), p.picked, "enter on a directory picks it in dirs mode")
}

func TestPicker_enterDescendsDirInFilesMode(t *testing.T) {
	root := pickerFixture(t)
	p := newFilePicker(newTheme(true), kindFiles, root, func(string) string { return "" })
	pkey(p, "enter") // alpha is first: a dir descends, never picks
	require.False(t, p.finished())
	require.Equal(t, filepath.Join(root, "alpha"), p.cwd)
	require.Equal(t, []string{"nested"}, pickNames(p.visible()))
	// A file picks.
	pkey(p, "left") // back to root
	p.sel = 4       // delta.sh (dirs: alpha Beta linkdir, then broken, delta.sh)
	require.Equal(t, "delta.sh", p.visible()[p.sel].name)
	pkey(p, "enter")
	require.True(t, p.finished())
	require.Equal(t, filepath.Join(root, "delta.sh"), p.picked)
}

func TestPicker_eitherModeAndCtrlEnter(t *testing.T) {
	root := pickerFixture(t)
	p := newFilePicker(newTheme(true), kindEither, root, func(string) string { return "" })
	pkey(p, "enter") // a dir descends in either mode
	require.False(t, p.finished())
	require.Equal(t, filepath.Join(root, "alpha"), p.cwd)
	pkey(p, "ctrl+enter") // ctrl+enter picks the directory being shown
	require.True(t, p.finished())
	require.Equal(t, filepath.Join(root, "alpha"), p.picked)
}

func TestPicker_navigation(t *testing.T) {
	root := pickerFixture(t)
	p := newFilePicker(newTheme(true), kindEither, root, func(string) string { return "" })

	pkey(p, "right") // descend into alpha
	require.Equal(t, filepath.Join(root, "alpha"), p.cwd)
	pkey(p, "left") // parent
	require.Equal(t, root, p.cwd)
	pkey(p, "l")
	require.Equal(t, filepath.Join(root, "alpha"), p.cwd)
	pkey(p, "backspace") // parent aliases left
	require.Equal(t, root, p.cwd)

	pkey(p, "~")
	home, _ := os.UserHomeDir()
	require.Equal(t, home, p.cwd, "~ jumps home")
}

func TestPicker_pagingAndEnds(t *testing.T) {
	root := t.TempDir()
	for i := 0; i < 30; i++ {
		name := filepath.Join(root, "f"+string(rune('a'+i%26))+string(rune('0'+i/26)))
		require.NoError(t, os.WriteFile(name, []byte("x"), 0o644))
	}
	p := newFilePicker(newTheme(true), kindFiles, root, func(string) string { return "" })
	p.pageH = 10
	pkey(p, "end")
	require.Equal(t, len(p.visible())-1, p.sel)
	pkey(p, "home")
	require.Equal(t, 0, p.sel)
	pkey(p, "pgdown")
	require.Equal(t, 10, p.sel, "pgdn steps one page")
	pkey(p, "pgup")
	require.Equal(t, 0, p.sel)
	pkey(p, "j")
	require.Equal(t, 1, p.sel)
	pkey(p, "k")
	require.Equal(t, 0, p.sel)
	pkey(p, "k") // clamps at the top
	require.Equal(t, 0, p.sel)
}

func TestPicker_locationBarPicksFile(t *testing.T) {
	root := pickerFixture(t)
	p := newFilePicker(newTheme(true), kindFiles, root, func(string) string { return "" })
	pkey(p, "ctrl+l")
	require.True(t, p.locating, "ctrl+l opens the location bar")
	pkey(p, "ctrl+u") // a real terminal pastes over: clear the seeded cwd
	ptype(p, filepath.Join(root, "gamma.txt"))
	pkey(p, "enter")
	require.True(t, p.finished())
	require.Equal(t, filepath.Join(root, "gamma.txt"), p.picked)
}

func TestPicker_locationBarCdsIntoDir(t *testing.T) {
	root := pickerFixture(t)
	p := newFilePicker(newTheme(true), kindEither, root, func(string) string { return "" })
	pkey(p, "ctrl+l")
	pkey(p, "ctrl+u")
	ptype(p, filepath.Join(root, "alpha"))
	pkey(p, "enter")
	require.False(t, p.finished(), "a directory in the location bar navigates, not picks")
	require.Equal(t, filepath.Join(root, "alpha"), p.cwd)
	require.False(t, p.locating)
}

func TestPicker_locationBarAcceptsNewFileWhereAllowed(t *testing.T) {
	root := pickerFixture(t)
	p := newFilePicker(newTheme(true), kindFiles, root, func(string) string { return "" })
	pkey(p, "ctrl+l")
	pkey(p, "ctrl+u")
	fresh := filepath.Join(root, "alpha", "newfile.conf")
	ptype(p, fresh)
	pkey(p, "enter")
	require.True(t, p.finished(), "a not-yet-existing file path is a valid pick where the field creates files")
	require.Equal(t, fresh, p.picked)
}

func TestPicker_locationBarRefusesBadPaths(t *testing.T) {
	root := pickerFixture(t)
	p := newFilePicker(newTheme(true), kindDirs, root, func(string) string { return "" })

	pkey(p, "ctrl+l")
	pkey(p, "ctrl+u")
	ptype(p, filepath.Join(root, "gamma.txt")) // a file, but the mode wants a dir
	pkey(p, "enter")
	require.False(t, p.finished())
	require.NotEmpty(t, p.err, "picking a file in dirs mode names the problem")

	pkey(p, "ctrl+u")
	ptype(p, filepath.Join(root, "no", "such", "place"))
	pkey(p, "enter")
	require.False(t, p.finished())
	require.NotEmpty(t, p.err, "a typo in the path's spine names the problem")
	require.True(t, p.locating, "the bar stays open on a refusal")
}

func TestPicker_locationBarAcceptsNewDirWithExistingParent(t *testing.T) {
	root := pickerFixture(t)
	p := newFilePicker(newTheme(true), kindDirs, root, func(string) string { return "" })
	pkey(p, "ctrl+l")
	pkey(p, "ctrl+u")
	fresh := filepath.Join(root, "newdir")
	ptype(p, fresh)
	pkey(p, "enter")
	require.True(t, p.finished(),
		"a typed directory that does not exist yet is a valid pick (mountpoints, share dirs) when its parent exists")
	require.Equal(t, fresh, p.picked)
}

func TestPicker_locationBarExpandsTilde(t *testing.T) {
	root := pickerFixture(t)
	p := newFilePicker(newTheme(true), kindEither, root, func(string) string { return "" })
	home, _ := os.UserHomeDir()
	pkey(p, "ctrl+l")
	pkey(p, "ctrl+u")
	ptype(p, "~")
	pkey(p, "enter")
	require.Equal(t, home, p.cwd, "~ expands to the home directory")
}

func TestPicker_seedResolution(t *testing.T) {
	root := pickerFixture(t)

	p := newFilePicker(newTheme(true), kindEither, filepath.Join(root, "gamma.txt"), func(string) string { return "" })
	require.Equal(t, root, p.cwd, "a file seed opens its directory")

	p = newFilePicker(newTheme(true), kindEither, filepath.Join(root, "alpha"), func(string) string { return "" })
	require.Equal(t, filepath.Join(root, "alpha"), p.cwd, "a directory seed opens it")

	p = newFilePicker(newTheme(true), kindEither, filepath.Join(root, "alpha", "no", "deeper"), func(string) string { return "" })
	require.Equal(t, filepath.Join(root, "alpha"), p.cwd, "a missing seed climbs to the nearest existing ancestor")

	p = newFilePicker(newTheme(true), kindEither, "", func(string) string { return "" })
	home, _ := os.UserHomeDir()
	require.Equal(t, home, p.cwd, "an empty seed opens the home directory")

	p = newFilePicker(newTheme(true), kindEither, "~/", func(string) string { return "" })
	require.Equal(t, home, p.cwd, "a tilde seed expands")
}

func TestPicker_commitRefusalStaysOpen(t *testing.T) {
	root := pickerFixture(t)
	p := newFilePicker(newTheme(true), kindEither, root, func(string) string { return "field says no" })
	pkey(p, "ctrl+enter")
	require.False(t, p.finished(), "a refused pick keeps the picker open")
	require.Equal(t, "field says no", p.err)
}

func TestPicker_unreadableDirNamesTheProblem(t *testing.T) {
	if runtime.GOOS == "windows" || os.Geteuid() == 0 {
		t.Skip("chmod 0 does not block root or windows")
	}
	root := pickerFixture(t)
	locked := filepath.Join(root, "Beta")
	require.NoError(t, os.Chmod(locked, 0o000))
	t.Cleanup(func() { _ = os.Chmod(locked, 0o755) })
	p := newFilePicker(newTheme(true), kindEither, root, func(string) string { return "" })
	p.sel = 1 // Beta
	pkey(p, "right")
	require.Equal(t, root, p.cwd, "an unreadable directory is not entered")
	require.NotEmpty(t, p.err, "the failure is named inline")
}

func TestPicker_ownsTextOnlyWhileTyping(t *testing.T) {
	root := pickerFixture(t)
	p := newFilePicker(newTheme(true), kindEither, root, func(string) string { return "" })
	require.False(t, p.ownsText(), "plain browsing leaves ? to the help modal")
	pkey(p, "/")
	require.True(t, p.ownsText(), "the filter owns text (? is a query character)")
	pkey(p, "esc")
	pkey(p, "ctrl+l")
	require.True(t, p.ownsText(), "the location bar owns text")
}

func TestPicker_pasteIntoLocationBar(t *testing.T) {
	root := pickerFixture(t)
	p := newFilePicker(newTheme(true), kindFiles, root, func(string) string { return "" })
	pkey(p, "ctrl+l")
	pkey(p, "ctrl+u")
	p.update(tea.PasteMsg{Content: filepath.Join(root, "delta.sh") + "\n"})
	pkey(p, "enter")
	require.True(t, p.finished(), "0091: paste is a message, and it lands in the location bar")
	require.Equal(t, filepath.Join(root, "delta.sh"), p.picked)
}

func TestPicker_mouseWheelScrolls(t *testing.T) {
	root := pickerFixture(t)
	p := newFilePicker(newTheme(true), kindEither, root, func(string) string { return "" })
	p.update(tea.MouseWheelMsg{Button: tea.MouseWheelDown})
	require.Equal(t, 1, p.sel)
	p.update(tea.MouseWheelMsg{Button: tea.MouseWheelUp})
	require.Equal(t, 0, p.sel)
}

func TestPicker_viewNamesCwdAndEntries(t *testing.T) {
	root := pickerFixture(t)
	p := newFilePicker(newTheme(true), kindDirs, root, func(string) string { return "" })
	plain := ansiRe.ReplaceAllString(p.view(100, 30), "")
	require.Contains(t, plain, root, "the current directory is named")
	require.Contains(t, plain, "alpha/", "directories render with a trailing slash")
	require.NotContains(t, plain, "gamma.txt", "dirs mode shows no files")
}

func TestPicker_tildeSeedSortsStable(t *testing.T) {
	root := pickerFixture(t)
	p := newFilePicker(newTheme(true), kindEither, root, func(string) string { return "" })
	first := pickNames(p.visible())
	pkey(p, ".") // hidden on
	pkey(p, ".") // and off again
	require.Equal(t, first, pickNames(p.visible()), "toggles never reorder the listing")
	ciLess := func(a, b string) int { return strings.Compare(strings.ToLower(a), strings.ToLower(b)) }
	require.True(t, slices.IsSortedFunc(first[:3], ciLess), "dirs sort case-insensitively")
	require.True(t, slices.IsSortedFunc(first[3:], ciLess), "files sort case-insensitively")
}
