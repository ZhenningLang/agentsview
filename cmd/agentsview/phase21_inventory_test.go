package main

import (
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestPhase21SQLiteOpenInventoryClassifiesProductionCallSites(t *testing.T) {
	got := phase21SQLiteOpenInventory(t)

	want := map[string]string{
		"classifier.go:clearSQLiteClassifierHash:sql.Open(sqlite3)": "write",
		"duckdb.go:runDuckDBPush:db.Open":                           "write",
		"duckdb.go:runDuckDBStatus:db.OpenReadOnly":                 "read",
		"health.go:runHealth:db.OpenReadOnly":                       "read",
		"import.go:runImport:db.Open":                               "write",
		"main.go:openDB:db.Open":                                    "write",
		"pg.go:runPGPush:db.Open":                                   "write",
		"pg.go:runPGStatus:db.OpenReadOnly":                         "read",
		"projects.go:runProjects:db.OpenReadOnly":                   "read",
		"prune.go:runPrune:db.Open":                                 "write",
		"session_export.go:newSessionExportCommand:db.OpenReadOnly": "read",
		"session_sync.go:syncService:db.Open":                       "write",
		"sync.go:doSync:db.Open":                                    "write",
		"token_use.go:sessionUsageData:db.Open":                     "write",
		"transport.go:newService:db.Open":                           "write",
		"transport.go:newService:db.OpenReadOnly":                   "read",
	}

	assert.Equal(t, want, got)
}

func phase21SQLiteOpenInventory(t *testing.T) map[string]string {
	t.Helper()
	_, file, _, ok := runtime.Caller(0)
	require.True(t, ok, "resolve current file")
	dir := filepath.Dir(file)
	entries, err := os.ReadDir(dir)
	require.NoError(t, err)

	fset := token.NewFileSet()
	observed := map[string]string{}
	for _, entry := range entries {
		name := entry.Name()
		if entry.IsDir() || !strings.HasSuffix(name, ".go") || strings.HasSuffix(name, "_test.go") {
			continue
		}
		path := filepath.Join(dir, name)
		fileAST, err := parser.ParseFile(fset, path, nil, 0)
		require.NoError(t, err, "parse %s", name)
		for _, decl := range fileAST.Decls {
			fn, ok := decl.(*ast.FuncDecl)
			if !ok || fn.Body == nil {
				continue
			}
			funcName := fn.Name.Name
			ast.Inspect(fn.Body, func(n ast.Node) bool {
				call, ok := n.(*ast.CallExpr)
				if !ok {
					return true
				}
				kind := phase21SQLiteOpenKind(call)
				if kind == "" {
					return true
				}
				key := name + ":" + funcName + ":" + kind
				observed[key] = phase21ExpectedOpenClass(kind, key)
				return true
			})
		}
	}
	keys := make([]string, 0, len(observed))
	for key := range observed {
		keys = append(keys, key)
	}
	slices.Sort(keys)
	ordered := map[string]string{}
	for _, key := range keys {
		ordered[key] = observed[key]
	}
	return ordered
}

func phase21SQLiteOpenKind(call *ast.CallExpr) string {
	sel, ok := call.Fun.(*ast.SelectorExpr)
	if !ok {
		return ""
	}
	x, ok := sel.X.(*ast.Ident)
	if !ok {
		return ""
	}
	if x.Name == "db" && (sel.Sel.Name == "Open" || sel.Sel.Name == "OpenReadOnly") {
		return "db." + sel.Sel.Name
	}
	if x.Name == "sql" && sel.Sel.Name == "Open" && len(call.Args) > 0 {
		if lit, ok := call.Args[0].(*ast.BasicLit); ok && lit.Value == "\"sqlite3\"" {
			return "sql.Open(sqlite3)"
		}
	}
	return ""
}

func phase21ExpectedOpenClass(kind, key string) string {
	if strings.Contains(kind, "OpenReadOnly") {
		return "read"
	}
	if strings.Contains(key, "runDuckDBStatus") || strings.Contains(key, "runPGStatus") ||
		strings.Contains(key, "runHealth") || strings.Contains(key, "runProjects") ||
		strings.Contains(key, "session_export.go") {
		return "unclassified-read"
	}
	return "write"
}
