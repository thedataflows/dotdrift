package drift

import (
	"os"
)

// Source-validity probes (issue: symlink/symlink-each sources must still
// exist). Stat reports whether a path exists (file or directory; not-exist
// is (false, nil), other errors propagate). ListDir returns the direct
// child names of a directory. Both live on Probes so tests can stub them;
// callers nil-guard when a check is optional.

func defaultStat(path string) (bool, error) {
	if _, err := os.Stat(path); err != nil {
		if os.IsNotExist(err) {
			return false, nil
		}
		return false, err
	}
	return true, nil
}

func defaultListDir(path string) ([]string, error) {
	entries, err := os.ReadDir(path)
	if err != nil {
		return nil, err
	}
	names := make([]string, 0, len(entries))
	for _, e := range entries {
		names = append(names, e.Name())
	}
	return names, nil
}
