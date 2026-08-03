package facts

import (
	"fmt"
	"os/exec"
	"regexp"
	"strconv"
	"strings"
)

// KernelRelease returns the running kernel release (`uname -r`). It is a
// package-level var so tests can stub the lookup.
var KernelRelease = func() (string, error) {
	out, err := exec.Command("uname", "-r").Output()
	if err != nil {
		return "", fmt.Errorf("query kernel release: %w", err)
	}
	return strings.TrimSpace(string(out)), nil
}

var versionPattern = regexp.MustCompile(`^\d+(\.\d+)*`)

// rcSuffix matches a pre-release marker directly after the numeric prefix
// ("-rc5", "-rc-5"); distro build suffixes ("-arch1-1", "-1-lts") are not
// pre-releases and stay ignored.
var rcSuffix = regexp.MustCompile(`(?i)^-rc[.-]?\d+`)

// CheckKernelConstraint validates one "<op> <version>" kernel constraint
// expression (the when.kernel grammar), with <op> one of < <= > >= == !=.
// Empty or malformed expressions are rejected loudly; callers treating an
// empty expression as "unset" must check for it first.
func CheckKernelConstraint(expr string) error {
	op, version, err := splitConstraint(expr)
	if err != nil {
		return err
	}
	if !validKernelOp(op) {
		return fmt.Errorf("unsupported operator in kernel constraint %q: %q", expr, op)
	}
	if _, err := parseVersion(version); err != nil {
		return fmt.Errorf("invalid kernel constraint %q: %w", expr, err)
	}
	return nil
}

// CompareKernel compares a running kernel release against one constraint
// ("<", "<=", ">", ">=", "==", "!=" and a dotted-numeric version). Version
// comparison is numeric per dotted segment (7.10 > 7.1), with missing
// segments treated as zero; a distro suffix on the release
// ("6.12.1-arch1-1") is ignored.
func CompareKernel(release, op, version string) (bool, error) {
	if !validKernelOp(op) {
		return false, fmt.Errorf("unsupported kernel operator %q", op)
	}
	want, err := parseVersion(version)
	if err != nil {
		return false, err
	}
	got, err := parseRelease(release)
	if err != nil {
		return false, err
	}
	cmp := compareVersions(got, want)
	switch op {
	case "<":
		return cmp < 0, nil
	case "<=":
		return cmp <= 0, nil
	case ">":
		return cmp > 0, nil
	case ">=":
		return cmp >= 0, nil
	case "==":
		return cmp == 0, nil
	default: // "!="
		return cmp != 0, nil
	}
}

// splitConstraint splits "<op> <version>" into its two fields.
func splitConstraint(expr string) (op, version string, err error) {
	fields := strings.Fields(expr)
	if len(fields) != 2 {
		return "", "", fmt.Errorf("invalid kernel constraint %q: want \"<op> <version>\"", expr)
	}
	return fields[0], fields[1], nil
}

func validKernelOp(op string) bool {
	switch op {
	case "<", "<=", ">", ">=", "==", "!=":
		return true
	}
	return false
}

// parseVersion parses a strict dotted-numeric version ("7.1", "6.12.3").
func parseVersion(s string) ([]int, error) {
	if !versionPattern.MatchString(s) || versionPattern.FindString(s) != s {
		return nil, fmt.Errorf("invalid version %q: want dotted numerics", s)
	}
	return splitSegments(s), nil
}

// parseRelease extracts the leading dotted-numeric prefix of a kernel
// release ("6.12.1-arch1-1" -> 6.12.1). An "-rcN" suffix marks a
// pre-release of that prefix ("7.1-rc5" is before 7.1 but after 7.0):
// it appends a negative sentinel segment, which compareVersions orders
// below the zero padding of the plain release. The rc iteration itself
// is ignored — constraints gate on releases, not rc numbers.
func parseRelease(release string) ([]int, error) {
	s := strings.TrimSpace(release)
	m := versionPattern.FindString(s)
	if m == "" {
		return nil, fmt.Errorf("unparseable kernel release %q", release)
	}
	segs := splitSegments(m)
	if rcSuffix.MatchString(s[len(m):]) {
		segs = append(segs, -1)
	}
	return segs, nil
}

func splitSegments(s string) []int {
	parts := strings.Split(s, ".")
	segs := make([]int, len(parts))
	for i, p := range parts {
		segs[i], _ = strconv.Atoi(p) // pattern guarantees digits
	}
	return segs
}

// compareVersions compares segment-wise, padding the shorter side with
// zeros; it returns -1, 0, or +1.
func compareVersions(a, b []int) int {
	for i := range max(len(a), len(b)) {
		x, y := 0, 0
		if i < len(a) {
			x = a[i]
		}
		if i < len(b) {
			y = b[i]
		}
		if x != y {
			if x < y {
				return -1
			}
			return 1
		}
	}
	return 0
}
