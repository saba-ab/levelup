// Command modgen scaffolds a new module into internal/modules/<name>
// (PRD §6.3, R12). It lives in the tools module ON PURPOSE: go-stub must
// never enter the service dependency graph, and a root-module cmd/ would
// pull it into the root go.mod.
//
// Usage (via the task runner): task new-module NAME=billing
package main

import (
	"embed"
	"flag"
	"fmt"
	"go/format"
	"log"
	"os"
	"path/filepath"
	"strings"

	stub "github.com/binafy/go-stub"
)

//go:embed stubs
var stubs embed.FS

func main() {
	name := flag.String("name", "", "module name, e.g. billing")
	flag.Parse()
	if *name == "" {
		log.Fatal("-name required")
	}

	root := repoRoot()
	dst := filepath.Join(root, "internal", "modules", stub.ToSnake(*name))
	if _, err := os.Stat(dst); err == nil {
		// Default no-overwrite is correct; this guard just makes the
		// failure message readable (PRD §6.3).
		log.Fatalf("module %s already exists at %s", *name, dst)
	}

	err := stub.GenerateDirFS(stubs, "stubs/module", dst,
		stub.WithReplaces(map[string]any{
			"PACKAGE": stub.ToSnake(*name),          // billing
			"NAME":    stub.ToPascal(*name),         // Billing
			"SCHEMA":  stub.ToSnake(*name) + "_svc", // billing_svc
			"TOPIC":   stub.ToSnake(*name),          // billing.created.v1
			"MODULE":  modulePath(root),             // myapp
		}),
		stub.WithTrimSuffix(".stub"),
		stub.WithDelimiters("<<", ">>"), // {{ }} must survive for text/template users
		stub.WithStrict(),               // a stub typo fails naming the key, not with a gofmt riddle
		// NOT WithFormat(): it gofmts EVERY output including .sql (its docs
		// say Go-only trees). gofmtTree below keeps the gofmt-clean
		// guarantee for the .go files (R12, ADR-0007 amendment).
	)
	if err != nil {
		_ = os.RemoveAll(dst) // never leave a half-written module behind
		log.Fatal(err)
	}
	if err := gofmtTree(dst); err != nil {
		_ = os.RemoveAll(dst)
		log.Fatal(err)
	}

	snake := stub.ToSnake(*name)
	fmt.Printf(`created %s

wire it up (one line):
  internal/app/registry.go:
    %q: func() modkit.Module { return %s.New(p.DepsFor(%q)) },

if the module grows settings, give it a Config struct and embed it in
internal/config/config.go with envPrefix %q (PRD §7.3).

then: task migrate && task arch
`, dst, snake, snake, snake, strings.ToUpper(snake)+"_")
}

// gofmtTree runs go/format over every generated .go file so `task lint`
// passes immediately after scaffolding (R12).
func gofmtTree(dst string) error {
	return filepath.WalkDir(dst, func(path string, d os.DirEntry, err error) error {
		if err != nil || d.IsDir() || !strings.HasSuffix(path, ".go") {
			return err
		}
		src, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		out, err := format.Source(src)
		if err != nil {
			return fmt.Errorf("gofmt %s: %w", path, err)
		}
		return os.WriteFile(path, out, 0o644)
	})
}

// repoRoot walks up from CWD to the directory holding the ROOT go.mod (the
// one whose module path is not the tools module).
func repoRoot() string {
	dir, err := os.Getwd()
	if err != nil {
		log.Fatal(err)
	}
	for {
		if b, err := os.ReadFile(filepath.Join(dir, "go.mod")); err == nil {
			if !strings.Contains(string(b), "module levelup/tools") {
				return dir
			}
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			log.Fatal("root go.mod not found above working directory")
		}
		dir = parent
	}
}

func modulePath(root string) string {
	b, err := os.ReadFile(filepath.Join(root, "go.mod"))
	if err != nil {
		log.Fatal(err)
	}
	for line := range strings.Lines(string(b)) {
		if rest, ok := strings.CutPrefix(strings.TrimSpace(line), "module "); ok {
			return strings.TrimSpace(rest)
		}
	}
	log.Fatal("module path not found in go.mod")
	return ""
}
