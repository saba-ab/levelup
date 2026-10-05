package migrations

import (
	"context"
	"database/sql"
	"fmt"

	"github.com/pressly/goose/v3"

	"levelup/internal/modules/eventcatalog/contracts"
	identitycontracts "levelup/internal/modules/identity/contracts"
	"levelup/internal/platform/authz"
	"levelup/internal/shared/id"
)

// Go returns the typed migrations. Versions continue after 0001_init.sql.
//
//	2: permission catalogue
//	3: default role grants
//	4: the global category and event-type catalogue
func Go() []*goose.Migration {
	return []*goose.Migration{
		goose.NewGoMigration(2,
			&goose.GoFunc{RunTx: upSeedPermissions},
			&goose.GoFunc{RunTx: downSeedPermissions},
		),
		goose.NewGoMigration(3,
			&goose.GoFunc{RunTx: upSeedGrants},
			&goose.GoFunc{RunTx: downSeedGrants},
		),
		CatalogMigration(),
	}
}

// CatalogMigration is version 4 on its own, so repository tests can seed the
// catalogue without the authz schema.
func CatalogMigration() *goose.Migration {
	return goose.NewGoMigration(4,
		&goose.GoFunc{RunTx: upSeedCatalog},
		&goose.GoFunc{RunTx: downSeedCatalog},
	)
}

// --- permissions ---------------------------------------------------------------

func upSeedPermissions(ctx context.Context, tx *sql.Tx) error {
	for _, p := range contracts.AllPermissions {
		if _, err := tx.ExecContext(ctx,
			`INSERT INTO authz_svc.permissions (key, module) VALUES ($1, $2)
			 ON CONFLICT (key) DO NOTHING`, p.Key(), p.Module); err != nil {
			return err
		}
	}
	return nil
}

func downSeedPermissions(ctx context.Context, tx *sql.Tx) error {
	_, err := tx.ExecContext(ctx, `DELETE FROM authz_svc.permissions WHERE module = $1`, contracts.Module)
	return err
}

// GrantedRoles is the default grant matrix (IMPLEMENTATION.md §3, doc 03
// §11.5): delete is an admin permission, manage_global is platform-only,
// everything else goes to every tenant role.
func GrantedRoles(p authz.Permission) []int64 {
	switch p {
	case contracts.PermDelete:
		return identitycontracts.AdminRoles
	case contracts.PermManageGlobal:
		return []int64{identitycontracts.RolePlatformAdmin}
	default:
		return identitycontracts.MemberRoles
	}
}

func upSeedGrants(ctx context.Context, tx *sql.Tx) error {
	for _, p := range contracts.AllPermissions {
		for _, role := range GrantedRoles(p) {
			// NOT EXISTS rather than ON CONFLICT: the unique index spans the
			// NULL v2..v5 columns, and NULLs never conflict.
			if _, err := tx.ExecContext(ctx,
				`INSERT INTO authz_svc.casbin_rule (ptype, v0, v1)
				 SELECT 'p', $1::varchar, $2::varchar
				 WHERE NOT EXISTS (
				     SELECT 1 FROM authz_svc.casbin_rule
				     WHERE ptype = 'p' AND v0 = $1::varchar AND v1 = $2::varchar)`,
				fmt.Sprintf("role:%d", role), p.Key()); err != nil {
				return err
			}
		}
	}
	return nil
}

func downSeedGrants(ctx context.Context, tx *sql.Tx) error {
	_, err := tx.ExecContext(ctx,
		`DELETE FROM authz_svc.casbin_rule WHERE ptype = 'p' AND v1 LIKE $1`, contracts.Module+":%")
	return err
}

// --- global catalogue --------------------------------------------------------------

// SeedCategory and SeedEventType describe the global catalogue. Ids are
// derived (id.Derive), so every environment and the data migration compute
// the same uuid for the same slug.
type SeedCategory struct {
	Slug, Name, Description string
	SortOrder               int
}

type SeedEventType struct {
	Slug, Name, Description, Category string
}

// CategoryID and EventTypeID are the deterministic ids of seeded rows.
func CategoryID(slug string) string  { return id.Derive("eventcatalog", "event_category", slug) }
func EventTypeID(slug string) string { return id.Derive("eventcatalog", "event_type", slug) }

// GlobalCategories: Laravel seeded none (categories were Filament-only);
// these group the predefined types for catalogue UIs.
var GlobalCategories = []SeedCategory{
	{"account", "Account", "Sign-ups, logins and other account lifecycle events", 10},
	{"commerce", "Commerce", "Purchases and subscription changes", 20},
	{"engagement", "Engagement", "Tasks and other in-product engagement", 30},
	{"social", "Social", "Referrals and other social events", 40},
	{"gamification", "Gamification", "Progress inside the gamification programme itself", 50},
}

// GlobalEventTypes are the 10 predefined events of Laravel's
// PredefinedEventsSeeder, verbatim (slug, name, description), all active.
var GlobalEventTypes = []SeedEventType{
	{"purchase_completed", "Purchase Completed", "Triggered when a user completes a purchase", "commerce"},
	{"user_signup", "User Signup", "Triggered when a new user signs up", "account"},
	{"user_login", "User Login", "Triggered when a user logs in", "account"},
	{"referral_completed", "Referral Completed", "Triggered when a referral is completed", "social"},
	{"subscription_upgraded", "Subscription Upgraded", "Triggered when a user upgrades their subscription", "commerce"},
	{"task_completed", "Task Completed", "Triggered when a task is completed", "engagement"},
	{"achievement_unlocked", "Achievement Unlocked", "Triggered when an achievement is unlocked", "gamification"},
	{"level_up", "Level Up", "Triggered when a player levels up", "gamification"},
	{"badge_earned", "Badge Earned", "Triggered when a badge is earned", "gamification"},
	{"mission_completed", "Mission Completed", "Triggered when a mission is completed", "gamification"},
}

func upSeedCatalog(ctx context.Context, tx *sql.Tx) error {
	for _, c := range GlobalCategories {
		if _, err := tx.ExecContext(ctx,
			`INSERT INTO eventcatalog_svc.event_categories
			     (id, tenant_id, slug, name, description, sort_order, created_at, updated_at)
			 VALUES ($1, NULL, $2, $3, $4, $5, now(), now())
			 ON CONFLICT DO NOTHING`,
			CategoryID(c.Slug), c.Slug, c.Name, c.Description, c.SortOrder); err != nil {
			return err
		}
	}
	for _, e := range GlobalEventTypes {
		// The category is looked up by slug: should a global category with
		// that slug already exist under another id, the type still links.
		if _, err := tx.ExecContext(ctx,
			`INSERT INTO eventcatalog_svc.event_types
			     (id, tenant_id, category_id, slug, name, description, is_active, created_at, updated_at)
			 VALUES ($1, NULL,
			     (SELECT id FROM eventcatalog_svc.event_categories WHERE tenant_id IS NULL AND slug = $2),
			     $3, $4, $5, TRUE, now(), now())
			 ON CONFLICT DO NOTHING`,
			EventTypeID(e.Slug), e.Category, e.Slug, e.Name, e.Description); err != nil {
			return err
		}
	}
	return nil
}

func downSeedCatalog(ctx context.Context, tx *sql.Tx) error {
	for _, e := range GlobalEventTypes {
		if _, err := tx.ExecContext(ctx,
			`DELETE FROM eventcatalog_svc.event_types WHERE id = $1`, EventTypeID(e.Slug)); err != nil {
			return err
		}
	}
	for _, c := range GlobalCategories {
		if _, err := tx.ExecContext(ctx,
			`DELETE FROM eventcatalog_svc.event_categories WHERE id = $1`, CategoryID(c.Slug)); err != nil {
			return err
		}
	}
	return nil
}
