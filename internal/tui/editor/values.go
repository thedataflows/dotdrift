package editor

import (
	"github.com/BurntSushi/toml"
	"github.com/thedataflows/dotdrift/internal/profile"
)

// Value plumbing shared by the section adapters: passthrough directive
// values round-trip through raw TOML text, so editing a value means
// editing its TOML spelling and parsing it back — the types survive.

// rawValue renders a typed TOML value back to its text spelling.
func rawValue(v any) string {
	return profile.EncodeTomlValue(v)
}

// tomlDecodeAny parses one raw TOML value back into its typed value.
func tomlDecodeAny(raw string) (any, error) {
	var v struct {
		V any `toml:"v"`
	}
	if _, err := toml.Decode("v = "+raw+"\n", &v); err != nil {
		return nil, err
	}
	return v.V, nil
}
