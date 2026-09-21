// Package service is dotdrift's service layer (issue 0061): one package,
// per-area structs composed into a root, typed errors at the boundary.
// The apply session (issue 0064) is the apply area; the reads areas
// (modules, plan, status, diff, detect) migrated with T-tui-reads.
package service

// Service is the root facade over dotdrift's operations. Areas are
// addressable directly; the root exists so consumers hold one value.
type Service struct {
	Apply *ApplyArea
	Reads *ReadsArea
}
