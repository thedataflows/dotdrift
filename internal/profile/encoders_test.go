package profile

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

// The per-section encoders (0065-D7 layer 2): typed section values →
// canonical TOML text. Deterministic (sorted keyed tables, preserved
// arrays), splice-ready (each block stands alone under its family), and a
// canonical fixed point: encode ∘ decode ∘ encode = encode.

func TestEncodeKeysSection(t *testing.T) {
	require.Equal(t, "id = \"zsh\"\n", EncodeKeysSection(ModuleConfig{ID: "zsh"}),
		"minimal scaffold: zero values omitted")
	require.Equal(t,
		"id = \"zsh\"\ndescription = \"my shell\"\ndisabled = true\nscope = \"system\"\n",
		EncodeKeysSection(ModuleConfig{ID: "zsh", Description: "my shell", Disabled: true, Scope: "system"}))
	require.Equal(t, "", EncodeKeysSection(ModuleConfig{}),
		"omitted id and scope stay omitted")
}

func TestEncodePackagesSection(t *testing.T) {
	require.Equal(t, "[packages]\npresent = [\n  \"ripgrep\",\n  \"fd\",\n]\n",
		EncodePackagesSection([]PackageEntry{{Name: "ripgrep"}, {Name: "fd"}}, nil),
		"order preserved, one per line, trailing commas")
	require.Equal(t, "[packages]\npresent = [\n  \"ripgrep\", # the good one\n]\n",
		EncodePackagesSection([]PackageEntry{{Name: "ripgrep", Description: "the good one"}}, nil),
		"description comments render next to the entry")
	require.Equal(t, "[packages]\nabsent = [\n  \"vim\",\n]\n",
		EncodePackagesSection(nil, []string{"vim"}), "absent list stands alone")
	require.Equal(t, "", EncodePackagesSection(nil, nil), "nothing declared: no section")
}

func TestEncodeToolsSection(t *testing.T) {
	require.Equal(t, "[tools]\nfd = \"9\"\nripgrep = \"latest\"\n",
		EncodeToolsSection(map[string]string{"ripgrep": "latest", "fd": "9"}),
		"keys sorted")
	require.Equal(t, "", EncodeToolsSection(nil), "nothing declared: no section")
	require.Equal(t, "[tools]\n\"weird.key\" = \"1\"\n", EncodeToolsSection(map[string]string{"weird.key": "1"}),
		"non-bare keys quoted")
}

func TestEncodeSecretsSection(t *testing.T) {
	require.Equal(t, "[secrets]\ntoken = \"MISE_TOKEN\"\n",
		EncodeSecretsSection(map[string]Secret{"token": {Env: "MISE_TOKEN"}}),
		"short form when only env is set")
	require.Equal(t,
		"[secrets]\ncache = { env = \"MISE_CACHE\", description = \"the cache\", allow_empty = true }\n",
		EncodeSecretsSection(map[string]Secret{"cache": {Env: "MISE_CACHE", Description: "the cache", AllowEmpty: true}}),
		"table form when description or allow_empty is set")
	require.Equal(t, "[secrets]\na = \"A\"\nb = \"B\"\n",
		EncodeSecretsSection(map[string]Secret{"b": {Env: "B"}, "a": {Env: "A"}}),
		"sorted names; env-only entries all take the short form")
	require.Equal(t, "", EncodeSecretsSection(nil), "nothing declared: no section")
}

func TestEncodeDotfilesSection(t *testing.T) {
	require.Equal(t,
		"[dotfiles]\n\"~/.a/line\" = { line = \"set -x\" }\n\"~/.zshrc\" = { source = \"zshrc\", mode = \"symlink\" }\n",
		EncodeDotfilesSection(map[string]Dotfile{
			"~/.zshrc":  {Source: "zshrc", Mode: "symlink"},
			"~/.a/line": {Line: "set -x"},
		}),
		"sorted keys, inline tables, zero fields omitted")
	require.Equal(t, "", EncodeDotfilesSection(nil), "nothing declared: no section")
}

func TestEncodeWhenSection(t *testing.T) {
	require.Equal(t, "[when]\nhosts = [\"box\"]\n",
		EncodeWhenSection(When{Hosts: []string{"box"}}))
	require.Equal(t, "[when]\nkernel = \">= 6.1\"\npackages = [\"docker.*\"]\n",
		EncodeWhenSection(When{Kernel: ">= 6.1", Packages: []string{"docker.*"}}),
		"leaf order: hosts, users, os, gpu, kernel, packages, tools")
	require.Equal(t,
		"[when]\nand = [{ hosts = [\"a\"] }, { gpu = \"nvidia\" }]\nnot = { packages = [\"vim\"] }\n",
		EncodeWhenSection(When{
			And: []When{{Hosts: []string{"a"}}, {GPU: "nvidia"}},
			Not: &When{Packages: []string{"vim"}},
		}),
		"combinators encode as inline tables")
	require.Equal(t, "", EncodeWhenSection(When{}), "empty when: no section")
}

func TestEncodeHooksSection(t *testing.T) {
	require.Equal(t, "[hooks]\npre = [\"echo hi\"]\n",
		EncodeHooksSection([]HookRow{{Command: "echo hi"}}, nil),
		"an all-plain list keeps the concise array spelling")
	require.Equal(t,
		"[hooks]\npre = [\"a\"]\npost = [{ command = \"b\", optional = true }]\n",
		EncodeHooksSection([]HookRow{{Command: "a"}}, []HookRow{{Command: "b", Optional: true, Structured: true}}),
		"post after pre; a structured row takes the inline-table spelling")
	require.Equal(t,
		"[hooks]\npost = [{ command = \"b\", optional = true }]\n",
		EncodeHooksSection(nil, []HookRow{{Command: "b", Optional: true, Structured: true}}),
		"a structured row encodes as an inline table")
	require.Equal(t, "", EncodeHooksSection(nil, nil), "nothing declared: no section")
}

func TestEncodeMountsSection(t *testing.T) {
	require.Equal(t,
		"[mounts.data]\nsource = \"tank:/x\"\ndestination = \"/mnt/x\"\ntype = \"nfs\"\nstate = \"enabled\"\n",
		EncodeMountsSection(map[string]MountSpec{"data": {
			Source: "tank:/x", Destination: "/mnt/x", Type: "nfs", State: "enabled",
		}}),
		"field order source/destination/type/options/startat/state, zeros omitted")
	require.Equal(t,
		"[mounts.a]\nsource = \"//h/s\"\ndestination = \"/mnt/s\"\ntype = \"cifs\"\noptions = [\"ro\", \"nofail\"]\n",
		EncodeMountsSection(map[string]MountSpec{"a": {
			Source: "//h/s", Destination: "/mnt/s", Type: "cifs", Options: []string{"ro", "nofail"},
		}}))
	require.Equal(t, "", EncodeMountsSection(nil), "nothing declared: no section")
}

func TestEncodeSmbSection(t *testing.T) {
	avahiFalse := false
	require.Equal(t,
		"[smb]\ngroup = \"media\"\nusers = [\"kim\"]\navahi = false\n\n[smb.shares.media]\npath = \"/srv/media\"\nwritable = true\n",
		EncodeSmbSection(SmbSpec{
			Group: "media", Users: []string{"kim"}, Avahi: &avahiFalse,
			Shares: map[string]ShareSpec{"media": {Path: "/srv/media", Writable: true}},
		}),
		"server table first, shares as keyed sub-tables, zeros omitted")
	require.Equal(t, "", EncodeSmbSection(SmbSpec{}), "nothing declared: no section")
}

func TestEncodeSystemdSection(t *testing.T) {
	require.Equal(t,
		"[systemd.units.\"app.service\"]\nEnvironment = \"KEY=1\"\n\n[systemd.units.\"app.timer\"]\nOnCalendar = \"daily\"\n",
		EncodeSystemdSection(map[string]SystemdUnit{
			"app.timer":   {"OnCalendar": "daily"},
			"app.service": {"Environment": "KEY=1"},
		}),
		"unit names quoted (they carry dots), sorted, directive passthrough")
	require.Equal(t,
		"[systemd.units.\"x.service\"]\nWantedBy = \"default.target\"\n",
		EncodeSystemdSection(map[string]SystemdUnit{"x.service": {"WantedBy": "default.target"}}))
	require.Equal(t, "", EncodeSystemdSection(nil), "nothing declared: no section")
}

// TestEncode_canonicalFixedPoint: encode ∘ decode ∘ encode = encode, for a
// config exercising every section. The save pipeline's strict decode makes
// this a test-mandated invariant (0065-D8).
func TestEncode_canonicalFixedPoint(t *testing.T) {
	avahi := true
	cfg := ModuleConfig{
		ID: "full", Description: "every section", Disabled: true, Scope: ScopeSystem,
		When: When{
			Hosts: []string{"box"}, Users: []string{"kim"}, OS: []string{"arch"}, GPU: "nvidia",
			Kernel: ">= 6.1", Packages: []string{"docker.*"}, Tools: []string{"node"},
			And: []When{{Hosts: []string{"box"}}},
			Or:  []When{{GPU: "amd"}, {GPU: "nvidia"}},
			Not: &When{Packages: []string{"vim"}},
		},
		Packages: Packages{Present: []string{"ripgrep", "fd"}, Absent: []string{"vim"}},
		Tools:    map[string]string{"node": "22"},
		Secrets: map[string]Secret{
			"token": {Env: "MISE_TOKEN"},
			"cache": {Env: "MISE_CACHE", Description: "d", AllowEmpty: true},
		},
		Dotfiles: map[string]Dotfile{
			"~/.zshrc":  {Source: "zshrc", Mode: "symlink"},
			"~/.a/edit": {Source: "e.sh", Line: "set -x"},
		},
		Hooks: Hooks{
			Pre:  []HookCommand{{Command: "echo pre"}},
			Post: []HookCommand{{Command: "echo post", Optional: true}},
		},
		Mounts: map[string]MountSpec{"data": {
			Source: "tank:/x", Destination: "/mnt/x", Type: "nfs",
			Options: []string{"ro"}, StartAt: "boot", State: "enabled",
		}},
		Smb: SmbSpec{
			Group: "media", Users: []string{"kim"}, Avahi: &avahi,
			Shares: map[string]ShareSpec{"media": {Path: "/srv/media", Comment: "c", ValidUsers: "@g", Writable: true}},
		},
		Systemd: SystemdSpec{Units: map[string]SystemdUnit{
			"app.service": {"Environment": "KEY=1", "WantedBy": "default.target"},
		}},
	}

	encodeAll := func(c ModuleConfig) map[string]string {
		blocks := map[string]string{
			FamilyKeys:     EncodeKeysSection(c),
			FamilyPackages: EncodePackagesSection(PackageEntries(c.Packages.Present), c.Packages.Absent),
			FamilyTools:    EncodeToolsSection(c.Tools),
			FamilySecrets:  EncodeSecretsSection(c.Secrets),
			FamilyDotfiles: EncodeDotfilesSection(c.Dotfiles),
			FamilyWhen:     EncodeWhenSection(c.When),
			FamilyHooks:    EncodeHooksSection(HookRows(c.Hooks.Pre), HookRows(c.Hooks.Post)),
			FamilyMounts:   EncodeMountsSection(c.Mounts),
			FamilySmb:      EncodeSmbSection(c.Smb),
			FamilySystemd:  EncodeSystemdSection(c.Systemd.Units),
		}
		return blocks
	}

	first := encodeAll(cfg)
	var doc strings.Builder
	for _, fam := range []string{FamilyKeys, FamilyPackages, FamilyTools, FamilySecrets,
		FamilyDotfiles, FamilyWhen, FamilyHooks, FamilyMounts, FamilySmb, FamilySystemd} {
		if first[fam] != "" {
			doc.WriteString("\n" + first[fam])
		}
	}

	var decoded ModuleConfig
	require.NoError(t, DecodeModuleTOML("module.toml", []byte(doc.String()), &decoded),
		"the encoded document must strict-decode")
	second := encodeAll(decoded)
	for _, fam := range []string{FamilyKeys, FamilyPackages, FamilyTools, FamilySecrets,
		FamilyDotfiles, FamilyWhen, FamilyHooks, FamilyMounts, FamilySmb, FamilySystemd} {
		require.Equal(t, first[fam], second[fam], "section %s must be a canonical fixed point", fam)
	}
}

func TestEncode_keyedTablesSorted(t *testing.T) {
	// Every keyed-table family emits entries in sorted name order; order
	// is data, not map accident.
	mounts := EncodeMountsSection(map[string]MountSpec{
		"zeta":  {Source: "z", Destination: "/z", Type: "nfs"},
		"alpha": {Source: "a", Destination: "/a", Type: "nfs"},
	})
	require.Less(t, strings.Index(mounts, "[mounts.alpha]"), strings.Index(mounts, "[mounts.zeta]"))

	units := EncodeSystemdSection(map[string]SystemdUnit{
		"z.service": {"A": 1}, "a.service": {"B": 2},
	})
	require.Less(t, strings.Index(units, "\"a.service\""), strings.Index(units, "\"z.service\""))

	dotfiles := EncodeDotfilesSection(map[string]Dotfile{
		"~/.z": {Source: "z", Mode: "symlink"}, "~/.a": {Source: "a", Mode: "symlink"},
	})
	require.Less(t, strings.Index(dotfiles, "\"~/.a\""), strings.Index(dotfiles, "\"~/.z\""))
}

func TestEncode_orderedArraysPreserveOrder(t *testing.T) {
	pkgs := EncodePackagesSection([]PackageEntry{{Name: "b"}, {Name: "a"}, {Name: "m"}}, nil)
	require.Equal(t, "[packages]\npresent = [\n  \"b\",\n  \"a\",\n  \"m\",\n]\n", pkgs,
		"package order is the declaration's data")

	hooks := EncodeHooksSection([]HookRow{{Command: "second"}, {Command: "first"}}, nil)
	require.Equal(t, "[hooks]\npre = [\"second\", \"first\"]\n", hooks,
		"hook order is the declaration's data")

	w := EncodeWhenSection(When{Hosts: []string{"z", "a"}})
	require.Equal(t, "[when]\nhosts = [\"z\", \"a\"]\n", w, "when list order is data")
}

func TestEncode_whenGrammarRoundTrip(t *testing.T) {
	// Deeply nested combinators survive the encode ∘ decode round trip.
	w := When{
		And: []When{{Or: []When{{Kernel: ">= 6.1"}, {Not: &When{GPU: "nvidia"}}}}},
	}
	encoded := EncodeWhenSection(w)
	var back ModuleConfig
	require.NoError(t, DecodeModuleTOML("module.toml", []byte(encoded), &back))
	require.Equal(t, encoded, EncodeWhenSection(back.When), "nested when round-trips")
	require.NoError(t, ValidateWhen("m", back.When), "the encoded when must pass the grammar")
}
