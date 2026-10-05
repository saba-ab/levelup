package repo

import (
	"context"
	"errors"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgconn"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	"levelup/internal/modules/identity/internal/app"
	"levelup/internal/modules/identity/internal/domain"
	"levelup/internal/shared/errs"
)

// Postgres implements app.Repository.
type Postgres struct{ db *gorm.DB }

func NewPostgres(db *gorm.DB) *Postgres { return &Postgres{db: db} }

var _ app.Repository = (*Postgres)(nil)

// ---- tenants ----

func (r *Postgres) CreateTenant(ctx context.Context, tx *gorm.DB, t domain.Tenant) error {
	m, err := tenantFromDomain(t)
	if err != nil {
		return errs.Wrap(errs.Invalid, "encode tenant settings", err)
	}
	if err := tx.WithContext(ctx).Create(&m).Error; err != nil {
		if uniqueViolation(err, "tenants_slug_key") {
			return domain.ErrSlugTaken
		}
		return errs.Wrap(errs.Internal, "insert tenant", err)
	}
	return nil
}

func (r *Postgres) TenantByID(ctx context.Context, id string) (domain.Tenant, error) {
	return r.loadTenant(r.db.WithContext(ctx), id)
}

func (r *Postgres) TenantByIDForUpdate(ctx context.Context, tx *gorm.DB, id string) (domain.Tenant, error) {
	return r.loadTenant(tx.WithContext(ctx).Clauses(clause.Locking{Strength: "UPDATE"}), id)
}

func (r *Postgres) loadTenant(q *gorm.DB, id string) (domain.Tenant, error) {
	if !validUUID(id) {
		return domain.Tenant{}, domain.ErrTenantNotFound
	}
	var m tenant
	err := q.Where("id = ? AND deleted_at IS NULL", id).Take(&m).Error
	switch {
	case errors.Is(err, gorm.ErrRecordNotFound):
		return domain.Tenant{}, domain.ErrTenantNotFound
	case err != nil:
		return domain.Tenant{}, errs.Wrap(errs.Internal, "load tenant", err)
	}
	return m.toDomain(), nil
}

func (r *Postgres) TenantsByIDs(ctx context.Context, ids []string) ([]domain.Tenant, error) {
	var ms []tenant
	if err := r.db.WithContext(ctx).
		Where("id IN ? AND deleted_at IS NULL", ids).
		Find(&ms).Error; err != nil {
		return nil, errs.Wrap(errs.Internal, "load tenants", err)
	}
	out := make([]domain.Tenant, len(ms))
	for i, m := range ms {
		out[i] = m.toDomain()
	}
	return out, nil
}

func (r *Postgres) ListTenants(ctx context.Context, page app.Page) ([]domain.Tenant, error) {
	q := r.db.WithContext(ctx).Where("deleted_at IS NULL")
	q = keyset(q, page)
	var ms []tenant
	if err := q.Find(&ms).Error; err != nil {
		return nil, errs.Wrap(errs.Internal, "list tenants", err)
	}
	out := make([]domain.Tenant, len(ms))
	for i, m := range ms {
		out[i] = m.toDomain()
	}
	return out, nil
}

// SaveTenant writes mutable fields guarded by the optimistic version.
func (r *Postgres) SaveTenant(ctx context.Context, tx *gorm.DB, t domain.Tenant) error {
	m, err := tenantFromDomain(t)
	if err != nil {
		return errs.Wrap(errs.Invalid, "encode tenant settings", err)
	}
	res := tx.WithContext(ctx).Model(&tenant{}).
		Where("id = ? AND version = ?", m.ID, m.Version).
		Updates(map[string]any{
			"name":          m.Name,
			"owner_user_id": m.OwnerUserID,
			"active":        m.Active,
			"timezone":      m.Timezone,
			"settings":      gorm.Expr("?::jsonb", m.Settings),
			"updated_at":    m.UpdatedAt,
			"deleted_at":    m.DeletedAt,
			"version":       gorm.Expr("version + 1"),
		})
	if res.Error != nil {
		return errs.Wrap(errs.Internal, "save tenant", res.Error)
	}
	if res.RowsAffected == 0 {
		return domain.ErrVersionConflict
	}
	return nil
}

// ---- users ----

func (r *Postgres) CreateUser(ctx context.Context, tx *gorm.DB, u domain.User) error {
	m := userFromDomain(u)
	if err := tx.WithContext(ctx).Create(&m).Error; err != nil {
		if uniqueViolation(err, "users_email_key") {
			return domain.ErrEmailTaken
		}
		return errs.Wrap(errs.Internal, "insert user", err)
	}
	return r.insertRoles(ctx, tx, u.ID, u.RoleIDs, u.CreatedAt)
}

func (r *Postgres) insertRoles(ctx context.Context, tx *gorm.DB, userID string, roleIDs []int64, at time.Time) error {
	if len(roleIDs) == 0 {
		return nil
	}
	rows := make([]userRole, len(roleIDs))
	for i, id := range roleIDs {
		rows[i] = userRole{UserID: userID, RoleID: id, GrantedAt: at}
	}
	if err := tx.WithContext(ctx).Create(&rows).Error; err != nil {
		return errs.Wrap(errs.Internal, "insert user roles", err)
	}
	return nil
}

func (r *Postgres) UserInTenant(ctx context.Context, tenantID, id string) (domain.User, error) {
	return r.loadUser(ctx, r.db.WithContext(ctx), "id = ? AND tenant_id = ?", id, tenantID)
}

func (r *Postgres) UserInTenantForUpdate(ctx context.Context, tx *gorm.DB, tenantID, id string) (domain.User, error) {
	q := tx.WithContext(ctx).Clauses(clause.Locking{Strength: "UPDATE"})
	return r.loadUserWith(ctx, q, tx, "id = ? AND tenant_id = ?", id, tenantID)
}

func (r *Postgres) UserByID(ctx context.Context, id string) (domain.User, error) {
	return r.loadUser(ctx, r.db.WithContext(ctx), "id = ?", id)
}

func (r *Postgres) UserByEmail(ctx context.Context, email string) (domain.User, error) {
	var m user
	err := r.db.WithContext(ctx).Where("lower(email) = lower(?)", email).Take(&m).Error
	switch {
	case errors.Is(err, gorm.ErrRecordNotFound):
		return domain.User{}, domain.ErrUserNotFound
	case err != nil:
		return domain.User{}, errs.Wrap(errs.Internal, "load user by email", err)
	}
	roles, err := r.rolesOf(ctx, r.db, []string{m.ID})
	if err != nil {
		return domain.User{}, err
	}
	return m.toDomain(roles[m.ID]), nil
}

func (r *Postgres) loadUser(ctx context.Context, q *gorm.DB, where string, args ...any) (domain.User, error) {
	return r.loadUserWith(ctx, q, r.db, where, args...)
}

// loadUserWith reads the user through q and its roles through rolesDB (the
// tx when locking, so both reads see one snapshot).
func (r *Postgres) loadUserWith(ctx context.Context, q, rolesDB *gorm.DB, where string, args ...any) (domain.User, error) {
	for _, a := range args {
		if s, ok := a.(string); ok && !validUUID(s) {
			return domain.User{}, domain.ErrUserNotFound
		}
	}
	var m user
	err := q.Where(where, args...).Take(&m).Error
	switch {
	case errors.Is(err, gorm.ErrRecordNotFound):
		return domain.User{}, domain.ErrUserNotFound
	case err != nil:
		return domain.User{}, errs.Wrap(errs.Internal, "load user", err)
	}
	roles, err := r.rolesOf(ctx, rolesDB, []string{m.ID})
	if err != nil {
		return domain.User{}, err
	}
	return m.toDomain(roles[m.ID]), nil
}

func (r *Postgres) rolesOf(ctx context.Context, db *gorm.DB, userIDs []string) (map[string][]int64, error) {
	out := make(map[string][]int64, len(userIDs))
	if len(userIDs) == 0 {
		return out, nil
	}
	var rows []userRole
	if err := db.WithContext(ctx).
		Where("user_id IN ?", userIDs).
		Order("user_id, role_id").
		Find(&rows).Error; err != nil {
		return nil, errs.Wrap(errs.Internal, "load user roles", err)
	}
	for _, row := range rows {
		out[row.UserID] = append(out[row.UserID], row.RoleID)
	}
	return out, nil
}

func (r *Postgres) ListUsers(ctx context.Context, tenantID string, page app.Page) ([]domain.User, error) {
	q := keyset(r.db.WithContext(ctx).Where("tenant_id = ?", tenantID), page)
	var ms []user
	if err := q.Find(&ms).Error; err != nil {
		return nil, errs.Wrap(errs.Internal, "list users", err)
	}
	ids := make([]string, len(ms))
	for i, m := range ms {
		ids[i] = m.ID
	}
	roles, err := r.rolesOf(ctx, r.db, ids)
	if err != nil {
		return nil, err
	}
	out := make([]domain.User, len(ms))
	for i, m := range ms {
		out[i] = m.toDomain(roles[m.ID])
	}
	return out, nil
}

func (r *Postgres) UserIDsInTenant(ctx context.Context, tenantID string) ([]string, error) {
	var ids []string
	if err := r.db.WithContext(ctx).Model(&user{}).
		Where("tenant_id = ?", tenantID).
		Pluck("id", &ids).Error; err != nil {
		return nil, errs.Wrap(errs.Internal, "list tenant member ids", err)
	}
	return ids, nil
}

// SaveUser writes profile fields guarded by the optimistic version. A
// duplicate email is ErrEmailTaken (409), never a 500 (doc 02 §10 bug 13).
func (r *Postgres) SaveUser(ctx context.Context, tx *gorm.DB, u domain.User) error {
	m := userFromDomain(u)
	res := tx.WithContext(ctx).Model(&user{}).
		Where("id = ? AND version = ?", m.ID, m.Version).
		Updates(map[string]any{
			"name":                m.Name,
			"email":               m.Email,
			"password_hash":       m.PasswordHash,
			"active":              m.Active,
			"email_verified_at":   m.EmailVerifiedAt,
			"password_changed_at": m.PasswordChangedAt,
			"updated_at":          m.UpdatedAt,
			"version":             gorm.Expr("version + 1"),
		})
	if res.Error != nil {
		if uniqueViolation(res.Error, "users_email_key") {
			return domain.ErrEmailTaken
		}
		return errs.Wrap(errs.Internal, "save user", res.Error)
	}
	if res.RowsAffected == 0 {
		return domain.ErrVersionConflict
	}
	return nil
}

func (r *Postgres) SetUserRoles(ctx context.Context, tx *gorm.DB, userID string, roleIDs []int64, at time.Time) error {
	if err := tx.WithContext(ctx).Where("user_id = ?", userID).Delete(&userRole{}).Error; err != nil {
		return errs.Wrap(errs.Internal, "clear user roles", err)
	}
	return r.insertRoles(ctx, tx, userID, roleIDs, at)
}

// UpdatePasswordHash is the opportunistic rehash: no version bump, the
// password itself did not change.
func (r *Postgres) UpdatePasswordHash(ctx context.Context, tx *gorm.DB, userID, hash string) error {
	if err := tx.WithContext(ctx).Model(&user{}).
		Where("id = ?", userID).
		Update("password_hash", hash).Error; err != nil {
		return errs.Wrap(errs.Internal, "rehash password", err)
	}
	return nil
}

func (r *Postgres) DeleteUser(ctx context.Context, tx *gorm.DB, tenantID, id string) error {
	res := tx.WithContext(ctx).Where("id = ? AND tenant_id = ?", id, tenantID).Delete(&user{})
	if res.Error != nil {
		return errs.Wrap(errs.Internal, "delete user", res.Error)
	}
	if res.RowsAffected == 0 {
		return domain.ErrUserNotFound
	}
	return nil
}

// ---- helpers ----

// keyset applies (created_at, id) DESC paging, fetching one probe row.
func keyset(q *gorm.DB, page app.Page) *gorm.DB {
	if !page.AfterCreated.IsZero() && validUUID(page.AfterID) {
		q = q.Where("(created_at, id) < (?, ?)", page.AfterCreated, page.AfterID)
	}
	return q.Order("created_at DESC, id DESC").Limit(page.Limit + 1)
}

func validUUID(s string) bool {
	_, err := uuid.Parse(s)
	return err == nil
}

func uniqueViolation(err error, constraint string) bool {
	var pgErr *pgconn.PgError
	if !errors.As(err, &pgErr) || pgErr.Code != "23505" {
		return false
	}
	return constraint == "" || strings.EqualFold(pgErr.ConstraintName, constraint)
}
