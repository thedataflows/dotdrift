package editor

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/thedataflows/dotdrift/internal/generate"
	"github.com/thedataflows/dotdrift/internal/profile"
	"github.com/thedataflows/dotdrift/internal/service"
)

// The frame's mandated behaviors (0065-D8/D11): file-scoped drafts (one
// splice per save), deep-compare dirty tracking, the dirty-confirm guard,
// and the three validation tiers. The adapter vectors reuse profile's and
// generate's own checks.

// fakeConfig records WriteModuleLayer calls; saveErr, when set, is what
// the save pipeline reports (the tier-3 authority).
type fakeConfig struct {
	read    *service.ModuleLayerRead
	saveErr error
	calls   []service.SaveRequest
}

func (f *fakeConfig) ReadModuleLayer(dir string) (*service.ModuleLayerRead, error) {
	r := *f.read
	r.Dir = dir
	return &r, nil
}

func (f *fakeConfig) WriteModuleLayer(req service.SaveRequest) (*service.SaveResult, error) {
	f.calls = append(f.calls, req)
	if f.saveErr != nil {
		return nil, f.saveErr
	}
	return &service.SaveResult{Hash: "next", Raw: "saved"}, nil
}

// testRegistry is the embedded generate registry, loaded once.
func testRegistry() *generate.Registry {
	reg, err := generate.Load()
	if err != nil {
		panic(err)
	}
	return reg
}

func draftFixture() *service.ModuleLayerRead {
	return &service.ModuleLayerRead{
		Dir:  "/profile/modules/zsh",
		Path: "/profile/modules/zsh/module.toml",
		Hash: "base0",
		Config: &profile.ModuleConfig{
			ID:  "zsh",
			App: "zsh",
			Packages: profile.Packages{
				Present: []string{"ripgrep"},
				Absent:  []string{"nano"},
			},
			Tools: map[string]string{"fd": "9"},
		},
	}
}

func newTestFrame(fc *fakeConfig) *Frame {
	return NewFrame(fc, nil, "/profile/modules/zsh")
}

func (f *Frame) section(id string) Adapter {
	for _, s := range f.sections {
		if s.ID() == id {
			return s
		}
	}
	return nil
}

func TestFrame_draftIsFileScoped(t *testing.T) {
	fc := &fakeConfig{read: draftFixture()}
	f := newTestFrame(fc)

	// Two sections of the SAME file get dirty.
	pkg := f.section("packages").(*packagesAdapter)
	pkg.rows = append(pkg.rows, pkgRow{Name: "bat"})
	tools := f.section("tools").(*toolsAdapter)
	fl := tools.values["fd"]
	fl.set("10")
	tools.values["fd"] = fl

	staged := f.Staged()
	require.Len(t, staged, 2, "both dirty families stage together")
	require.Contains(t, staged, "packages")
	require.Contains(t, staged, "tools")

	require.NoError(t, f.Save())
	require.Len(t, fc.calls, 1, "one file, one splice — never one call per section")
	require.Equal(t, "base0", fc.calls[0].BaseHash, "the save baselines the draft's read hash")
	require.Len(t, fc.calls[0].Replacements, 2)
}

func TestFrame_dirtyDeepCompare(t *testing.T) {
	fc := &fakeConfig{read: draftFixture()}
	f := newTestFrame(fc)
	keys := f.section("keys").(*keysAdapter)

	require.False(t, f.Dirty(), "a fresh draft is clean")

	// Setting a field to exactly its baseline value changes nothing.
	keys.description.set("")
	require.False(t, keys.Dirty(), "deep-compare: identical values are not dirty")

	keys.description.set("my shell")
	require.True(t, keys.Dirty())
	require.True(t, f.Dirty())

	// And back again: clean.
	keys.description.set("")
	require.False(t, f.Dirty())

	// A bool toggle round-trips the same way.
	keys.disabled = true
	require.True(t, f.Dirty())
	keys.disabled = false
	require.False(t, f.Dirty())
}

func TestFrame_dirtyConfirmGuardsEscQ(t *testing.T) {
	fc := &fakeConfig{read: draftFixture()}
	f := newTestFrame(fc)

	// Clean esc asks to leave immediately.
	f.HandleKey("esc")
	require.True(t, f.PopRequested())
	require.False(t, f.Dirty())

	// Dirty esc shows the confirm instead of leaving.
	f2 := newTestFrame(&fakeConfig{read: draftFixture()})
	keys := f2.section("keys").(*keysAdapter)
	keys.description.set("my shell")
	f2.HandleKey("esc")
	require.False(t, f2.PopRequested(), "dirty esc must not leave")
	require.True(t, f2.confirm, "the dirty-confirm guard shows")

	// Cancelling keeps editing.
	f2.HandleKey("esc")
	require.False(t, f2.confirm)
	require.True(t, f2.Dirty())
	require.False(t, f2.PopRequested())

	// Discard reverts and leaves.
	f2.HandleKey("esc")
	f2.HandleKey("d")
	require.False(t, f2.Dirty(), "discard reverts to the baseline")
	require.True(t, f2.PopRequested())

	// Save-then-leave saves exactly once.
	f3 := newTestFrame(&fakeConfig{read: draftFixture()})
	keys = f3.section("keys").(*keysAdapter)
	keys.description.set("my shell")
	f3.HandleKey("esc")
	f3.HandleKey("s")
	require.False(t, f3.Dirty())
	require.True(t, f3.PopRequested())
}

func TestFrame_validationTiers(t *testing.T) {
	// Tier 1 — live field checks gate the save.
	fc := &fakeConfig{read: draftFixture()}
	f := newTestFrame(fc)
	keys := f.section("keys").(*keysAdapter)
	keys.scope = "bogus"
	err := f.Save()
	require.Error(t, err, "tier 1 refuses the save")
	require.Empty(t, fc.calls, "nothing reached the service")

	// Tier 2 — the debounced cross-checks gate the save: mounts under a
	// user-scope module (resolve's rule, said early).
	fc2 := &fakeConfig{read: draftFixture()}
	f2 := newTestFrame(fc2)
	keys2 := f2.section("keys").(*keysAdapter)
	keys2.scope = profile.ScopeUser
	mounts := f2.section("mounts").(*mountsAdapter)
	require.NoError(t, mounts.ApplyChoice(testRegistry(), generate.MountChoice{
		Name: "data", Source: "tank:/x", Destination: "/mnt/x", Type: "nfs",
	}))
	f2.Tick()
	require.NotEmpty(t, f2.CrossChecks(), "scope vs sections is a cross-check finding")
	err = f2.Save()
	require.Error(t, err, "tier 2 refuses the save")
	require.Empty(t, fc2.calls)

	// Tier 3 — the service save pipeline is the authority: its error
	// surfaces and the draft stays dirty for another attempt.
	fc3 := &fakeConfig{read: draftFixture(), saveErr: errors.New("resolve: module zsh: dotfile target conflict with module other")}
	f3 := newTestFrame(fc3)
	keys3 := f3.section("keys").(*keysAdapter)
	keys3.description.set("my shell")
	err = f3.Save()
	require.Error(t, err, "tier 3 refuses the save")
	require.Len(t, fc3.calls, 1, "the pipeline ran once")
	require.Contains(t, f3.Status(), "target conflict")
	require.True(t, f3.Dirty(), "a refused save keeps the draft")
}

func TestFrame_sectionNav(t *testing.T) {
	fc := &fakeConfig{read: draftFixture()}
	f := newTestFrame(fc)
	require.Equal(t, "keys", f.Current())
	f.HandleKey("]")
	require.Equal(t, "packages", f.Current())
	f.HandleKey("[")
	require.Equal(t, "keys", f.Current())
	// Wrap-around.
	f.HandleKey("[")
	require.Equal(t, "systemd", f.Current())
}

// Adapter field/validator vectors.

func TestKeysAdapter_scopeVocabulary(t *testing.T) {
	a := newKeysAdapter(&profile.ModuleConfig{})
	require.Empty(t, a.Errors(), "the empty scope is the user default")
	for _, s := range []string{"user", "system"} {
		a.scope = s
		require.Empty(t, a.Errors())
	}
	a.scope = "bogus"
	require.Equal(t, []string{"unknown scope \"bogus\" (valid: user, system)"}, a.Errors(),
		"resolve's own scope vocabulary, said live")

	// The key cycles the same vocabulary.
	a.scope = ""
	a.HandleKey("5")
	require.Equal(t, "user", a.scope)
	a.HandleKey("5")
	require.Equal(t, "system", a.scope)
	a.HandleKey("5")
	require.Equal(t, "", a.scope)
}

func TestPackagesAdapter_orderIsData(t *testing.T) {
	cfg := &profile.ModuleConfig{
		Packages: profile.Packages{Present: []string{"ripgrep", "fd"}, Absent: []string{"nano"}},
	}
	a := newPackagesAdapter(cfg)
	require.False(t, a.Dirty())
	require.Equal(t,
		"[packages]\npresent = [\n  \"ripgrep\",\n  \"fd\",\n]\nabsent = [\n  \"nano\",\n]\n",
		a.Block(), "order preserved through the baseline block")

	// Moving a row changes the block, and moving it back un-dirties.
	a.HandleKey(">")
	require.True(t, a.Dirty())
	require.Equal(t,
		"[packages]\npresent = [\n  \"fd\",\n  \"ripgrep\",\n]\nabsent = [\n  \"nano\",\n]\n",
		a.Block())
	a.HandleKey("<")
	require.False(t, a.Dirty())
}

func TestToolsAdapter_sortedAndEditable(t *testing.T) {
	cfg := &profile.ModuleConfig{Tools: map[string]string{"ripgrep": "latest", "fd": "9"}}
	a := newToolsAdapter(cfg)
	require.Equal(t, []string{"fd", "ripgrep"}, a.names, "sorted regardless of map order")
	fl := a.values["fd"]
	fl.set("10")
	a.values["fd"] = fl
	require.True(t, a.Dirty())
	require.Equal(t, "[tools]\nfd = \"10\"\nripgrep = \"latest\"\n", a.Block())
}

func TestSecretsAdapter_spellings(t *testing.T) {
	cfg := &profile.ModuleConfig{Secrets: map[string]profile.Secret{
		"token": {Env: "MISE_TOKEN"},
		"cache": {Env: "MISE_CACHE", Description: "d", AllowEmpty: true},
	}}
	a := newSecretsAdapter(cfg)
	require.Equal(t,
		"[secrets]\ncache = { env = \"MISE_CACHE\", description = \"d\", allow_empty = true }\ntoken = \"MISE_TOKEN\"\n",
		a.Block(), "short and table forms by content")
}

// The contract-15 seam (0066's retarget): for any choice set the flag
// path accepts, the adapters' assembled input equals generate's assembled
// input — the adapters route through generate's own assembly.

func TestGenerate_cliTuiEquivalence_mountsAdapter(t *testing.T) {
	reg := testRegistry()
	choices := []generate.MountChoice{
		{Name: "syn01", Source: "synology.local:/volume1/syn01", Destination: "/mnt/synology/syn01", Type: "nfs", StartAt: "*-*-* 18:05:00"},
		{Name: "data", Source: "UUID=7FFD-B6CF", Destination: "/mnt/ventoy", Type: "exfat", Options: []string{"rw", "nofail"}, State: "disabled"},
	}

	// The wizard path.
	wiz := generate.NewMountsWizard(generate.Selection{}, reg)
	for _, c := range choices {
		require.NoError(t, wiz.AddMount(c))
	}
	want := generate.MountsInput(wiz.Mounts(), 1000, 1000)

	// The adapter path, same choices through ApplyChoice.
	a := newMountsAdapter(&profile.ModuleConfig{})
	for _, c := range choices {
		require.NoError(t, a.ApplyChoice(reg, c))
	}
	got := generate.MountsInput(a.Mounts(), 1000, 1000)

	require.Equal(t, want.Mounts, got.Mounts,
		"the adapter's assembled mounts equal the wizard's for the same choices")

	// The volume prefill rides PrefillForVolume and MountChoice.Validate.
	vol := generate.VolumeChoice{Volume: generate.Volume{UUID: "7FFD-B6CF", FSType: "exfat"}, Name: "ventoy", Destination: "/mnt/ventoy", Type: "exfat"}
	b := newMountsAdapter(&profile.ModuleConfig{})
	require.NoError(t, b.PrefillVolume(reg, vol, generate.MountChoice{}, true))
	require.Contains(t, b.Mounts(), "ventoy")
	require.Equal(t, "UUID=7FFD-B6CF", b.Mounts()["ventoy"].Source,
		"the volume's source derivation is generate's own")
}

func TestGenerate_cliTuiEquivalence_smbAdapter(t *testing.T) {
	// Any share flag set ParseShareFlags accepts.
	raw := []string{"media=/srv/media", "data=/mnt/data"}
	shares, err := generate.ParseShareFlags(raw, true, false)
	require.NoError(t, err)

	// The CLI assembly.
	want := generate.SmbInput("smb", []string{"cri"}, nil, shares, "cri", 1000, 1000)

	// The adapter assembly, fed by the same parsed flags.
	a := newSmbAdapter(&profile.ModuleConfig{})
	a.ReplaceShares(generate.SmbServerChoice{Users: []string{"cri"}}, shares, "cri")
	got := a.Smb()

	require.Equal(t, want.Smb.Group, got.Group, "the default group rides SmbInput")
	require.Equal(t, want.Smb.Users, got.Users, "the default user rides SmbInput")
	require.Equal(t, want.Smb.Shares, got.Shares)

	// ShareChoice.Validate is the same gate the wizard loop uses.
	err = a.ApplyShare(generate.ShareChoice{Name: "bad", Path: ""})
	require.Error(t, err, "a shareless path is refused by generate's own check")
	require.NotContains(t, a.shareNames, "bad")
}

func TestSmbAdapter_usersAnnotationInView(t *testing.T) {
	a := newSmbAdapter(&profile.ModuleConfig{
		Smb: profile.SmbSpec{Group: "media", Users: []string{"kim"}},
	})
	require.Contains(t, a.View(plainPalette{}), "become OS accounts at apply",
		"0065-D3's annotation lives in the smb view")
}
