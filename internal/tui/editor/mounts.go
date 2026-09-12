package editor

import (
	"fmt"
	"maps"
	"slices"
	"strings"

	"github.com/thedataflows/dotdrift/internal/generate"
	"github.com/thedataflows/dotdrift/internal/profile"
	"github.com/thedataflows/dotdrift/internal/service"
)

// The mounts and smb editors (0065-D6/D9): form adapters whose prefill
// absorbs the generate wizards' state machines — KindForDefaults,
// PrefillForVolume, MountChoice.validate, and the SmbInput assembly are
// generate's own code, reused across the seam (0066's absorption starts
// here). The machines' write semantics carry over: choices accumulate and
// the section replaces wholesale on save.

type mountsAdapter struct {
	names     []string
	draft     map[string]profile.MountSpec
	baseline  map[string]profile.MountSpec
	cur       int
	inForm    bool
	formFocus int
	fields    [5]field // source, destination, type, startat, state
	prompting bool
	prompt    field
}

var mountsFieldSpecs = [5]struct {
	key   string
	label string
	pick  func(m profile.MountSpec) string
	apply func(*profile.MountSpec, string)
}{
	{"source", "source", func(m profile.MountSpec) string { return m.Source }, func(m *profile.MountSpec, v string) { m.Source = v }},
	{"destination", "destination", func(m profile.MountSpec) string { return m.Destination }, func(m *profile.MountSpec, v string) { m.Destination = v }},
	{"type", "type", func(m profile.MountSpec) string { return m.Type }, func(m *profile.MountSpec, v string) { m.Type = v }},
	{"startat", "startat", func(m profile.MountSpec) string { return m.StartAt }, func(m *profile.MountSpec, v string) { m.StartAt = v }},
	{"state", "state", func(m profile.MountSpec) string { return m.State }, func(m *profile.MountSpec, v string) { m.State = v }},
}

func newMountsAdapter(cfg *profile.ModuleConfig) *mountsAdapter {
	a := &mountsAdapter{
		draft:    map[string]profile.MountSpec{},
		baseline: map[string]profile.MountSpec{},
	}
	for k, m := range cfg.Mounts {
		a.draft[k] = m
		a.baseline[k] = m
		a.names = append(a.names, k)
	}
	slices.Sort(a.names)
	return a
}

func (a *mountsAdapter) ID() string     { return "mounts" }
func (a *mountsAdapter) Title() string  { return "mounts" }
func (a *mountsAdapter) Family() string { return service.FamilyMounts }

// Mounts returns the assembled draft — the seam the CLI/TUI equivalence
// check reads (contract 15).
func (a *mountsAdapter) Mounts() map[string]profile.MountSpec { return a.draft }

func (a *mountsAdapter) Dirty() bool {
	return !reflectMounts(a.draft, a.baseline)
}

func reflectMounts(x, y map[string]profile.MountSpec) bool {
	if len(x) != len(y) {
		return false
	}
	for k, m := range x {
		b, ok := y[k]
		if !ok || b.Source != m.Source || b.Destination != m.Destination ||
			b.Type != m.Type || b.StartAt != m.StartAt || b.State != m.State ||
			!slices.Equal(b.Options, m.Options) {
			return false
		}
	}
	return true
}

func (a *mountsAdapter) Block() string {
	return profile.EncodeMountsSection(a.draft)
}

// Errors reuses resolve's structural mount checks live (non-empty
// source/destination/type, known state); the registry check stays a
// generate-side concern — resolve accepts types outside the registry.
func (a *mountsAdapter) Errors() []string {
	var out []string
	for _, name := range a.names {
		m := a.draft[name]
		if m.Source == "" {
			out = append(out, name+": source is required")
		}
		if m.Destination == "" {
			out = append(out, name+": destination is required")
		}
		if m.Type == "" {
			out = append(out, name+": type is required")
		}
		switch m.State {
		case "", "enabled", "disabled":
		default:
			out = append(out, fmt.Sprintf("%s: state must be \"enabled\" or \"disabled\", got %q", name, m.State))
		}
	}
	return out
}

// PrefillVolume absorbs one volume through generate's own prefill policy
// and gate: PrefillForVolume merges the defaults, MountChoice.validate
// gates the choice before it lands in the draft. A later choice with the
// same name replaces the earlier one (the machines' whole-entry-by-name
// semantics).
func (a *mountsAdapter) PrefillVolume(reg *generate.Registry, c generate.VolumeChoice, defaults generate.MountChoice, first bool) error {
	choice := generate.PrefillForVolume(c, defaults, first)
	if err := choice.Validate(reg); err != nil {
		return err
	}
	a.draft[choice.Name] = choice.Spec()
	slices.Sort(a.names)
	return nil
}

// ApplyChoice validates and accumulates one mount choice (the
// interactive-loop entry point, AddMount's adapter counterpart).
func (a *mountsAdapter) ApplyChoice(reg *generate.Registry, c generate.MountChoice) error {
	if err := c.Validate(reg); err != nil {
		return err
	}
	a.draft[c.Name] = c.Spec()
	slices.Sort(a.names)
	return nil
}

func (a *mountsAdapter) HandleKey(key string) {
	if a.prompting {
		if a.prompt.handleEditKey(key) && !a.prompt.edit {
			if name := strings.TrimSpace(a.prompt.String()); name != "" {
				if _, ok := a.draft[name]; !ok {
					a.draft[name] = profile.MountSpec{}
					a.names = append(a.names, name)
					slices.Sort(a.names)
				}
				a.cur = slices.Index(a.names, name)
				a.openForm(name)
			}
			a.prompting = false
		}
		return
	}
	if a.inForm {
		if a.fields[a.formFocus].edit {
			a.fields[a.formFocus].handleEditKey(key)
			a.commitForm()
			return
		}
		switch key {
		case "j", "down":
			a.formFocus = (a.formFocus + 1) % len(a.fields)
		case "k", "up":
			a.formFocus = (a.formFocus + len(a.fields) - 1) % len(a.fields)
		case "enter":
			a.fields[a.formFocus].beginEdit()
		case "esc":
			a.inForm = false
		}
		return
	}
	switch {
	case key == "j" || key == "down":
		if a.cur < len(a.names)-1 {
			a.cur++
		}
	case key == "k" || key == "up":
		if a.cur > 0 {
			a.cur--
		}
	case key == "enter":
		if a.cur < len(a.names) {
			a.openForm(a.names[a.cur])
		}
	case key == "n":
		a.prompting = true
		a.prompt = newField("name", "mount", "")
		a.prompt.beginEdit()
	case key == "x":
		if a.cur < len(a.names) {
			delete(a.draft, a.names[a.cur])
			a.names = slices.Delete(a.names, a.cur, a.cur+1)
			if a.cur >= len(a.names) && a.cur > 0 {
				a.cur--
			}
		}
	}
}

func (a *mountsAdapter) openForm(name string) {
	a.inForm = true
	a.formFocus = 0
	m := a.draft[name]
	for i, spec := range mountsFieldSpecs {
		a.fields[i] = newField(spec.key, spec.label, spec.pick(m))
	}
	a.fields[0].edit = true
}

func (a *mountsAdapter) commitForm() {
	m := a.draft[a.names[a.cur]]
	for i, spec := range mountsFieldSpecs {
		spec.apply(&m, a.fields[i].String())
	}
	a.draft[a.names[a.cur]] = m
}

func (a *mountsAdapter) View(p Palette) string {
	if a.prompting {
		return p.Meta("mount name: ") + renderField(p, &a.prompt) + p.Meta("  enter confirm · esc cancel")
	}
	if a.inForm {
		var b strings.Builder
		b.WriteString(p.Label(a.names[a.cur]) + p.Meta(" — mount") + "\n")
		for i, spec := range mountsFieldSpecs {
			cur := "  "
			if i == a.formFocus {
				cur = "> "
			}
			b.WriteString(cur + spec.label + " · " + renderField(p, &a.fields[i]) + "\n")
		}
		return b.String()
	}
	if len(a.names) == 0 {
		return p.Meta("no mounts — n adds one")
	}
	var b strings.Builder
	for i, name := range a.names {
		m := a.draft[name]
		cur := "  "
		if i == a.cur {
			cur = "> "
		}
		line := cur + name + ": " + m.Source + " → " + m.Destination
		if i == a.cur {
			line = p.Mark(line)
		}
		b.WriteString(line + "\n")
	}
	return b.String()
}

type smbAdapter struct {
	group, users   string
	usersBaseline  string
	groupBaseline  string
	avahi          *bool
	avahiBaseline  *bool
	shareNames     []string
	shares         map[string]profile.ShareSpec
	sharesBaseline map[string]profile.ShareSpec
	cur            int
	inForm         bool
	formFocus      int
	shareFields    [5]field // path, comment, valid_users, writable, public
	serverFocus    int
	serverEditing  bool
	prompting      bool
	prompt         field
}

var smbFieldSpecs = [5]struct {
	key   string
	label string
	pick  func(s profile.ShareSpec) string
	apply func(*profile.ShareSpec, string)
}{
	{"path", "path", func(s profile.ShareSpec) string { return s.Path }, func(s *profile.ShareSpec, v string) { s.Path = v }},
	{"comment", "comment", func(s profile.ShareSpec) string { return s.Comment }, func(s *profile.ShareSpec, v string) { s.Comment = v }},
	{"valid_users", "valid_users", func(s profile.ShareSpec) string { return s.ValidUsers }, func(s *profile.ShareSpec, v string) { s.ValidUsers = v }},
	{"writable", "writable", func(s profile.ShareSpec) string { return boolText(s.Writable) }, func(s *profile.ShareSpec, v string) { s.Writable = v == "true" }},
	{"public", "public", func(s profile.ShareSpec) string { return boolText(s.Public) }, func(s *profile.ShareSpec, v string) { s.Public = v == "true" }},
}

func boolText(b bool) string {
	if b {
		return "true"
	}
	return "false"
}

func newSmbAdapter(cfg *profile.ModuleConfig) *smbAdapter {
	a := &smbAdapter{
		group:          cfg.Smb.Group,
		groupBaseline:  cfg.Smb.Group,
		users:          strings.Join(cfg.Smb.Users, ", "),
		avahi:          cfg.Smb.Avahi,
		avahiBaseline:  cfg.Smb.Avahi,
		shares:         map[string]profile.ShareSpec{},
		sharesBaseline: map[string]profile.ShareSpec{},
	}
	for k, s := range cfg.Smb.Shares {
		a.shares[k] = s
		a.sharesBaseline[k] = s
		a.shareNames = append(a.shareNames, k)
	}
	slices.Sort(a.shareNames)
	return a
}

func (a *smbAdapter) ID() string     { return "smb" }
func (a *smbAdapter) Title() string  { return "smb" }
func (a *smbAdapter) Family() string { return service.FamilySmb }

// Smb returns the assembled draft — the contract-15 seam read.
func (a *smbAdapter) Smb() *profile.SmbSpec {
	return &profile.SmbSpec{
		Group:  a.group,
		Users:  splitList(a.users),
		Avahi:  a.avahi,
		Shares: a.shares,
	}
}

func splitList(s string) []string {
	if strings.TrimSpace(s) == "" {
		return nil
	}
	var out []string
	for _, part := range strings.Split(s, ",") {
		if p := strings.TrimSpace(part); p != "" {
			out = append(out, p)
		}
	}
	return out
}

func (a *smbAdapter) Dirty() bool {
	if a.group != a.groupBaseline || a.users != a.usersBaseline || a.avahi != a.avahiBaseline {
		return true
	}
	return !reflectMounts0(a.shares, a.sharesBaseline)
}

func reflectMounts0(x, y map[string]profile.ShareSpec) bool {
	if len(x) != len(y) {
		return false
	}
	for k, s := range x {
		b, ok := y[k]
		if !ok || b != s {
			return false
		}
	}
	return true
}

func (a *smbAdapter) Block() string {
	return profile.EncodeSmbSection(*a.Smb())
}

// Errors reuses ShareChoice's own gate live (name and path required).
func (a *smbAdapter) Errors() []string {
	var out []string
	for _, name := range a.shareNames {
		if a.shares[name].Path == "" {
			out = append(out, name+": path is required")
		}
	}
	return out
}

// ReplaceShares absorbs a whole [smb] section through generate's shared
// SmbInput assembly — the machines' write-once, wholesale-replace
// semantics. users carries D3's annotation in the view: the list becomes
// OS accounts at apply.
func (a *smbAdapter) ReplaceShares(server generate.SmbServerChoice, shares map[string]profile.ShareSpec, defaultUser string) {
	input := generate.SmbInput(server.Group, server.Users, server.Avahi, shares, defaultUser, 0, 0)
	a.group = input.Smb.Group
	a.users = strings.Join(input.Smb.Users, ", ")
	a.avahi = input.Smb.Avahi
	a.shares = input.Smb.Shares
	a.shareNames = slices.Sorted(maps.Keys(a.shares))
}

// ApplyShare validates and accumulates one share (AddShare's adapter
// counterpart).
func (a *smbAdapter) ApplyShare(c generate.ShareChoice) error {
	if err := c.Validate(); err != nil {
		return err
	}
	a.shares[c.Name] = profile.ShareSpec{Path: c.Path, Comment: c.Comment, Writable: c.Writable, Public: c.Public}
	slices.Sort(a.shareNames)
	return nil
}

func (a *smbAdapter) HandleKey(key string) {
	if a.prompting {
		if a.prompt.handleEditKey(key) && !a.prompt.edit {
			if name := strings.TrimSpace(a.prompt.String()); name != "" {
				if _, ok := a.shares[name]; !ok {
					a.shares[name] = profile.ShareSpec{}
					a.shareNames = append(a.shareNames, name)
					slices.Sort(a.shareNames)
				}
				a.cur = slices.Index(a.shareNames, name)
				a.openShareForm(name)
			}
			a.prompting = false
		}
		return
	}
	if a.inForm {
		if a.shareFields[a.formFocus].edit {
			a.shareFields[a.formFocus].handleEditKey(key)
			a.commitShareForm()
			return
		}
		switch key {
		case "j", "down":
			a.formFocus = (a.formFocus + 1) % len(a.shareFields)
		case "k", "up":
			a.formFocus = (a.formFocus + len(a.shareFields) - 1) % len(a.shareFields)
		case "enter":
			a.shareFields[a.formFocus].beginEdit()
		case "esc":
			a.inForm = false
		}
		return
	}
	switch {
	case key == "j" || key == "down":
		if a.cur < len(a.shareNames)-1 {
			a.cur++
		}
	case key == "k" || key == "up":
		if a.cur > 0 {
			a.cur--
		}
	case key == "enter":
		if a.cur < len(a.shareNames) {
			a.openShareForm(a.shareNames[a.cur])
		}
	case key == "n":
		a.prompting = true
		a.prompt = newField("name", "share", "")
		a.prompt.beginEdit()
	case key == "x":
		if a.cur < len(a.shareNames) {
			delete(a.shares, a.shareNames[a.cur])
			a.shareNames = slices.Delete(a.shareNames, a.cur, a.cur+1)
			if a.cur >= len(a.shareNames) && a.cur > 0 {
				a.cur--
			}
		}
	}
}

func (a *smbAdapter) openShareForm(name string) {
	a.inForm = true
	a.formFocus = 0
	s := a.shares[name]
	for i, spec := range smbFieldSpecs {
		a.shareFields[i] = newField(spec.key, spec.label, spec.pick(s))
	}
	a.shareFields[0].edit = true
}

func (a *smbAdapter) commitShareForm() {
	s := a.shares[a.shareNames[a.cur]]
	for i, spec := range smbFieldSpecs {
		spec.apply(&s, a.shareFields[i].String())
	}
	a.shares[a.shareNames[a.cur]] = s
}

func (a *smbAdapter) View(p Palette) string {
	if a.prompting {
		return p.Meta("share name: ") + renderField(p, &a.prompt) + p.Meta("  enter confirm · esc cancel")
	}
	var b strings.Builder
	b.WriteString(p.Meta("group ") + a.group + p.Meta(" · users ") + a.users +
		p.Meta("  (users become OS accounts at apply)") + "\n")
	if a.inForm {
		b.WriteString(p.Label(a.shareNames[a.cur]) + p.Meta(" — share") + "\n")
		for i, spec := range smbFieldSpecs {
			cur := "  "
			if i == a.formFocus {
				cur = "> "
			}
			b.WriteString(cur + spec.label + " · " + renderField(p, &a.shareFields[i]) + "\n")
		}
		return b.String()
	}
	if len(a.shareNames) == 0 {
		b.WriteString(p.Meta("no shares — n adds one"))
		return b.String()
	}
	for i, name := range a.shareNames {
		s := a.shares[name]
		cur := "  "
		if i == a.cur {
			cur = "> "
		}
		line := cur + name + " → " + s.Path
		if s.Writable {
			line += " (rw)"
		}
		if i == a.cur {
			line = p.Mark(line)
		}
		b.WriteString(line + "\n")
	}
	return b.String()
}
