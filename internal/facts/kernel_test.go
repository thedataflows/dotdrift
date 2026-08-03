package facts_test

import (
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/thedataflows/dotdrift/internal/facts"
)

func TestCompareKernel(t *testing.T) {
	tests := []struct {
		name    string
		release string
		op      string
		version string
		want    bool
	}{
		{"lt true", "6.12", "<", "7.1", true},
		{"lt false at boundary", "7.1", "<", "7.1", false},
		{"lt false above", "7.10", "<", "7.1", false},
		{"lte boundary", "7.1", "<=", "7.1", true},
		{"lte zero-padded release", "7.1.0", "<=", "7.1", true},
		{"gt numeric segment compare", "7.10", ">", "7.1", true},
		{"gt false at boundary", "7.1", ">", "7.1", false},
		{"gte boundary", "7.1", ">=", "7.1", true},
		{"gte below", "6.12", ">=", "7.1", false},
		{"gte numeric segment compare", "7.10", ">=", "7.1", true},
		{"eq", "7.1", "==", "7.1", true},
		{"eq false", "7.2", "==", "7.1", false},
		{"ne", "6.12", "!=", "7.1", true},
		{"ne false", "7.1", "!=", "7.1", false},
		{"distro-suffixed release", "6.12.1-arch1-1", "<", "7.1", true},
		{"rc is pre-release (lt)", "7.1-rc5", "<", "7.1", true},
		{"rc dashed iteration (lt)", "7.1-rc-5", "<", "7.1", true},
		{"rc is pre-release (gte false)", "7.1-rc5", ">=", "7.1", false},
		{"rc equals neither final (ne)", "7.1-rc5", "!=", "7.1", true},
		{"rc after previous final", "7.1-rc5", ">", "7.0", true},
		{"distro rc suffix", "7.2.0-rc5-1-cachyos-rc", "<", "7.2", true},
		{"non-rc suffix stays equal", "7.1.2-arch1-1", "==", "7.1.2", true},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, err := facts.CompareKernel(tc.release, tc.op, tc.version)
			require.NoError(t, err)
			require.Equal(t, tc.want, got)
		})
	}
}

func TestCompareKernel_errors(t *testing.T) {
	t.Run("unsupported operator", func(t *testing.T) {
		_, err := facts.CompareKernel("7.1", "~", "7.1")
		require.Error(t, err)
		require.Contains(t, err.Error(), "~")
	})
	t.Run("invalid version", func(t *testing.T) {
		_, err := facts.CompareKernel("7.1", ">=", "seven")
		require.Error(t, err)
		require.Contains(t, err.Error(), "seven")
	})
	t.Run("unparseable release", func(t *testing.T) {
		_, err := facts.CompareKernel("not-a-version", ">=", "7.1")
		require.Error(t, err)
		require.Contains(t, err.Error(), "not-a-version")
	})
}

func TestCheckKernelConstraint(t *testing.T) {
	t.Run("valid", func(t *testing.T) {
		for _, expr := range []string{">= 7.1", "< 7.1", "== 6.12.3", "!= 7", "> 6.12", "<= 7.1.2"} {
			require.NoError(t, facts.CheckKernelConstraint(expr), "expression %q must be accepted", expr)
		}
	})
	t.Run("invalid", func(t *testing.T) {
		for _, expr := range []string{"", "kernel >= 7.1", ">=", "~ 7.1", ">= seven", ">= 7.1 extra", "7.1"} {
			require.Error(t, facts.CheckKernelConstraint(expr), "expression %q must be rejected loudly", expr)
		}
	})
}
