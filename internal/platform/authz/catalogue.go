package authz

import (
	"fmt"
	"strings"
)

// Cataloguer is what the composition root needs from an enforcer to run the
// boot-time drift check. The casbin implementation satisfies it; AllowAll
// deliberately does not (nothing to check).
type Cataloguer interface {
	Reload() error
	GrantedKeys() ([]string, error)
}

// VerifyCatalogue fails the boot if any stored policy grants a permission no
// enabled module declares (R19, PRD §7.6 constraint 2). Silent authorization
// drift after a rename or deletion otherwise surfaces as the first 403
// nobody reports.
func VerifyCatalogue(enf Cataloguer, declared []Permission) error {
	if err := enf.Reload(); err != nil {
		return fmt.Errorf("authz: reload policy for catalogue check: %w", err)
	}
	known := make(map[string]bool, len(declared))
	for _, p := range declared {
		known[p.Key()] = true
	}
	granted, err := enf.GrantedKeys()
	if err != nil {
		return err
	}
	var orphans []string
	for _, key := range granted {
		if !known[key] {
			orphans = append(orphans, key)
		}
	}
	if len(orphans) > 0 {
		return fmt.Errorf(
			"authz: casbin_rule grants permissions no enabled module declares: %s — "+
				"either enable the owning module or delete the stale policy rows (R19)",
			strings.Join(orphans, ", "))
	}
	return nil
}
