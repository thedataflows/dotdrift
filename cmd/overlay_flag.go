package cmd

import (
	"github.com/alecthomas/kong"
)

// overlayFlag is a bool-like flag with an optional value, for onboard's
// --host/--user: bare `--host` selects the CURRENT host, `--host=<name>`
// an explicit one (`--host=` also means current). Implementing kong's
// BoolMapperValue (Decode + IsBool) makes the flag accept both spellings;
// bool-like flags bind values ONLY through the = form, so a bare --host
// before a positional path can never swallow it.
type overlayFlag struct {
	Set   bool
	Value string
}

// IsBool marks the flag bool-like: bare usage is valid.
func (o *overlayFlag) IsBool() bool { return true }

// Decode records the flag as set, consuming a =value when present.
func (o *overlayFlag) Decode(ctx *kong.DecodeContext) error {
	o.Set = true
	if ctx.Scan.Peek().Type == kong.FlagValueToken {
		var v string
		if err := ctx.Scan.PopValueInto("value", &v); err != nil {
			return err
		}
		o.Value = v
	}
	return nil
}

// overlayOwner resolves an overlay flag's owner: an explicit value wins,
// a bare flag falls back to the detected fact.
func overlayOwner(flag overlayFlag, detected string) string {
	if flag.Set && flag.Value != "" {
		return flag.Value
	}
	return detected
}
