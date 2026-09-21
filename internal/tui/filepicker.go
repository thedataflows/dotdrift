package tui

// T-tui-typed-inputs (0094): the file-picker modal — the desktop-style
// path input for fields that name files or directories. A directory
// listing (dirs first, case-insensitive, symlinks resolved, dotfiles
// behind `.`), arrows/pgup/pgdn/home/end navigation, enter semantics
// per mode (dirs mode picks directories, files mode descends into them,
// either does both, ctrl+enter picks the shown directory), `/` filters
// the current listing with the nav filter's semantics, and a ctrl+l
// location bar takes typed or pasted paths — the escape hatch for
// device paths, tilde paths, and not-yet-existing files. esc backs out
// of the filter/location bar first (onEsc), then closes. The picker
// opens on the field's current value (0096): a valid seed shows its
// parent directory with its own entry selected, a missing seed climbs
// to the nearest existing ancestor. The listing's first row is ../
// whenever the shown directory has a parent (0097) — enter and right
// on it navigate up like left, never pick; the cursor rests on it via
// the -1 sentinel below the first real entry. A pick commits through
// the caller's callback; a refusal renders inside and keeps the
// picker open.

import (
	"os"
	"path/filepath"
	"slices"
	"strings"

	tea "charm.land/bubbletea/v2"
)

// editKind names a field's input type (0094). kindText is the inline
// single-line input (the zero value — every row edits this way unless
// marked); kindMulti opens the multi-line editor; the path kinds open
// the file picker with that pick mode.
type editKind int

const (
	kindText editKind = iota
	kindMulti
	kindDirs
	kindFiles
	kindEither
)

// pickEntry is one listing row: the base name and whether it is a
// directory (symlinks resolved; a broken symlink is a file).
type pickEntry struct {
	name string
	dir  bool
}

// filePicker is the path-picking modal. commit returns "" on success;
// anything else keeps the picker open with the error inside (the same
// contract as the add form's commit).
type filePicker struct {
	th         theme
	mode       editKind
	cwd        string
	entries    []pickEntry
	showHidden bool
	filter     []rune
	filtering  bool
	locating   bool
	loc        []rune
	sel        int
	off        int
	pageH      int // the last render's list height: the paging step
	err        string
	picked     string
	commit     func(string) string
	done       bool
}

// newFilePicker opens the picker on the seed (0096): an existing seed
// shows its parent directory with the seed's own entry selected — enter
// re-confirms the current value, the siblings are one keystroke away —
// a missing seed climbs to the nearest existing ancestor, and empty or
// unresolvable seeds open the home directory. A hidden seed reveals
// dotfiles so the listing can select it.
func newFilePicker(th theme, mode editKind, seed string, commit func(string) string) *filePicker {
	p := &filePicker{th: th, mode: mode, commit: commit, pageH: 10}
	dir, name := pickStart(seed)
	p.cwd = dir
	if strings.HasPrefix(name, ".") {
		p.showHidden = true
	}
	p.readDir()
	p.selectName(name)
	return p
}

func (p *filePicker) finished() bool { return p.done }

// ownsText keeps ? and other printable keys for the picker's own text
// inputs (the compositor would otherwise open help over the modal).
func (p *filePicker) ownsText() bool { return p.filtering || p.locating }

// onEsc is the compositor's esc hook: esc backs out of the location
// bar or clears the filter first; only a plain browsing esc closes.
func (p *filePicker) onEsc() bool {
	switch {
	case p.locating:
		p.locating = false
		p.err = ""
		return true
	case p.filtering:
		p.filtering = false
		p.filter = p.filter[:0]
		p.sel = 0
		return true
	}
	return false
}

// expandHome resolves a leading ~ against home.
func expandHome(s, home string) string {
	if s == "~" {
		return home
	}
	if strings.HasPrefix(s, "~/") {
		return filepath.Join(home, s[2:])
	}
	return s
}

// pickStart resolves where the picker opens and which entry to select
// (0096): an existing path opens its parent and selects its own name —
// the filesystem root opens itself, there is no parent to select in — a
// missing path climbs to the nearest existing ancestor and selects
// nothing, and an empty seed opens home.
func pickStart(seed string) (dir, name string) {
	home, _ := os.UserHomeDir()
	s := expandHome(strings.TrimSpace(seed), home)
	if s == "" {
		return home, ""
	}
	if _, err := os.Stat(s); err == nil {
		if parent := filepath.Dir(s); parent != s {
			return parent, filepath.Base(s)
		}
		return s, ""
	}
	for {
		parent := filepath.Dir(s)
		if parent == s {
			return home, ""
		}
		s = parent
		if st, err := os.Stat(s); err == nil && st.IsDir() {
			return s, ""
		}
	}
}

// selectName moves the cursor onto the named entry — the seeded path's
// own row. A name the listing does not show (filtered by the mode, or
// a missing seed) leaves the cursor at the top.
func (p *filePicker) selectName(name string) {
	if name == "" {
		return
	}
	for i, e := range p.visible() {
		if e.name == name {
			p.sel = i
			return
		}
	}
}

// readDir rebuilds the listing for the current directory (the
// constructor's seed read and the dotfile toggle).
func (p *filePicker) readDir() {
	entries, err := readPickDir(p.cwd, p.mode, p.showHidden)
	if err != nil {
		p.err = "cannot read: " + err.Error()
		p.entries = nil
		p.clampSel()
		return
	}
	p.entries = entries
	p.err = ""
	p.clampSel()
}

// readPickDir lists one directory: symlinks resolved via Stat, dotfiles
// behind the toggle, directories only in dirs mode, sorted
// directories-first case-insensitively.
func readPickDir(dir string, mode editKind, showHidden bool) ([]pickEntry, error) {
	des, err := os.ReadDir(dir)
	if err != nil {
		return nil, err
	}
	var entries []pickEntry
	for _, de := range des {
		name := de.Name()
		if !showHidden && strings.HasPrefix(name, ".") {
			continue
		}
		isDir := false
		if st, serr := os.Stat(filepath.Join(dir, name)); serr == nil && st.IsDir() {
			isDir = true
		}
		if mode == kindDirs && !isDir {
			continue
		}
		entries = append(entries, pickEntry{name: name, dir: isDir})
	}
	slices.SortFunc(entries, func(a, b pickEntry) int {
		if a.dir != b.dir {
			if a.dir {
				return -1
			}
			return 1
		}
		if c := strings.Compare(strings.ToLower(a.name), strings.ToLower(b.name)); c != 0 {
			return c
		}
		return strings.Compare(a.name, b.name)
	})
	return entries, nil
}

// visible is the listing after the filter: case-insensitive substring
// on the entry name.
func (p *filePicker) visible() []pickEntry {
	if len(p.filter) == 0 {
		return p.entries
	}
	needle := strings.ToLower(string(p.filter))
	var out []pickEntry
	for _, e := range p.entries {
		if strings.Contains(strings.ToLower(e.name), needle) {
			out = append(out, e)
		}
	}
	return out
}

// hasUp reports whether the listing shows the pinned ../ row (0097):
// every directory but the filesystem root has a parent.
func (p *filePicker) hasUp() bool { return filepath.Dir(p.cwd) != p.cwd }

// goUp navigates to the parent directory (left, and enter or right on
// the ../ row). At the root it is a no-op.
func (p *filePicker) goUp() {
	if parent := filepath.Dir(p.cwd); parent != p.cwd {
		p.cd(parent)
	}
}

func (p *filePicker) clampSel() {
	// The ../ row sits at -1 above the first real entry (0097): the
	// cursor's floor whenever the shown directory has a parent.
	floor := 0
	if p.hasUp() {
		floor = -1
	}
	n := len(p.visible())
	if n == 0 {
		p.sel = floor // an emptied listing's only row is ../
		return
	}
	p.sel = min(max(p.sel, floor), n-1)
}

// cd navigates into a directory and re-reads the listing. A directory
// that cannot be read is never entered — the failure names itself
// inline and the picker stays where it was.
func (p *filePicker) cd(dir string) {
	entries, err := readPickDir(dir, p.mode, p.showHidden)
	if err != nil {
		p.err = "cannot read: " + err.Error()
		return
	}
	p.cwd = dir
	p.entries = entries
	p.err = ""
	p.sel, p.off = 0, 0
	p.clampSel()
}

// pick hands a path to the caller; a refusal renders inside and the
// picker stays open.
func (p *filePicker) pick(path string) {
	if err := p.commit(path); err != "" {
		p.err = err
		return
	}
	p.picked = path
	p.done = true
}

func (p *filePicker) update(msg tea.Msg) tea.Cmd {
	switch msg := msg.(type) {
	case tea.PasteMsg:
		// 0091: paste is a message, not keys — it lands in the active
		// text input (location bar, then filter).
		rs := pasteRunes(msg.Content)
		switch {
		case p.locating:
			p.loc = append(p.loc, rs...)
		case p.filtering:
			p.filter = append(p.filter, rs...)
			p.clampSel()
		}
	case tea.MouseWheelMsg:
		switch msg.Button {
		case tea.MouseWheelDown:
			p.sel++
		case tea.MouseWheelUp:
			p.sel--
		}
		p.clampSel()
	case tea.KeyPressMsg:
		switch {
		case p.locating:
			p.locKey(msg)
		case p.filtering:
			p.filterKey(msg)
		default:
			p.navKey(msg)
		}
	}
	return nil
}

// navKey is plain browsing: move, descend, select, toggle, open the
// text inputs.
func (p *filePicker) navKey(k tea.KeyPressMsg) {
	page := max(p.pageH, 1)
	switch k.String() {
	case "up", "k":
		p.sel--
	case "down", "j":
		p.sel++
	case "pgup":
		p.sel -= page
	case "pgdown":
		p.sel += page
	case "home", "g":
		p.sel = -1 // the very top: the ../ row, clamped to 0 at the root
	case "end", "G":
		p.sel = len(p.visible()) - 1
	case "enter":
		p.enter()
	case "ctrl+enter":
		if p.mode != kindFiles {
			p.pick(p.cwd) // pick the directory being shown
		}
	case "right", "l":
		p.descend()
	case "left", "h", "backspace":
		p.goUp()
	case "~":
		home, _ := os.UserHomeDir()
		p.cd(home)
	case ".":
		p.showHidden = !p.showHidden
		p.readDir()
	case "/":
		p.filtering = true
	case "ctrl+l":
		p.locating = true
		p.loc = []rune(p.cwd)
		p.err = ""
	}
	p.clampSel()
}

// enter applies the mode's select rule to the highlighted entry; on
// the ../ row it navigates up in every mode — it never picks.
func (p *filePicker) enter() {
	if p.sel == -1 {
		p.goUp()
		return
	}
	vis := p.visible()
	if len(vis) == 0 {
		return
	}
	e := vis[p.sel]
	full := filepath.Join(p.cwd, e.name)
	switch {
	case e.dir && p.mode == kindDirs:
		p.pick(full)
	case e.dir:
		p.cd(full)
	case p.mode != kindDirs:
		p.pick(full)
	}
}

// descend enters the highlighted directory (any mode); on the ../ row
// it goes up like left.
func (p *filePicker) descend() {
	if p.sel == -1 {
		p.goUp()
		return
	}
	vis := p.visible()
	if len(vis) == 0 {
		return
	}
	if e := vis[p.sel]; e.dir {
		p.cd(filepath.Join(p.cwd, e.name))
	}
}

// filterKey edits the listing filter (the nav filter's semantics:
// typing narrows, enter commits, esc — via onEsc — clears).
func (p *filePicker) filterKey(k tea.KeyPressMsg) {
	switch k.String() {
	case "enter":
		p.filtering = false
	case "backspace":
		if len(p.filter) > 0 {
			p.filter = p.filter[:len(p.filter)-1]
		}
	default:
		if k.Text != "" {
			p.filter = append(p.filter, []rune(k.Text)...)
		}
	}
	p.clampSel()
}

// locKey edits the location bar: enter resolves the typed path.
func (p *filePicker) locKey(k tea.KeyPressMsg) {
	switch k.String() {
	case "enter":
		p.locEnter()
	case "backspace":
		if len(p.loc) > 0 {
			p.loc = p.loc[:len(p.loc)-1]
		}
	case "ctrl+u":
		p.loc = p.loc[:0]
	default:
		if k.Text != "" {
			p.loc = append(p.loc, []rune(k.Text)...)
		}
	}
}

// locEnter resolves the location bar: a directory navigates, a file
// picks where the mode allows files, and a not-yet-existing leaf picks
// where the field creates files (its parent must exist).
func (p *filePicker) locEnter() {
	home, _ := os.UserHomeDir()
	s := expandHome(strings.TrimSpace(string(p.loc)), home)
	if s == "" {
		return
	}
	st, err := os.Stat(s)
	switch {
	case err == nil && st.IsDir():
		p.locating = false
		p.cd(s)
	case err == nil:
		if p.mode == kindDirs {
			p.err = "that is a file — pick a directory"
			return
		}
		p.locating = false
		p.pick(s)
	default:
		// Not found: a typed leaf is a valid pick wherever the field
		// may name something new — a file to write, a mountpoint or
		// share directory to create — as long as its parent exists
		// (a typo in the path's spine refuses).
		if pst, perr := os.Stat(filepath.Dir(s)); perr != nil || !pst.IsDir() {
			p.err = "no such directory: " + filepath.Dir(s)
			return
		}
		p.locating = false
		p.pick(s)
	}
}

func (p *filePicker) view(w, h int) string {
	title := "select path"
	enterHint := "enter pick"
	switch p.mode {
	case kindDirs:
		title = "select directory"
		enterHint = "enter pick dir · ^↵ pick shown"
	case kindFiles:
		title = "select file"
	}
	lines := []string{p.th.modalTitle.Render(title), ""}
	cwd := p.cwd
	if maxw := max(w-24, 20); len([]rune(cwd)) > maxw {
		rs := []rune(cwd)
		cwd = "…" + string(rs[len(rs)-maxw:])
	}
	lines = append(lines, p.th.fieldLabel.Render(cwd))
	if p.filtering || len(p.filter) > 0 {
		lines = append(lines, p.th.meta.Render("filter: "+string(p.filter))+
			p.th.disabledMark.Render("  ("+itoa(len(p.visible()))+"/"+itoa(len(p.entries))+")"))
	}
	if p.locating {
		lines = append(lines, p.th.fieldLabel.Render("path: ")+string(p.loc)+p.th.caret.Render(" "))
	}
	if p.err != "" {
		lines = append(lines, p.th.errorMark.Render("✗ "+p.err))
	}
	lines = append(lines, "")
	listH := min(max(h-16, 4), 18)
	if p.hasUp() {
		listH-- // the pinned ../ row takes one line of the list
	}
	p.pageH = listH
	vis := p.visible()
	if p.sel < 0 {
		p.off = 0 // the ../ row is pinned at the top
	}
	if p.sel < p.off {
		p.off = max(p.sel, 0)
	}
	if p.sel >= p.off+listH {
		p.off = p.sel - listH + 1
	}
	if p.hasUp() {
		if p.sel == -1 {
			lines = append(lines, p.th.cursorRow.MaxWidth(max(w-16, 30)).Render(" ../"))
		} else {
			lines = append(lines, "  "+p.th.sectionLabel.Render("../"))
		}
	}
	if len(vis) == 0 {
		lines = append(lines, p.th.disabledMark.Render("  (no entries)"))
	}
	for i := p.off; i < len(vis) && i < p.off+listH; i++ {
		e := vis[i]
		name := e.name
		if e.dir {
			name += "/"
		}
		if i == p.sel {
			lines = append(lines, p.th.cursorRow.MaxWidth(max(w-16, 30)).Render(" "+name))
		} else if e.dir {
			lines = append(lines, "  "+p.th.sectionLabel.Render(name))
		} else {
			lines = append(lines, "  "+name)
		}
	}
	lines = append(lines, "",
		p.th.meta.Render(enterHint+" · → in · ← up · / filter · ^L path · . hidden · esc close"))
	return p.th.modalBorder.Padding(1, 2).Render(strings.Join(lines, "\n"))
}

// itoa avoids strconv for the two counts the picker renders.
func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	var b [8]byte
	i := len(b)
	for n > 0 {
		i--
		b[i] = byte('0' + n%10)
		n /= 10
	}
	return string(b[i:])
}
