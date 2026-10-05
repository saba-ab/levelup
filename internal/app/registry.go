package app

import (
	"fmt"

	"myapp/internal/config"
	"myapp/internal/platform/modkit"
)

// buildModules is the ONE place modules are listed (PRD §6). Adding a module
// to the system is exactly one line here (plus its config embed). Selection
// is runtime via MODULES_ENABLED (R2): disabling a module must not break the
// others — if it does, the coupling was real and a test just found it.
//
// The template ships with an empty registry. docs/examples.md §3 shows a
// full entry, including the local/remote adapter switch that is the
// extraction seam. The shape:
//
//	"billing": func() modkit.Module {
//		return billing.New(p.DepsFor("billing"), cfg.Billing)
//	},
//
// Module-specific wiring (a module that owns credentials taking p.Authn, a
// consumer choosing between a local and a remote adapter) stays visible
// here on purpose — this file is hand-written, never generated (PRD §7.2).
func buildModules(p *Platform, cfg config.Config) ([]modkit.Module, error) {
	all := map[string]func() modkit.Module{}
	_ = p // every real entry uses p.DepsFor(name)
	return selectModules(all, cfg.ModulesEnabled)
}

// selectModules filters constructors by the enabled list, preserving the
// enabled order. Unknown names fail fast at boot, not at first use.
func selectModules(all map[string]func() modkit.Module, enabled []string) ([]modkit.Module, error) {
	out := make([]modkit.Module, 0, len(enabled))
	seen := map[string]bool{}
	for _, name := range enabled {
		if seen[name] {
			return nil, fmt.Errorf("module %q enabled twice", name)
		}
		seen[name] = true
		ctor, ok := all[name]
		if !ok {
			return nil, fmt.Errorf("unknown module %q in MODULES_ENABLED", name)
		}
		out = append(out, ctor())
	}
	return out, nil
}
