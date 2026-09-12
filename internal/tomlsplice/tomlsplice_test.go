package tomlsplice_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/BurntSushi/toml"
	"github.com/stretchr/testify/require"
	"github.com/thedataflows/dotdrift/internal/profile"
	"github.com/thedataflows/dotdrift/internal/tomlsplice"
)

// The splicer's four properties (0065-D11): untouched sections byte-identical,
// determinism, multi-line-string immunity, and spliced output that still
// strict-decodes. The splice is text mechanics only — no schema knowledge.

const fixtureDoc = `# hand-written module: zsh
# managed carefully, comments matter
id = "zsh"
app = "zsh"

[packages] # trailing header comment
present = [
  "ripgrep", # cosmetic description lives in a comment
  "fd",
]

[tools]
ripgrep = "latest"
# a comment between tools survives too
fd = "latest"

	[when]
gpu = """
[nvidia]
fake header line
"""

[dotfiles]
"~/.zshrc" = { source = "zshrc", mode = "symlink" }

[smb]
group = "media"
`

// wantSection extracts one section's exact bytes (header + body up to the
// next header) from a document.
func wantSection(t *testing.T, doc, header string) string {
	t.Helper()
	lines := strings.Split(doc, "\n")
	var start int
	for i, l := range lines {
		if strings.TrimSpace(l) == header {
			start = i
			break
		}
	}
	end := len(lines)
	for i := start + 1; i < len(lines); i++ {
		s := strings.TrimSpace(lines[i])
		if strings.HasPrefix(s, "[") && strings.HasSuffix(s, "]") {
			end = i
			break
		}
	}
	return strings.Join(lines[start:end], "\n")
}

func TestSplice_untouchedSectionsByteIdentical(t *testing.T) {
	newPackages := "[packages]\npresent = [\n  \"ripgrep\",\n  \"fd\",\n  \"bat\",\n]\n"
	out := tomlsplice.Splice(fixtureDoc, map[string]string{"packages": newPackages})

	// The replaced section carries the new content, verbatim.
	require.True(t, strings.Contains(out, newPackages[:30]), "spliced packages block missing")
	// The preamble (module keys + their comments) is byte-identical.
	wantPreamble := `# hand-written module: zsh
# managed carefully, comments matter
id = "zsh"
app = "zsh"
`
	require.Equal(t, wantPreamble, strings.Split(out, "\n[")[0], "preamble must pass through verbatim")
	// Every other section is byte-identical, position included.
	require.Equal(t, wantSection(t, fixtureDoc, "[tools]"), wantSection(t, out, "[tools]"),
		"untouched [tools] must be byte-identical")
	require.Equal(t, wantSection(t, fixtureDoc, "[when]"), wantSection(t, out, "[when]"),
		"untouched [when] must be byte-identical")
	require.Equal(t, wantSection(t, fixtureDoc, "[dotfiles]"), wantSection(t, out, "[dotfiles]"),
		"untouched [dotfiles] must be byte-identical")
	require.Equal(t, wantSection(t, fixtureDoc, "[smb]"), wantSection(t, out, "[smb]"),
		"untouched [smb] must be byte-identical")
	// The replaced section stays where it was: [packages] still precedes [tools].
	require.True(t, strings.Index(out, "[packages]") < strings.Index(out, "[tools]"),
		"replaced section keeps its position")
}

func TestSplice_deterministic(t *testing.T) {
	repl := map[string]string{
		"tools":    "[tools]\nfd = \"9\"\n",
		"packages": "[packages]\npresent = [\"fd\"]\n",
		"dotfiles": "[dotfiles]\n\"~/.zshrc\" = { source = \"zshrc\", mode = \"copy\" }\n",
	}
	first := tomlsplice.Splice(fixtureDoc, repl)
	for range 20 {
		require.Equal(t, first, tomlsplice.Splice(fixtureDoc, repl),
			"splice must be deterministic regardless of map order")
	}
}

func TestSplice_multilineStringImmunity(t *testing.T) {
	// The [when] body holds a multi-line basic string whose content looks
	// exactly like a table header. The header grammar must track the string
	// state: the fake header is body, and splicing another family leaves it
	// untouched.
	out := tomlsplice.Splice(fixtureDoc, map[string]string{
		"tools": "[tools]\nfd = \"9\"\n",
	})
	require.Equal(t, wantSection(t, fixtureDoc, "[when]"), wantSection(t, out, "[when]"),
		"the multi-line string's fake header must not disturb the section map")

	// Splicing the family that OWNS the multi-line string replaces all of it.
	out = tomlsplice.Splice(fixtureDoc, map[string]string{
		"when": "[when]\ngpu = \"amd\"\n",
	})
	require.True(t, !strings.Contains(out, "fake header line"),
		"replaced family must not leave multi-line remnants")
	require.True(t, !strings.Contains(out, "[nvidia]"), "stale content must be gone")
	// And the untouched sections around it are still intact.
	require.Equal(t, wantSection(t, fixtureDoc, "[dotfiles]"), wantSection(t, out, "[dotfiles]"))
}

func TestSplice_outputStrictDecodes(t *testing.T) {
	// Whatever the splicer emits must remain a valid module.toml — the save
	// pipeline's strict decode (contract 19) sits right behind it.
	repl := map[string]string{
		"keys":     "id = \"zsh\"\napp = \"zsh\"\ndescription = \"shell\"\ndisabled = true\nscope = \"system\"\n",
		"packages": "[packages]\npresent = [\n  \"bat\",\n]\nabsent = [\n  \"vim\",\n]\n",
		"tools":    "[tools]\nnode = \"22\"\n",
		"when":     "[when]\nhosts = [\"box\"]\n",
		"dotfiles": "[dotfiles]\n\"~/.zshrc\" = { source = \"zshrc\", mode = \"symlink\" }\n",
		"hooks":    "[[hooks.pre]]\ncommand = \"echo hi\"\n\n[[hooks.post]]\ncommand = \"echo bye\"\noptional = true\n",
		"secrets":  "[secrets]\ntoken = \"MISE_TOKEN\"\n",
		"mounts":   "[mounts.data]\nsource = \"tank:/x\"\ndestination = \"/mnt/x\"\ntype = \"nfs\"\nstate = \"enabled\"\n",
		"smb":      "[smb]\ngroup = \"smb\"\nusers = [\"kim\"]\n\n[smb.shares.media]\npath = \"/srv/media\"\nwritable = true\n",
		"systemd":  "[systemd.units.\"app.service\"]\nEnvironment = \"KEY=1\"\n",
	}
	out := tomlsplice.Splice(fixtureDoc, repl)
	require.True(t, !strings.Contains(out, "\"fd\" = \"latest\""), "replaced families must be rebuilt")

	path := filepath.Join(t.TempDir(), "module.toml")
	require.NoError(t, os.WriteFile(path, []byte(out), 0o644))
	var cfg profile.ModuleConfig
	require.NoError(t, profile.DecodeModuleTOML(path, []byte(out), &cfg), "spliced output must strict-decode")
	require.Equal(t, "zsh", cfg.ID)
	require.Equal(t, true, cfg.Disabled)
	require.Equal(t, []string{"bat"}, cfg.Packages.Present)
	require.Equal(t, "22", cfg.Tools["node"])
	require.Equal(t, []string{"box"}, cfg.When.Hosts)
	require.Equal(t, "MISE_TOKEN", cfg.Secrets["token"].Env)
	require.Equal(t, "tank:/x", cfg.Mounts["data"].Source)
	require.Equal(t, "/srv/media", cfg.Smb.Shares["media"].Path)
	require.Equal(t, "KEY=1", cfg.Systemd.Units["app.service"]["Environment"])
	require.Equal(t, 1, len(cfg.Hooks.Pre))
	require.Equal(t, true, cfg.Hooks.Post[0].Optional)

	// Round-trip stability: decode → the same decode again over the same
	// text is trivially stable, but the text must also be a fixed point of
	// splicing with the same blocks (idempotent splice).
	again := tomlsplice.Splice(out, repl)
	require.Equal(t, out, again, "splice must be idempotent for identical replacements")
}

func TestSplice_newAndRemovedFamilies(t *testing.T) {
	// A family the file does not have yet is appended (the scaffold path:
	// sections are added through the editors themselves).
	out := tomlsplice.Splice(fixtureDoc, map[string]string{
		"mounts": "[mounts.media]\nsource = \"/dev/sdb1\"\ndestination = \"/mnt/media\"\ntype = \"ext4\"\n",
	})
	require.True(t, strings.Contains(out, "[mounts.media]"), "new family appended")
	require.Equal(t, wantSection(t, fixtureDoc, "[tools]"), wantSection(t, out, "[tools]"))

	// An empty block removes the family's sections entirely.
	out = tomlsplice.Splice(fixtureDoc, map[string]string{"tools": ""})
	require.True(t, !strings.Contains(out, "fd = \"latest\""), "emptied family must vanish")
	require.True(t, !strings.Contains(out, "[tools]"), "its header goes too")
	require.Equal(t, wantSection(t, fixtureDoc, "[packages]"), wantSection(t, out, "[packages]"))
}

func TestSplit_headerGrammar(t *testing.T) {
	// The preamble, every section's family, and multi-line awareness.
	secs := tomlsplice.Split(fixtureDoc)
	var fams []string
	for _, s := range secs {
		fams = append(fams, s.Family)
	}
	// preamble (module keys), packages, tools, when, dotfiles, smb.
	require.Equal(t, []string{tomlsplice.FamilyKeys, "packages", "tools", "when", "dotfiles", "smb"}, fams)

	// Quoted keys and trailing comments are headers; assignment lines,
	// array rows, and lines inside multi-line strings are body.
	tricky := "a = 1\n" +
		"[dotfiles.\"~/.zshrc\"] # ok\n" +
		"x = { block = \"[nope]\" }\n" +
		"desc = \"\"\"\n" +
		"[still-body]\n" +
		"\"\"\"\n" +
		"[tools]\n" +
		"k = 'v'\n"
	secs = tomlsplice.Split(tricky)
	var got []string
	for _, s := range secs {
		got = append(got, s.Family)
	}
	require.Equal(t, []string{tomlsplice.FamilyKeys, "dotfiles", "tools"}, got,
		"inline tables and multi-line string content are body, never headers")
}

// The dotfiles sub-table spelling ([dotfiles."~/.x"]) belongs to the
// dotfiles family, so an editor saving [dotfiles] replaces both spellings.
func TestSplice_subTableFamilies(t *testing.T) {
	doc := "[packages]\npresent = [\"fd\"]\n\n[dotfiles]\n\"~/.a\" = { source = \"a\", mode = \"symlink\" }\n\n[dotfiles.\"~/.b\"]\nsource = \"b\"\nmode = \"copy\"\n"
	out := tomlsplice.Splice(doc, map[string]string{
		"dotfiles": "[dotfiles]\n\"~/.c\" = { source = \"c\", mode = \"symlink\" }\n",
	})
	require.True(t, !strings.Contains(out, "\"~/.a\""), "old inline spelling replaced")
	require.True(t, !strings.Contains(out, "[dotfiles.\"~/.b\"]"), "old sub-table spelling replaced")
	require.True(t, strings.Contains(out, "\"~/.c\""), "new block in place")
	require.True(t, strings.Contains(out, "[packages]"), "other families intact")

	var cfg profile.ModuleConfig
	if _, err := toml.Decode(out, &cfg); err != nil {
		t.Fatalf("spliced output must decode: %v", err)
	}
}
