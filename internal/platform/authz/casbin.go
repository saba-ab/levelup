package authz

// THE ONLY FILE THAT IMPORTS CASBIN (PRD §7.6 constraint 1). Everything else
// sees the Enforcer interface, which keeps OpenFGA/SpiceDB adoptable (R33).

import (
	"context"
	_ "embed"

	"github.com/casbin/casbin/v3"
	casbinmodel "github.com/casbin/casbin/v3/model"
	gormadapter "github.com/casbin/gorm-adapter/v3"
	rediswatcher "github.com/casbin/redis-watcher/v2"
	goredis "github.com/redis/go-redis/v9"
	"go.uber.org/zap"
	"gorm.io/gorm"

	"myapp/internal/shared/errs"
)

//go:embed model.conf
var modelConf string

// Casbin enforces the RBAC model over authz_svc.casbin_rule. Policy rows are
// goose-owned (no adapter auto-migration): two systems writing DDL against
// one database produce drift that only shows up in production (PRD §7.1).
type Casbin struct {
	e   *casbin.SyncedEnforcer
	log *zap.Logger
}

// NewCasbin builds the enforcer. db must be scoped to authz_svc. coreAddr is
// redis-core, carrying the policy-change channel: without the watcher,
// revoking someone's access on one replica appears to work and does not —
// the single most likely production incident in this stack (PRD §7.6).
func NewCasbin(db *gorm.DB, core goredis.UniversalClient, log *zap.Logger) (*Casbin, func(), error) {
	m, err := casbinmodel.NewModelFromString(modelConf)
	if err != nil {
		return nil, nil, errs.Wrap(errs.Internal, "casbin model", err)
	}

	// goose owns the schema; the adapter must never CREATE TABLE (PRD §7.1).
	// The schema qualifier rides the table name: the adapter's own prefix
	// parameter joins with "_", and explicit table names bypass GORM's
	// TablePrefix — so this is the one reliable spelling.
	gormadapter.TurnOffAutoMigrate(db)
	adapter, err := gormadapter.NewAdapterByDBUseTableName(db, "", "authz_svc.casbin_rule")
	if err != nil {
		return nil, nil, errs.Wrap(errs.Internal, "casbin adapter", err)
	}

	// Model-only construction, adapter attached afterwards: the combined
	// constructor auto-loads policy, and on a FRESH database (before the
	// very first cmd/migrate) the table does not exist yet — the enforcer
	// must still construct so migrate can run at all.
	e, err := casbin.NewSyncedEnforcer(m)
	if err != nil {
		return nil, nil, errs.Wrap(errs.Internal, "casbin enforcer", err)
	}
	e.SetAdapter(adapter)

	// Multi-replica staleness fix: policy writes publish on redis-core;
	// every replica reloads (R19: propagation within 5s).
	watcher, err := rediswatcher.NewWatcher("", rediswatcher.WatcherOptions{
		SubClient: core,
		PubClient: core,
		Channel:   "casbin:policy",
	})
	if err != nil {
		return nil, nil, errs.Wrap(errs.Internal, "casbin watcher", err)
	}
	if err := e.SetWatcher(watcher); err != nil {
		return nil, nil, errs.Wrap(errs.Internal, "casbin set watcher", err)
	}
	if err := watcher.SetUpdateCallback(func(string) {
		if err := e.LoadPolicy(); err != nil {
			log.Error("casbin policy reload failed", zap.Error(err))
		}
	}); err != nil {
		return nil, nil, errs.Wrap(errs.Internal, "casbin watcher callback", err)
	}

	// Tolerate a missing table at construction (first boot before migrate):
	// enforcement then denies everything, which fails safe. Serving
	// binaries call VerifyCatalogue afterwards, which reloads and fails
	// loudly on real drift (R19).
	if err := e.LoadPolicy(); err != nil {
		log.Warn("casbin policy load failed — starting with empty policy (deny-all)", zap.Error(err))
	}

	cleanup := func() { watcher.Close() }
	return &Casbin{e: e, log: log}, cleanup, nil
}

// Authorize answers the role question. resource is accepted so services can
// pass the loaded entity, but ownership stays a service-layer check
// (§7.6 constraint 3) — the matcher never sees it.
func (c *Casbin) Authorize(_ context.Context, p Principal, perm Permission, _ any) error {
	subjects := append([]string{p.Subject()}, p.RoleSubjects()...)
	for _, sub := range subjects {
		ok, err := c.e.Enforce(sub, perm.Key())
		if err != nil {
			return errs.Wrap(errs.Internal, "casbin enforce", err)
		}
		if ok {
			return nil
		}
	}
	return errs.New(errs.PermissionDenied, "missing permission "+perm.Key())
}

// Grant allows a role to exercise a permission. IDs, never names.
func (c *Casbin) Grant(roleID int64, perm Permission) error {
	_, err := c.e.AddPolicy(roleSubject(roleID), perm.Key())
	return err
}

// Revoke removes a role's permission.
func (c *Casbin) Revoke(roleID int64, perm Permission) error {
	_, err := c.e.RemovePolicy(roleSubject(roleID), perm.Key())
	return err
}

// AssignRole links a user to a role.
func (c *Casbin) AssignRole(userID string, roleID int64) error {
	_, err := c.e.AddGroupingPolicy("user:"+userID, roleSubject(roleID))
	return err
}

// GrantedKeys returns every permission key any policy row references —
// input to the boot-time catalogue check (R19).
func (c *Casbin) GrantedKeys() ([]string, error) {
	policies, err := c.e.GetPolicy()
	if err != nil {
		return nil, errs.Wrap(errs.Internal, "casbin get policy", err)
	}
	keys := make([]string, 0, len(policies))
	for _, row := range policies {
		if len(row) >= 2 {
			keys = append(keys, row[1])
		}
	}
	return keys, nil
}

// Reload re-reads policy from storage (used before the catalogue check).
func (c *Casbin) Reload() error { return c.e.LoadPolicy() }

func roleSubject(id int64) string {
	return Principal{RoleIDs: []int64{id}}.RoleSubjects()[0]
}
