package tui

// The M15 nav pane (T-tui-nav): modules as rows, overlay layers
// (base / user / host) as expandable children. A pure state machine over
// the modules read — the compositor feeds it navLoadedMsg, asks it for
// rows, and renders its view. The tree position is the layer picker:
// selecting a child points the workspace at that layer file.

import (
	"cmp"
	"path/filepath"
	"slices"
	"strings"

	"github.com/thedataflows/dotdrift/internal/drift"
	"github.com/thedataflows/dotdrift/internal/profile"
	"github.com/thedataflows/dotdrift/internal/service"
)

// navLayer is one existing overlay of a module, in fixed base/user/host
// order.
type navLayer struct {
	layer string // "base" | "user" | "host"
	owner string // layer owner; empty for base
	dir   string // absolute layer module directory (the draft-ledger key)
	label string // "base", "user cri", "host myhost", "user root (superuser)"
	super bool   // a superuser overlay (0029): the module needs root
}

// navModule is one module row plus its layers. A non-empty reason greys
// the row out (skipped or failed to load; the row names why).
type navModule struct {
	id     string
	layers []navLayer
	reason string
}

// navRow is one visible row: a module (layer == "") or a layer child.
type navRow struct {
	moduleID string
	layer    string
	label    string
	dir      string
	reason   string
}

type navModel struct {
	modules  []navModule
	expanded map[string]bool
	cursor   int
	offset   int
	bodyH    int    // the visible body height from the last layout: the paging step
	pending  bool   // a read is in flight: placeholder rows
	loadErr  string // the last read failed: the row names it
}

// navModules builds the module list from a landed read: one row per
// module across layers (contract 1), overlays as children, layer
// visibility per issue 0029 (host layers for this host, user layers for
// this user, superuser overlays named). Superuser-only modules surface
// from the skip list, greyed, with their reason.
func navModules(read *service.ModulesRead) []navModule {
	if read == nil || read.Profile == nil {
		return nil
	}
	var host, user string
	if f := read.Facts; f != nil {
		host, user = f.Hostname, f.Username
	}
	layers := service.ModuleLayers(read.Profile)
	super := superuserOwners(read.Profile.Skipped)

	byDir := map[string][]navLayer{}
	for _, l := range layers {
		if nl, ok := navLayerFor(l, host, user, super); ok {
			byDir[l.Dir] = append(byDir[l.Dir], nl)
		}
	}

	var out []navModule
	seen := map[string]bool{}
	add := func(m profile.Module, reason string) {
		seen[m.ID] = true
		ls := byDir[filepath.Base(m.Path)]
		orderLayers(ls)
		out = append(out, navModule{id: m.ID, layers: ls, reason: reason})
	}
	for _, m := range read.Profile.Modules {
		add(m, "")
	}
	for _, s := range read.Profile.Skipped {
		if !seen[s.Module.ID] {
			add(s.Module, s.Reason)
		}
	}
	return out
}

// navLayerFor maps a module layer onto its nav child, applying 0029
// visibility. ok is false for layers not visible to this user/host.
func navLayerFor(l drift.ModuleLayer, host, user string, super map[string]bool) (navLayer, bool) {
	nl := navLayer{layer: l.Layer, owner: l.Owner, dir: l.Path}
	switch l.Layer {
	case "base":
		nl.label = "base"
	case "host":
		if l.Owner != host || host == "" {
			return nl, false
		}
		nl.label = "host " + l.Owner
	case "user":
		switch {
		case super[l.Owner]:
			nl.label = "user " + l.Owner + " (superuser)"
			nl.super = true
		case user != "" && l.Owner == user:
			nl.label = "user " + l.Owner
		default:
			return nl, false
		}
	default:
		return nl, false
	}
	return nl, true
}

// orderLayers sorts children into the fixed base, user, host order.
func orderLayers(ls []navLayer) {
	rank := map[string]int{"base": 0, "user": 1, "host": 2}
	slices.SortFunc(ls, func(a, b navLayer) int {
		return cmp.Compare(rank[a.layer], rank[b.layer])
	})
}

// rows returns the visible rows: every module, plus the layer children of
// expanded modules.
func (n *navModel) rows() []navRow {
	var out []navRow
	for _, m := range n.modules {
		out = append(out, navRow{moduleID: m.id, label: m.id, reason: m.reason})
		if n.expanded[m.id] {
			for _, l := range m.layers {
				out = append(out, navRow{
					moduleID: m.id, layer: l.layer,
					label: l.label, dir: l.dir,
				})
			}
		}
	}
	return out
}

// selected returns the row under the cursor, or the zero row.
func (n *navModel) selected() navRow {
	rows := n.rows()
	if len(rows) == 0 || n.cursor >= len(rows) {
		return navRow{}
	}
	return rows[n.cursor]
}

// move moves the cursor by delta rows, keeping the scroll window on it.
func (n *navModel) move(delta int) {
	n.cursor += delta
	if n.cursor < 0 {
		n.cursor = 0
	}
	if rows := len(n.rows()); n.cursor >= rows {
		n.cursor = max(rows-1, 0)
	}
}

// page moves the cursor by a visible page (the body height minus the
// group-title line), clamped like move (0075 T-tui-page).
func (n *navModel) page(delta int) {
	for i, steps := 0, max(n.bodyH-1, 1); i < steps; i++ {
		before := n.cursor
		n.move(delta)
		if n.cursor == before {
			return
		}
	}
}

// home/end jump to the first/last row.
func (n *navModel) home() { n.cursor = 0 }

func (n *navModel) end() { n.cursor = max(len(n.rows())-1, 0) }

// collapse implements h/left: on a child row it returns to the module
// row; on an expanded module row it collapses it.
func (n *navModel) collapse() {
	rows := n.rows()
	if len(rows) == 0 {
		return
	}
	sel := rows[n.cursor]
	if sel.layer != "" {
		for i, r := range rows {
			if r.moduleID == sel.moduleID && r.layer == "" {
				n.cursor = i
				break
			}
		}
		return
	}
	delete(n.expanded, sel.moduleID)
}

// expand implements l/right: a collapsed module row with layers expands.
func (n *navModel) expand() {
	rows := n.rows()
	if len(rows) == 0 {
		return
	}
	sel := rows[n.cursor]
	if sel.layer != "" {
		return
	}
	for _, m := range n.modules {
		if m.id == sel.moduleID && len(m.layers) > 0 {
			if n.expanded == nil {
				n.expanded = map[string]bool{}
			}
			n.expanded[m.id] = true
		}
	}
}

// restore places the cursor back on want after a reload, clamping to the
// list when the row is gone — never lose the user's place.
func (n *navModel) restore(want navRow) {
	if want.moduleID == "" {
		n.cursor = 0
		return
	}
	for i, r := range n.rows() {
		if r.moduleID == want.moduleID && r.layer == want.layer {
			n.cursor = i
			return
		}
	}
	if rows := len(n.rows()); n.cursor >= rows {
		n.cursor = max(rows-1, 0)
	}
}

// view renders the pane: the group header, then the visible rows inside
// the scroll window. h is the content height.
func (n *navModel) view(w, h int, th theme, drafts map[string]bool) string {
	var lines []string
	lines = append(lines, th.groupTitle.Render("MODULES"))
	switch {
	case n.pending:
		lines = append(lines, th.loading.Render("  …"))
	case n.loadErr != "":
		lines = append(lines, th.errorMark.Render("  load failed: "+n.loadErr))
	case len(n.modules) == 0:
		lines = append(lines, th.disabledMark.Render("  (no modules)"))
	default:
		rows := n.rows()
		if avail := h - 1; avail > 0 { // the header line takes one row
			if n.cursor < n.offset {
				n.offset = n.cursor
			}
			if n.cursor >= n.offset+avail {
				n.offset = n.cursor - avail + 1
			}
		}
		for i := n.offset; i < len(rows) && len(lines) < h; i++ {
			lines = append(lines, n.rowView(rows[i], i == n.cursor, th, drafts, w))
		}
	}
	return strings.Join(lines, "\n")
}

// rowView renders one row: expansion marker, label, dirty and reason
// suffixes, truncated to the pane width. The cursor row is bold (the
// locked Selection treatment).
// rowView renders one row: lead/bar, the expansion marker, label, dirty
// and reason suffixes, truncated to the pane width. The cursor row takes
// the cursorRow treatment (bar + accent, 0075); plain rows keep the
// two-space lead so text columns align.
func (n *navModel) rowView(r navRow, cursor bool, th theme, drafts map[string]bool, w int) string {
	var rest string
	if r.layer == "" {
		marker := " "
		switch {
		case n.expanded[r.moduleID]:
			marker = "▾"
		case n.hasLayers(r.moduleID):
			marker = "▸"
		}
		rest = marker + " " + r.label
	} else {
		rest = "    " + r.label
	}
	if r.reason != "" && r.layer == "" {
		rest += " " + th.reasonMark.Render("· "+r.reason)
	}
	if n.rowDirty(r, drafts) {
		rest += " " + th.dirtyMark.Render("●")
	}
	if cursor {
		return th.cursorRow.MaxWidth(w).Render(" " + rest)
	}
	return th.rowText.MaxWidth(w).Render("  " + rest)
}

// rowDirty: a layer child is dirty when its file has a draft; a module
// row is dirty when any of its layer files has one.
func (n *navModel) rowDirty(r navRow, drafts map[string]bool) bool {
	if r.layer != "" {
		return r.dir != "" && drafts[r.dir]
	}
	for _, m := range n.modules {
		if m.id != r.moduleID {
			continue
		}
		for _, l := range m.layers {
			if drafts[l.dir] {
				return true
			}
		}
	}
	return false
}

func (n *navModel) hasLayers(id string) bool {
	for _, m := range n.modules {
		if m.id == id {
			return len(m.layers) > 0
		}
	}
	return false
}
