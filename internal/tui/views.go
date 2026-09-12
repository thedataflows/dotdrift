package tui

import (
	"bytes"
	"fmt"
	"maps"
	"path/filepath"
	"slices"
	"strings"

	"github.com/thedataflows/dotdrift/internal/profile"
	"github.com/thedataflows/dotdrift/internal/resolve"
	"github.com/thedataflows/dotdrift/internal/service"
)

// The shell's read views (T-tui-shell): the module's resolved overlay
// stack (what plan sees — 0062-D3), the raw per-layer declarations (the
// later editor input), accounts, the canonical status report in the
// scroll pane, the plan with per-step diffs and its apply-gate stub, and
// the disabled write-action stubs. TUI-styled through the registry; the
// canonical renderer bytes pass through untouched (0061-D5).

// renderModuleResolved renders the resolved overlay stack from the
// module-filtered plan read — fact-identical to `plan <module>` by
// construction.
func renderModuleResolved(m *Shell, pr *service.PlanRead, moduleID string) string {
	var b strings.Builder
	b.WriteString(m.th.viewTitle.Render("MODULE "+moduleID) + m.th.meta.Render(" — resolved for host "+factsHost(m)+", user "+factsUser(m)))
	if pr == nil || pr.Plan == nil {
		b.WriteString("\n" + m.th.errorMark.Render("error: no plan read"))
		return b.String()
	}
	if mod := findModule(m.read.Profile, moduleID); mod != nil {
		meta := "scope " + mod.Config.ScopeOrDefault()
		if mod.App != "" && mod.App != mod.ID {
			meta += " · app " + mod.App
		}
		if mod.Config.Description != "" {
			meta += " · " + mod.Config.Description
		}
		b.WriteString("\n" + m.th.meta.Render(meta))
	}
	if origins := m.moduleOriginLabels(moduleID); len(origins) > 0 {
		b.WriteString("\n" + m.th.sectionLabel.Render("origins") + "   " +
			m.th.meta.Render(strings.Join(origins, " → ")+" (later layers win)"))
	}

	p := pr.Plan
	writeList := func(label string, items []string) {
		if len(items) == 0 {
			return
		}
		b.WriteString("\n" + m.th.sectionLabel.Render(label) + "   " + strings.Join(items, " · "))
	}
	pkgs := make([]string, 0, len(p.Packages.Install)+len(p.Packages.Remove))
	for _, pkg := range p.Packages.Install {
		pkgs = append(pkgs, "+"+pkg)
	}
	for _, pkg := range p.Packages.Remove {
		pkgs = append(pkgs, "−"+pkg)
	}
	writeList("packages", pkgs)
	if len(p.Tools.Versions) > 0 {
		tools := make([]string, 0, len(p.Tools.Versions))
		for _, name := range slices.Sorted(maps.Keys(p.Tools.Versions)) {
			tools = append(tools, name+"="+p.Tools.Versions[name])
		}
		writeList("tools", tools)
	}
	if len(p.Dotfiles.Entries) > 0 {
		var lines []string
		for _, e := range p.Dotfiles.Entries {
			lines = append(lines, fmt.Sprintf("%s ← %s (%s)", e.Target, m.layerLabel(moduleID, e.Layer), e.Mode))
		}
		writeMultiline(&b, m.th, "dotfiles", lines)
	}
	if len(p.Hooks.Pre) > 0 || len(p.Hooks.Post) > 0 {
		var lines []string
		if pre := hookChain(p.Hooks.Pre); pre != "" {
			lines = append(lines, "pre:  "+pre)
		}
		if post := hookChain(p.Hooks.Post); post != "" {
			lines = append(lines, "post: "+post)
		}
		writeMultiline(&b, m.th, "hooks", lines)
	}
	writeSystemdMountsSmb(&b, m, p)
	return b.String()
}

// writeSystemdMountsSmb renders the plan's systemd/mounts/smb sections
// when the module declares them (parity with the plan report's tail).
func writeSystemdMountsSmb(b *strings.Builder, m *Shell, p *resolve.Plan) {
	if len(p.Systemd.Units) > 0 {
		lines := make([]string, 0, len(p.Systemd.Units))
		for _, u := range p.Systemd.Units {
			lines = append(lines, fmt.Sprintf("%s (%s, %s)", u.Name, u.Kind, m.layerLabel("", u.Layer)))
		}
		writeMultiline(b, m.th, "systemd", lines)
	}
	if len(p.Mounts.Entries) > 0 {
		lines := make([]string, 0, len(p.Mounts.Entries))
		for _, e := range p.Mounts.Entries {
			lines = append(lines, fmt.Sprintf("%s: %s → %s [%s]", e.Name, e.Spec.Source, e.Spec.Destination, e.Scope))
		}
		writeMultiline(b, m.th, "mounts", lines)
	}
	if len(p.Smb.Modules) > 0 {
		lines := make([]string, 0, len(p.Smb.Modules))
		for _, sm := range p.Smb.Modules {
			lines = append(lines, fmt.Sprintf("%s: users %s", sm.Module, strings.Join(sm.Spec.Users, ", ")))
		}
		writeMultiline(b, m.th, "smb", lines)
	}
}

func writeMultiline(b *strings.Builder, th theme, label string, lines []string) {
	b.WriteString("\n" + th.sectionLabel.Render(label))
	for i, l := range lines {
		if i == 0 {
			b.WriteString("   ")
		} else {
			b.WriteString("\n         ")
		}
		b.WriteString(l)
	}
}

func hookChain(cmds []profile.HookCommand) string {
	parts := make([]string, 0, len(cmds))
	for _, c := range cmds {
		parts = append(parts, c.Command)
	}
	return strings.Join(parts, " → ")
}

// renderModuleRaw renders every contributing layer's raw declaration —
// the exact editor input (T-tui-editors opens from here).
func renderModuleRaw(m *Shell, it treeItem) string {
	var b strings.Builder
	b.WriteString(m.th.viewTitle.Render("MODULE "+it.moduleID) + m.th.meta.Render(" — raw declarations"))
	for _, o := range m.moduleOriginItems(it.moduleID) {
		b.WriteString("\n")
		b.WriteString("\n" + m.th.originMark.Render("["+o.origin+"]") + m.th.meta.Render(" "+m.relModuleFile(o)))
		b.WriteString(renderRawConfig(m, o.dir))
	}
	return b.String()
}

// renderOriginView renders one layer's raw declaration (an overlay-tree
// node selected).
func renderOriginView(m *Shell, it treeItem) string {
	var b strings.Builder
	b.WriteString(m.th.viewTitle.Render("RAW "+it.origin) + m.th.meta.Render(" — "+it.moduleID))
	b.WriteString("\n" + m.th.meta.Render(m.relModuleFile(it)))
	b.WriteString(renderRawConfig(m, it.dir))
	return b.String()
}

// renderRawConfig renders one layer's module.toml as compact declaration
// lines (every section, none silently dropped).
func renderRawConfig(m *Shell, dir string) string {
	cfg, err := m.area.ModuleConfigAt(dir)
	if err != nil {
		return "\n" + m.th.errorMark.Render("error: "+err.Error())
	}
	if cfg == nil {
		return "\n" + m.th.meta.Render("(no module.toml)")
	}
	var b strings.Builder
	if cfg.Scope != "" && cfg.Scope != profile.ScopeUser {
		b.WriteString("\n  scope: " + cfg.Scope)
	}
	if cfg.Disabled {
		b.WriteString("\n  disabled: true")
	}
	if len(cfg.Packages.Present) > 0 || len(cfg.Packages.Absent) > 0 {
		line := "packages present: " + strings.Join(cfg.Packages.Present, ", ")
		if len(cfg.Packages.Absent) > 0 {
			line += " · absent: " + strings.Join(cfg.Packages.Absent, ", ")
		}
		b.WriteString("\n  " + line)
	}
	if len(cfg.Tools) > 0 {
		pairs := make([]string, 0, len(cfg.Tools))
		for _, name := range slices.Sorted(maps.Keys(cfg.Tools)) {
			pairs = append(pairs, name+"="+cfg.Tools[name])
		}
		b.WriteString("\n  tools: " + strings.Join(pairs, ", "))
	}
	if len(cfg.Secrets) > 0 {
		b.WriteString("\n  secrets: " + strings.Join(slices.Sorted(maps.Keys(cfg.Secrets)), ", "))
	}
	if len(cfg.Dotfiles) > 0 {
		b.WriteString("\n  dotfiles:")
		for _, target := range slices.Sorted(maps.Keys(cfg.Dotfiles)) {
			d := cfg.Dotfiles[target]
			b.WriteString(fmt.Sprintf("\n    %s ← %s (%s)", target, d.Source, d.Mode))
		}
	}
	if len(cfg.Hooks.Pre) > 0 || len(cfg.Hooks.Post) > 0 {
		b.WriteString("\n  hooks:")
		for _, c := range cfg.Hooks.Pre {
			b.WriteString("\n    pre:  " + c.Command + optionalSuffix(c.Optional))
		}
		for _, c := range cfg.Hooks.Post {
			b.WriteString("\n    post: " + c.Command + optionalSuffix(c.Optional))
		}
	}
	if len(cfg.Mounts) > 0 {
		b.WriteString("\n  mounts:")
		for _, name := range slices.Sorted(maps.Keys(cfg.Mounts)) {
			spec := cfg.Mounts[name]
			b.WriteString(fmt.Sprintf("\n    %s: %s → %s", name, spec.Source, spec.Destination))
		}
	}
	if len(cfg.Smb.Users) > 0 || cfg.Smb.Group != "" {
		line := "smb"
		if cfg.Smb.Group != "" {
			line += " group=" + cfg.Smb.Group
		}
		if len(cfg.Smb.Users) > 0 {
			line += " users=" + strings.Join(cfg.Smb.Users, ",")
		}
		b.WriteString("\n  " + line)
	}
	if len(cfg.Systemd.Units) > 0 {
		b.WriteString("\n  systemd.units:")
		for _, name := range slices.Sorted(maps.Keys(cfg.Systemd.Units)) {
			b.WriteString("\n    " + name)
		}
	}
	if b.Len() == 0 {
		return "\n  (empty declaration)"
	}
	return b.String()
}

func optionalSuffix(optional bool) string {
	if optional {
		return " (optional)"
	}
	return ""
}

// renderAccountView renders an account node: the modules its layers
// carry, and — for this machine — the detected facts.
func renderAccountView(m *Shell, it treeItem) string {
	var b strings.Builder
	title := accountTitle(it)
	if it.current {
		title += m.th.meta.Render(" (this machine)")
	}
	b.WriteString(m.th.viewTitle.Render(title))
	if it.reason != "" {
		b.WriteString("\n" + m.th.reasonMark.Render(it.reason))
		b.WriteString("\n" + m.th.meta.Render("selectable only under sudo — run `sudo dotdrift tui` to converge it"))
	}
	if mods := accountModules(m, it); len(mods) > 0 {
		writeMultiline(&b, m.th, "modules", mods)
	} else {
		b.WriteString("\n" + m.th.meta.Render("no module layers"))
	}
	if it.current && it.acctKind == "host" && m.read.Facts != nil {
		f := m.read.Facts
		writeMultiline(&b, m.th, "facts", []string{
			"os " + f.OS + " · kernel " + f.Kernel,
			"distro " + f.Distro + " · gpu " + f.GPU,
			"package backend " + f.Backend,
		})
	}
	return b.String()
}

// renderStatusView renders the full status read through the canonical
// renderer (0062-D9 parity): header, drift report, ADR-0006 notice.
func renderStatusView(th theme, r *service.StatusRead) string {
	var buf bytes.Buffer
	if err := service.RenderStatusSummary(&buf, r); err != nil {
		return th.viewTitle.Render("STATUS") + "\n" + th.errorMark.Render("error: "+err.Error())
	}
	return th.viewTitle.Render("STATUS") + "\n" + buf.String()
}

// renderPlanView renders the canonical plan report plus per-step diffs of
// differing copy-mode dotfiles, and the apply-gate stub (T-tui-apply owns
// the real gate — the plan's only write door).
func renderPlanView(m *Shell, pr *service.PlanRead, diff []service.DiffEntry) string {
	var buf bytes.Buffer
	if err := service.RenderPlanReport(&buf, pr, nil); err != nil {
		return m.th.viewTitle.Render("PLAN") + "\n" + m.th.errorMark.Render("error: "+err.Error())
	}
	content := m.th.viewTitle.Render("PLAN") + "\n" + buf.String()
	if len(diff) > 0 {
		var dbuf bytes.Buffer
		if err := service.RenderDiff(&dbuf, diff, "internal", false); err == nil {
			content += m.th.sectionLabel.Render("\ndiffs") + "\n" + dbuf.String()
		}
	}
	content += "\n" + m.th.reasonMark.Render("apply: arrives with T-tui-apply — the plan gate will be the TUI's only write path.")
	return content
}

func renderLoading(th theme, title, what string) string {
	return th.viewTitle.Render(title) + "\n" + th.loading.Render(what)
}

func renderHint(th theme, text string) string {
	return th.meta.Render(text)
}

func renderSkipNotice(th theme, title, reason string) string {
	return th.viewTitle.Render(title) + "\n" + th.reasonMark.Render(reason) +
		"\n" + th.meta.Render("selectable only under sudo — run `sudo dotdrift tui` to converge it")
}

func renderLoadError(th theme, title string, err error) string {
	return th.viewTitle.Render(title) + "\n" + th.errorMark.Render("error: "+err.Error())
}

func renderActionStub(th theme, action string) string {
	return th.viewTitle.Render(strings.ToUpper(action)+" — dialog") + "\n" +
		th.meta.Render("opens with T-tui-writes; disabled until then.")
}

// moduleOriginItems returns a module's origin items in merge order.
func (m *Shell) moduleOriginItems(moduleID string) []treeItem {
	var out []treeItem
	for _, g := range m.roots {
		if g.item.label != "MODULES" {
			continue
		}
		for _, mod := range g.kids {
			if mod.item.moduleID != moduleID {
				continue
			}
			for _, o := range mod.kids {
				out = append(out, o.item)
			}
		}
	}
	return out
}

func (m *Shell) moduleOriginLabels(moduleID string) []string {
	items := m.moduleOriginItems(moduleID)
	labels := make([]string, 0, len(items))
	for _, it := range items {
		labels = append(labels, it.origin)
	}
	return labels
}

// layerLabel maps a plan layer name (base/host/user) onto the module's
// origin marker (base/hosts/<h>/users/<u>).
func (m *Shell) layerLabel(moduleID, layer string) string {
	origins := m.moduleOriginItems(moduleID)
	switch layer {
	case "base":
		if len(origins) > 0 {
			return origins[0].origin
		}
	case "host":
		if len(origins) > 1 {
			return origins[1].origin
		}
	case "user":
		if len(origins) > 2 {
			return origins[2].origin
		}
	}
	return layer
}

// relModuleFile is an origin's module.toml path relative to the profile
// root (absolute when the root cannot be relativized).
func (m *Shell) relModuleFile(it treeItem) string {
	file := filepath.Join(it.dir, "module.toml")
	if m.read == nil || m.read.Profile == nil || m.read.Profile.Root == "" {
		return file
	}
	if rel, err := filepath.Rel(m.read.Profile.Root, it.dir); err == nil {
		return filepath.Join(rel, "module.toml")
	}
	return file
}

// accountModules lists the module dirs a host/user layer carries.
func accountModules(m *Shell, it treeItem) []string {
	var out []string
	for _, l := range service.ModuleLayers(m.read.Profile) {
		switch it.acctKind {
		case "host":
			if l.Layer == "host" && l.Owner == it.acctName {
				out = append(out, l.Dir)
			}
		case "user", "superuser":
			if l.Layer == "user" && l.Owner == it.acctName {
				out = append(out, l.Dir)
			}
		}
	}
	return out
}

func accountTitle(it treeItem) string {
	switch it.acctKind {
	case "host":
		return "HOST " + it.acctName
	case "superuser":
		return "SUPERUSER " + it.acctName
	default:
		return "USER " + it.acctName
	}
}

func findModule(p *profile.Profile, id string) *profile.Module {
	for i := range p.Modules {
		if p.Modules[i].ID == id {
			return &p.Modules[i]
		}
	}
	return nil
}

func factsHost(m *Shell) string {
	if m.read != nil && m.read.Facts != nil {
		return m.read.Facts.Hostname
	}
	return "?"
}

func factsUser(m *Shell) string {
	if m.read != nil && m.read.Facts != nil {
		return m.read.Facts.Username
	}
	return "?"
}
