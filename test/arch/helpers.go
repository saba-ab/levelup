// Package arch is Layer 3 of boundary enforcement (PRD §9): the precise
// import-graph and AST assertions that prefix-based lint cannot express.
// These tests need no Docker and run in `task arch` and `task check`.
package arch

import (
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"golang.org/x/tools/go/packages"
)

const modulePrefix = "myapp/internal/modules/"

var (
	loadOnce sync.Once
	loaded   []*packages.Package
	loadErr  error
)

// loadPackages loads the full syntax+imports graph once per test binary.
func loadPackages(t *testing.T) []*packages.Package {
	t.Helper()
	loadOnce.Do(func() {
		cfg := &packages.Config{
			Mode: packages.NeedName | packages.NeedImports | packages.NeedFiles |
				packages.NeedSyntax | packages.NeedTypes | packages.NeedDeps,
			Dir:   repoRoot(t),
			Tests: false,
		}
		loaded, loadErr = packages.Load(cfg, "myapp/...")
	})
	if loadErr != nil {
		t.Fatalf("loading packages: %v", loadErr)
	}
	for _, p := range loaded {
		if len(p.Errors) > 0 {
			t.Fatalf("package %s has load errors: %v", p.PkgPath, p.Errors)
		}
	}
	return loaded
}

func repoRoot(t *testing.T) string {
	t.Helper()
	dir, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	for {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return dir
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			t.Fatal("go.mod not found above working directory")
		}
		dir = parent
	}
}

// moduleOf returns the owning module name, or "" if the path is not inside
// internal/modules.
func moduleOf(pkgPath string) (string, bool) {
	rest, ok := strings.CutPrefix(pkgPath, modulePrefix)
	if !ok {
		return "", false
	}
	name, _, _ := strings.Cut(rest, "/")
	return name, name != ""
}

func isContractsPkg(pkgPath string) bool {
	return strings.HasSuffix(pkgPath, "/contracts")
}

func isAdaptersPkg(pkgPath string) bool {
	return strings.Contains(pkgPath, "/adapters")
}

// walkFiles visits raw file contents. AST-level assertions use the loaded
// package syntax trees instead (see arch_test.go); this is for SQL and
// string scans, where parsing would buy nothing.
func walkFiles(t *testing.T, root, suffix string, visit func(path string, content string)) {
	t.Helper()
	base := repoRoot(t)
	err := filepath.WalkDir(filepath.Join(base, root), func(path string, d os.DirEntry, err error) error {
		if err != nil || d.IsDir() || !strings.HasSuffix(path, suffix) {
			return err
		}
		b, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		visit(strings.TrimPrefix(path, base+string(os.PathSeparator)), string(b))
		return nil
	})
	if err != nil {
		t.Fatalf("walking %s: %v", root, err)
	}
}
