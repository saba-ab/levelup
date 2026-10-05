// Package modkit is the module contract (PRD §6). The composition root knows
// nothing about any module except this interface; a module implementing it is
// discovered, migrated, routed, subscribed, scheduled and health-checked with
// no changes outside internal/app/registry.go (R1).
package modkit

import (
	"context"
	"io/fs"

	"github.com/go-chi/chi/v5"

	"myapp/internal/platform/authz"
	"myapp/internal/platform/bus"
	"myapp/internal/platform/jobs"
)

type Module interface {
	// Name is the module's stable identifier. Used for config keys, schema
	// name (<Name>_svc), metrics labels, and health reporting.
	Name() string

	// Migrations returns this module's embedded SQL, applied into schema
	// <Name>_svc. Nil is valid for modules with no storage.
	Migrations() fs.FS

	// RegisterHTTP mounts this module's routes. The router passed in is
	// already scoped and has platform middleware applied.
	RegisterHTTP(r chi.Router)

	// Subscriptions declares the events this module consumes. The
	// composition root wires these to the bus; the module never touches
	// transport.
	Subscriptions() []bus.Subscription

	// Jobs declares background work: crons and queue consumers.
	Jobs() []jobs.Job

	// Health reports module-level readiness (its own deps only).
	Health(ctx context.Context) error

	// Permissions returns every permission this module defines. The
	// composition root aggregates these into the global catalogue and fails
	// the boot if a role in the DB grants a permission no enabled module
	// declares (PRD §7.6).
	Permissions() []authz.Permission
}

// Optional lifecycle hooks. Type-asserted in the composition root.
type Starter interface {
	Start(ctx context.Context) error
}

type Stopper interface {
	Stop(ctx context.Context) error
}
