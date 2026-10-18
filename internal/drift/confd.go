package drift

import (
	"os"
	"path/filepath"
	"strings"

	"github.com/thedataflows/dotdrift/internal/mise"
	"github.com/thedataflows/dotdrift/internal/resolve"
)

// CheckConfD lists the files in dir (mise's global conf.d) that the plan
// does not manage, for information (issue 0139). A file is managed when
// it is the snippet the plan writes: <module>.toml for a module with
// resolved tools. Everything else is Unmanaged — a user file, or a
// dotdrift snippet whose module is not in this plan. Apply never
// deletes in conf.d (issue 0138), so this list is how stale snippets
// surface. A missing or unreadable directory lists nothing.
func CheckConfD(plan *resolve.Plan, dir string) []Finding {
	if plan == nil || dir == "" {
		return nil
	}
	managed := make(map[string]bool)
	for mod, tools := range plan.Tools.ModuleTools {
		if len(tools) > 0 {
			managed[filepath.Base(mod)+".toml"] = true
		}
	}
	entries, err := os.ReadDir(dir) // sorted by file name
	if err != nil {
		return nil
	}
	var out []Finding
	for _, e := range entries {
		if e.IsDir() || managed[e.Name()] {
			continue
		}
		f := Finding{Section: "tools", Item: "conf.d/" + e.Name(), Status: Unmanaged,
			Detail: "not written by dotdrift"}
		if b, err := os.ReadFile(filepath.Join(dir, e.Name())); err == nil && mise.HasGeneratedMarker(b) {
			f.Module = strings.TrimSuffix(e.Name(), ".toml")
			f.Detail = "dotdrift snippet, but its module declares no tools in this plan — delete the file to stop its tools from resolving"
		}
		out = append(out, f)
	}
	return out
}
