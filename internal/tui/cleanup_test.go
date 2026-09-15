package tui

// T-tui-cleanup: the proof that nothing depends on the old paths.

import (
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

// TestNoDeadStyles scans the theme registry: every style field must be
// referenced by a non-theme source file in this package (the ADR-0003
// registry carries no entries only deleted views referenced).
func TestNoDeadStyles(t *testing.T) {
	fset := token.NewFileSet()
	f, err := parser.ParseFile(fset, "theme.go", nil, 0)
	require.NoError(t, err)
	var fields []string
	for _, decl := range f.Decls {
		gd, ok := decl.(*ast.GenDecl)
		if !ok {
			continue
		}
		for _, spec := range gd.Specs {
			ts, ok := spec.(*ast.TypeSpec)
			if !ok || ts.Name.Name != "theme" {
				continue
			}
			st := ts.Type.(*ast.StructType)
			for _, field := range st.Fields.List {
				for _, name := range field.Names {
					fields = append(fields, name.Name)
				}
			}
		}
	}
	require.NotEmpty(t, fields, "the theme struct was found")

	// Every field referenced outside theme.go (tests count: the registry
	// test renders them).
	files, err := filepath.Glob("*.go")
	require.NoError(t, err)
	var corpus strings.Builder
	for _, file := range files {
		if file == "theme.go" {
			continue
		}
		src, err := os.ReadFile(file)
		require.NoError(t, err)
		corpus.Write(src)
	}
	body := corpus.String()
	for _, field := range fields {
		require.True(t, strings.Contains(body, "."+field) || strings.Contains(body, "t."+field+"."),
			"theme.%s is unreferenced — dead style", field)
	}
}
