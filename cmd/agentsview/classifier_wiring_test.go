// ABOUTME: static AST scan that prevents new commands from
// ABOUTME: opening a store without first wiring the
// ABOUTME: user-prefix classifier singleton.
package main

import (
	"bytes"
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	"github.com/spf13/pflag"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.kenn.io/agentsview/internal/config"
	"go.kenn.io/agentsview/internal/db"
)

// triggerCalls names the qualified function calls that read
// the classifier singleton (directly or indirectly via
// backfill). Every function or function literal in
// cmd/agentsview/ that contains one of these calls must
// contain an EARLIER call to applyClassifierConfig in the
// same enclosing body.
var triggerCalls = map[string]struct{}{
	"db.Open":               {},
	"postgres.Open":         {},
	"postgres.NewStore":     {},
	"postgres.New":          {},
	"postgres.EnsureSchema": {},
}

const wiringHelper = "applyClassifierConfig"

// TestEveryStoreOpenPathIsWired enforces the rule documented
// in the design spec: every code path in cmd/agentsview that
// opens or initializes a store must first call
// applyClassifierConfig so user-defined prefixes reach the
// db package singleton.
func TestEveryStoreOpenPathIsWired(t *testing.T) {
	entries, err := os.ReadDir(".")
	require.NoError(t, err, "listing cmd/agentsview")

	fset := token.NewFileSet()
	var violations []string
	for _, e := range entries {
		name := e.Name()
		if e.IsDir() || !strings.HasSuffix(name, ".go") ||
			strings.HasSuffix(name, "_test.go") {
			continue
		}
		f, err := parser.ParseFile(
			fset, filepath.Join(".", name), nil,
			parser.ParseComments,
		)
		require.NoError(t, err, "parsing %s", name)
		violations = append(
			violations, scanFile(fset, f)...,
		)
	}
	if len(violations) > 0 {
		sort.Strings(violations)
		t.Fatalf(
			"functions or closures missing %s before "+
				"opening a store:\n  %s",
			wiringHelper,
			strings.Join(violations, "\n  "),
		)
	}
}

func TestPhase16AutomatedMatcherConfig(t *testing.T) {
	t.Cleanup(func() { db.SetUserAutomationPrefixes(nil) })

	dir := t.TempDir()
	t.Setenv("AGENTSVIEW_DATA_DIR", dir)
	require.NoError(t, os.WriteFile(
		filepath.Join(dir, "config.toml"), []byte(`
[automated]
prefixes = ["Phase16 prefix rule"]
substrings = ["phase16 embedded marker"]
exact_matches = ["Phase16 exact rule"]
`), 0o600,
	), "write config")

	cfg, err := loadPhase16Config(t)
	require.NoError(t, err, "load config with matcher kinds")
	applyClassifierConfig(cfg)

	assert.True(t, db.IsAutomatedSession("Phase16 prefix rule handles this"),
		"configured prefix should classify")
	assert.True(t, db.IsAutomatedSession("before phase16 embedded marker after"),
		"configured substring should classify")
	assert.True(t, db.IsAutomatedSession("  Phase16 exact rule\n"),
		"configured exact match should trim and classify")
	assert.False(t, db.IsAutomatedSession("Phase16 exact rule with suffix"),
		"configured exact match must not behave like a prefix")
	assert.True(t, db.IsAutomatedSession("Warmup"),
		"built-in exact matcher should remain available")

	var empty bytes.Buffer
	require.NoError(t, os.WriteFile(
		filepath.Join(dir, "config.toml"), empty.Bytes(), 0o600,
	), "write empty config")
	cfg, err = loadPhase16Config(t)
	require.NoError(t, err, "load empty config")
	applyClassifierConfig(cfg)
	assert.False(t, db.IsAutomatedSession("before phase16 embedded marker after"),
		"empty config should clear user substring matchers")

	badDir := t.TempDir()
	t.Setenv("AGENTSVIEW_DATA_DIR", badDir)
	require.NoError(t, os.WriteFile(filepath.Join(badDir, "config.toml"), []byte(`
[automated]
substrings = "phase16-not-a-list"
`), 0o600), "write invalid config")
	_, err = loadPhase16Config(t)
	assert.Error(t, err, "malformed matcher list should use config load error path")
}

func loadPhase16Config(t *testing.T) (config.Config, error) {
	t.Helper()
	fs := pflag.NewFlagSet("phase16", pflag.ContinueOnError)
	config.RegisterServePFlags(fs)
	require.NoError(t, fs.Parse(nil), "parse flags")
	return config.LoadPFlags(fs)
}

// scanFile walks every function declaration and function
// literal in f, returning a violation string for each body
// that contains a trigger call without an earlier
// applyClassifierConfig call.
func scanFile(
	fset *token.FileSet, f *ast.File,
) []string {
	var violations []string
	ast.Inspect(f, func(n ast.Node) bool {
		switch fn := n.(type) {
		case *ast.FuncDecl:
			if fn.Body == nil {
				return true
			}
			if v := checkBody(
				fset, fn.Body, funcLabel(fset, fn),
			); v != "" {
				violations = append(violations, v)
			}
		case *ast.FuncLit:
			if v := checkBody(
				fset, fn.Body, litLabel(fset, fn),
			); v != "" {
				violations = append(violations, v)
			}
		}
		return true
	})
	return violations
}

// checkBody walks body's statements in source order. If a
// trigger call appears before the helper call (or the helper
// call never appears), it returns a violation string. Helper
// and trigger searches descend into nested expressions but
// stop at nested function literals — those have their own
// scope and are checked separately by ast.Inspect.
func checkBody(
	fset *token.FileSet,
	body *ast.BlockStmt,
	label string,
) string {
	var (
		seenHelper  bool
		earlyTrig   string
		earlyTrigAt token.Pos
	)
	ast.Inspect(body, func(n ast.Node) bool {
		// Don't descend into nested func literals — they
		// carry their own scope and are visited by the
		// outer ast.Inspect in scanFile.
		if _, ok := n.(*ast.FuncLit); ok {
			return false
		}
		call, ok := n.(*ast.CallExpr)
		if !ok {
			return true
		}
		switch fn := call.Fun.(type) {
		case *ast.Ident:
			if fn.Name == wiringHelper {
				seenHelper = true
			}
		case *ast.SelectorExpr:
			pkg, ok := fn.X.(*ast.Ident)
			if !ok {
				return true
			}
			qname := pkg.Name + "." + fn.Sel.Name
			if _, isTrigger := triggerCalls[qname]; isTrigger {
				if !seenHelper && earlyTrig == "" {
					earlyTrig = qname
					earlyTrigAt = call.Pos()
				}
			}
		}
		return true
	})
	if earlyTrig == "" {
		return ""
	}
	pos := fset.Position(earlyTrigAt)
	return label + ": calls " + earlyTrig +
		" at " + pos.Filename + ":" +
		itoa(pos.Line) + " without earlier " +
		wiringHelper
}

func funcLabel(fset *token.FileSet, fn *ast.FuncDecl) string {
	pos := fset.Position(fn.Pos())
	return fn.Name.Name + " (" + pos.Filename + ":" +
		itoa(pos.Line) + ")"
}

func litLabel(fset *token.FileSet, fn *ast.FuncLit) string {
	pos := fset.Position(fn.Pos())
	return "anonymous func at " + pos.Filename + ":" +
		itoa(pos.Line)
}

// itoa avoids importing strconv just for line numbers.
func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	var buf [20]byte
	i := len(buf)
	for n > 0 {
		i--
		buf[i] = byte('0' + n%10)
		n /= 10
	}
	return string(buf[i:])
}
