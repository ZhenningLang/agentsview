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
	got := phase21SQLiteHelperInventory(t)

	want := map[string]string{
		"classifier.go:clearSQLiteClassifierHash:acquireWriteOwnerLock": "write",
		"daemon.go:runDaemonStartCommand:acquireWriteOwnerLock":         "write",
		"duckdb.go:runDuckDBPush:openWriteDB":                           "write",
		"duckdb.go:runDuckDBStatus:openReadOnlyDB":                      "read",
		"enrich.go:runEnrich:openDB":                                    "write",
		"health.go:runHealth:openReadOnlyDB":                            "read",
		"import.go:runImport:openWriteDB":                               "write",
		"main.go:mustOpenDB:openDB":                                     "write",
		"main.go:openDB:openWriteDB":                                    "write",
		"main.go:runServe:mustOpenDB":                                   "write",
		"pg.go:runPGPush:openWriteDB":                                   "write",
		"pg.go:runPGStatus:openReadOnlyDB":                              "read",
		"pg_service.go:runServiceStatus:openReadOnlyDB":                 "read",
		"pg_watch.go:runPGPushWatch:mustOpenDB":                         "write",
		"projects.go:runProjects:openReadOnlyDB":                        "read",
		"prune.go:runPrune:openWriteDB":                                 "write",
		"session_export.go:newSessionExportCommand:openReadOnlyDB":      "read",
		"session_sync.go:syncService:openWriteDB":                       "write",
		"stats.go:openStatsService:openReadOnlyDB":                      "read",
		"sync.go:doSync:openWriteDB":                                    "write",
		"token_use.go:sessionUsageData:openReadOnlyDB":                  "read",
		"token_use.go:sessionUsageData:openWriteDB":                     "write",
		"transport.go:newService:openReadOnlyDB":                        "read",
		"transport.go:newService:openWriteDB":                           "write",
		"usage.go:openUsageDB:openWriteDB":                              "write",
		"write_lock.go:openWriteDB:acquireWriteOwnerLock":               "write",
	}

	assert.Equal(t, want, got)
}

func TestPhase21SQLiteRawOpenInventoryAllowsOnlyCentralHelpers(t *testing.T) {
	got := phase21SQLiteRawOpenInventory(t)

	want := map[string]string{
		"classifier.go:clearSQLiteClassifierHash:sql.Open(sqlite3)": "write",
		"write_lock.go:openReadOnlyDB:db.OpenReadOnly":              "read",
		"write_lock.go:openWriteDB:db.Open":                         "write",
	}

	assert.Equal(t, want, got)
}

func phase21SQLiteHelperInventory(t *testing.T) map[string]string {
	t.Helper()
	return phase21Inventory(t, phase21SQLiteHelperKind, phase21ExpectedHelperClass)
}

func phase21SQLiteRawOpenInventory(t *testing.T) map[string]string {
	t.Helper()
	return phase21Inventory(t, phase21SQLiteRawOpenKind, phase21ExpectedRawOpenClass)
}

func phase21Inventory(
	t *testing.T,
	kindFor func(*ast.CallExpr) string,
	classFor func(string, string) string,
) map[string]string {
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
				kind := kindFor(call)
				if kind == "" {
					return true
				}
				key := name + ":" + funcName + ":" + kind
				observed[key] = classFor(kind, key)
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

func phase21SQLiteRawOpenKind(call *ast.CallExpr) string {
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

func phase21SQLiteHelperKind(call *ast.CallExpr) string {
	ident, ok := call.Fun.(*ast.Ident)
	if !ok {
		return ""
	}
	switch ident.Name {
	case "openWriteDB", "openReadOnlyDB", "openDB", "mustOpenDB", "acquireWriteOwnerLock":
		return ident.Name
	default:
		return ""
	}
}

func phase21ExpectedRawOpenClass(kind, key string) string {
	if strings.Contains(kind, "OpenReadOnly") {
		return "read"
	}
	return "write"
}

func phase21ExpectedHelperClass(kind, key string) string {
	if kind == "openReadOnlyDB" {
		return "read"
	}
	return "write"
}
