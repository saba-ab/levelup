package arch

import (
	"fmt"
	"go/ast"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"testing"
)

// TestModuleBoundaries is the PRD §9 assertion: a cross-module import is
// legal only when it targets the other module's contracts/ — with adapters/
// additionally allowed to bridge — and it produces docs/modules.md (R21).
func TestModuleBoundaries(t *testing.T) {
	pkgs := loadPackages(t)
	edges := map[string]map[string]bool{} // module → set of modules it reads

	for _, p := range pkgs {
		owner, ok := moduleOf(p.PkgPath)
		if !ok {
			continue
		}
		for imp := range p.Imports {
			target, ok := moduleOf(imp)
			if !ok || target == owner {
				continue
			}
			if edges[owner] == nil {
				edges[owner] = map[string]bool{}
			}
			edges[owner][target] = true

			if !isContractsPkg(imp) && !isAdaptersPkg(p.PkgPath) {
				t.Errorf("%s imports %s — cross-module import outside contracts/adapters (PRD §9)",
					p.PkgPath, imp)
			}
		}
	}

	writeModuleGraph(t, edges)
}

// TestPlatformHasNoModuleImports: infrastructure must never point at
// business code (belt to depguard's platform-purity).
func TestPlatformHasNoModuleImports(t *testing.T) {
	for _, p := range loadPackages(t) {
		if !strings.HasPrefix(p.PkgPath, "myapp/internal/platform") {
			continue
		}
		for imp := range p.Imports {
			if strings.HasPrefix(imp, modulePrefix) {
				t.Errorf("%s imports %s — platform must never depend on modules", p.PkgPath, imp)
			}
			if imp == "myapp/internal/config" || imp == "myapp/internal/app" {
				t.Errorf("%s imports %s — platform takes primitives, not application shape", p.PkgPath, imp)
			}
		}
	}
}

// TestSharedHasZeroInternalImports (PRD §14): the moment shared/ imports
// inward it stops being shared and starts being a dumping ground.
func TestSharedHasZeroInternalImports(t *testing.T) {
	for _, p := range loadPackages(t) {
		if !strings.HasPrefix(p.PkgPath, "myapp/internal/shared") {
			continue
		}
		for imp := range p.Imports {
			if strings.HasPrefix(imp, "myapp/") && !strings.HasPrefix(imp, "myapp/internal/shared") {
				t.Errorf("%s imports %s — shared/ packages import nothing of myapp but shared/", p.PkgPath, imp)
			}
		}
	}
}

// TestNoAutoMigrate (PRD §7.1 Mechanism 3): goose owns DDL. Two systems
// writing schema against one database produce drift that only shows up in
// production. (`TurnOffAutoMigrate` deliberately does not match.)
func TestNoAutoMigrate(t *testing.T) {
	call := regexp.MustCompile(`\.AutoMigrate\(`)
	walkFiles(t, "internal", ".go", func(path, content string) {
		if call.MatchString(content) {
			t.Errorf("%s calls AutoMigrate — goose owns the schema (PRD §7.1)", path)
		}
	})
}

// TestDomainCarriesNoFrameworkTags (R17, §7.1): a domain struct with gorm or
// validate tags can be constructed invalid by anything that skips the
// constructor.
func TestDomainCarriesNoFrameworkTags(t *testing.T) {
	tag := regexp.MustCompile("`[^`]*(gorm|validate):")
	walkFiles(t, "internal/modules", ".go", func(path, content string) {
		if !strings.Contains(path, "/internal/domain/") {
			return
		}
		if tag.MatchString(content) {
			t.Errorf("%s: domain structs must not carry gorm/validate tags (R17, §7.1)", path)
		}
	})
}

// TestNoCrossModuleForeignKeys (P5): FKs cannot cross databases; a
// cross-schema REFERENCES today is a drop-under-pressure tomorrow.
func TestNoCrossModuleForeignKeys(t *testing.T) {
	ref := regexp.MustCompile(`(?i)REFERENCES\s+([a-z0-9_]+_svc)\.`)
	walkFiles(t, "internal/modules", ".sql", func(path, content string) {
		parts := strings.Split(filepath.ToSlash(path), "/")
		// internal/modules/<name>/migrations/file.sql
		if len(parts) < 4 {
			return
		}
		owner := parts[2] + "_svc"
		for _, m := range ref.FindAllStringSubmatch(content, -1) {
			if m[1] != owner {
				t.Errorf("%s: foreign key REFERENCES %s.* crosses a module boundary (P5)", path, m[1])
			}
		}
	})
}

// TestNoCacheEvictionInsideTransactions (R44): deleting inside the tx opens
// the stale-repopulation window §7.8.1 describes. Statically: no .Del or
// .Evict call inside a function literal passed to InTx or s.tx.
func TestNoCacheEvictionInsideTransactions(t *testing.T) {
	pkgs := loadPackages(t)
	for _, p := range pkgs {
		if !strings.HasPrefix(p.PkgPath, "myapp/internal/") {
			continue
		}
		for _, f := range p.Syntax {
			ast.Inspect(f, func(n ast.Node) bool {
				call, ok := n.(*ast.CallExpr)
				if !ok || !isTxRunnerCall(call) {
					return true
				}
				for _, arg := range call.Args {
					lit, ok := arg.(*ast.FuncLit)
					if !ok {
						continue
					}
					ast.Inspect(lit, func(inner ast.Node) bool {
						c, ok := inner.(*ast.CallExpr)
						if !ok {
							return true
						}
						if sel, ok := c.Fun.(*ast.SelectorExpr); ok {
							if sel.Sel.Name == "Del" || sel.Sel.Name == "Evict" {
								pos := p.Fset.Position(c.Pos())
								t.Errorf("%s: cache eviction inside a transaction — evict AFTER commit (R44)", pos)
							}
						}
						return true
					})
				}
				return true
			})
		}
	}
}

func isTxRunnerCall(call *ast.CallExpr) bool {
	switch fun := call.Fun.(type) {
	case *ast.SelectorExpr:
		if fun.Sel.Name == "InTx" {
			return true
		}
		if fun.Sel.Name == "tx" { // the service-injected runner field
			return true
		}
	case *ast.Ident:
		if fun.Name == "InTx" {
			return true
		}
	}
	return false
}

// TestTransportNeverPublishes (PRD §14): an HTTP handler that talks to the
// bus or outbox has skipped the service layer and its transaction.
func TestTransportNeverPublishes(t *testing.T) {
	for _, p := range loadPackages(t) {
		if _, ok := moduleOf(p.PkgPath); !ok || !strings.Contains(p.PkgPath, "/transport") {
			continue
		}
		for imp := range p.Imports {
			if imp == "myapp/internal/platform/bus" || imp == "myapp/internal/platform/outbox" {
				t.Errorf("%s imports %s — transport never publishes; services do, inside InTx", p.PkgPath, imp)
			}
		}
	}
}

// TestEnqueueCallersAllowlisted (R46): direct Enqueue is fire-and-forget by
// definition. State-change-triggered work goes through the outbox; the only
// legitimate direct callers are the scheduler and explicitly listed spots.
func TestEnqueueCallersAllowlisted(t *testing.T) {
	allowed := map[string]bool{
		"myapp/internal/platform/scheduler": true, // enqueues on cron ticks
		"myapp/internal/platform/jobs":      true, // the implementation itself
	}
	for _, p := range loadPackages(t) {
		if !strings.HasPrefix(p.PkgPath, "myapp/internal/") || allowed[p.PkgPath] {
			continue
		}
		for _, f := range p.Syntax {
			ast.Inspect(f, func(n ast.Node) bool {
				call, ok := n.(*ast.CallExpr)
				if !ok {
					return true
				}
				if sel, ok := call.Fun.(*ast.SelectorExpr); ok && sel.Sel.Name == "Enqueue" {
					pos := p.Fset.Position(call.Pos())
					t.Errorf("%s: direct jobs.Enqueue outside the allowlist — route state-change jobs through the outbox as topic \"job.<name>\" (R46)", pos)
				}
				return true
			})
		}
	}
}

// TestNoTableNameOverridesInModules: GORM's TablePrefix — the §7.1
// containment mechanism — silently ignores models with TableName()
// overrides. Discovered the hard way; now it cannot regress.
func TestNoTableNameOverridesInModules(t *testing.T) {
	for _, p := range loadPackages(t) {
		if _, ok := moduleOf(p.PkgPath); !ok {
			continue
		}
		for _, f := range p.Syntax {
			for _, decl := range f.Decls {
				fn, ok := decl.(*ast.FuncDecl)
				if !ok || fn.Recv == nil || fn.Name.Name != "TableName" {
					continue
				}
				pos := p.Fset.Position(fn.Pos())
				t.Errorf("%s: TableName() override defeats the module TablePrefix (§7.1) — derive the table from the struct name", pos)
			}
		}
	}
}

func writeModuleGraph(t *testing.T, edges map[string]map[string]bool) {
	t.Helper()
	var b strings.Builder
	b.WriteString("# Module dependency graph\n\n")
	b.WriteString("Generated by `task arch` (test/arch, R21). Edges are compile-time\n")
	b.WriteString("imports of another module's contracts — the only legal kind (PRD §9).\n\n")
	b.WriteString("```mermaid\ngraph LR\n")

	var owners []string
	for owner := range edges {
		owners = append(owners, owner)
	}
	sort.Strings(owners)
	if len(owners) == 0 {
		b.WriteString("    %% no cross-module dependencies\n")
	}
	for _, owner := range owners {
		var targets []string
		for tgt := range edges[owner] {
			targets = append(targets, tgt)
		}
		sort.Strings(targets)
		for _, tgt := range targets {
			fmt.Fprintf(&b, "    %s -->|contracts| %s\n", owner, tgt)
		}
	}
	b.WriteString("```\n\nAsync reactions (event subscriptions) are not imports and do not\nappear above; see each module's Subscriptions() for those edges.\n")

	out := filepath.Join(repoRoot(t), "docs", "modules.md")
	if err := os.WriteFile(out, []byte(b.String()), 0o644); err != nil {
		t.Fatalf("writing modules.md: %v", err)
	}
}
