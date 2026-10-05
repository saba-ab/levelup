package app

import (
	"context"
	"slices"
	"time"

	"gorm.io/gorm"

	"levelup/internal/modules/identity/contracts"
	"levelup/internal/modules/identity/internal/domain"
	"levelup/internal/platform/authz"
)

// ListUsers is tenant-scoped by construction: the tenant comes from the
// principal, never a parameter (fixes doc 02 §10 bug 1).
func (s *Service) ListUsers(ctx context.Context, limit int, cursor string) ([]domain.User, string, error) {
	p, err := authz.RequireTenant(ctx)
	if err != nil {
		return nil, "", err
	}
	if err := s.authz.Authorize(ctx, p, contracts.PermUsersViewAny, nil); err != nil {
		return nil, "", err
	}
	page, err := pageFrom(limit, cursor)
	if err != nil {
		return nil, "", err
	}
	rows, err := s.repo.ListUsers(ctx, p.TenantID, page)
	if err != nil {
		return nil, "", err
	}
	out, next := trimPage(rows, page.Limit, func(u domain.User) (time.Time, string) { return u.CreatedAt, u.ID })
	return out, next, nil
}

// GetUser: self always; anyone else needs users_view_any. Another tenant's
// user is 404.
func (s *Service) GetUser(ctx context.Context, id string) (domain.User, error) {
	p, err := authz.RequireTenant(ctx)
	if err != nil {
		return domain.User{}, err
	}
	if id != p.UserID {
		if err := s.authz.Authorize(ctx, p, contracts.PermUsersViewAny, nil); err != nil {
			return domain.User{}, err
		}
	}
	return s.repo.UserInTenant(ctx, p.TenantID, id)
}

type CreateUserCmd struct {
	Name     string
	Email    string
	Password string
	RoleIDs  []int64
}

// CreateUser adds a user to the CALLER's tenant (never one from the request:
// fixes doc 02 §10 bug 2). Requested roles must be tenant roles no senior
// to the caller's own; the platform role is never grantable here.
func (s *Service) CreateUser(ctx context.Context, cmd CreateUserCmd) (domain.User, error) {
	p, err := authz.RequireTenant(ctx)
	if err != nil {
		return domain.User{}, err
	}
	if err := s.authz.Authorize(ctx, p, contracts.PermUsersCreate, nil); err != nil {
		return domain.User{}, err
	}
	roles := domain.NormalizeRoleIDs(cmd.RoleIDs)
	if err := domain.CheckGrantable(p.RoleIDs, roles); err != nil {
		return domain.User{}, err
	}
	now := s.clock.Now()
	u, err := domain.NewUser(p.TenantID, cmd.Name, cmd.Email, "", roles, now)
	if err != nil {
		return domain.User{}, err
	}
	if u.PasswordHash, err = s.pw.Hash(cmd.Password); err != nil {
		return domain.User{}, err
	}
	err = s.tx(ctx, func(tx *gorm.DB) error {
		if err := s.repo.CreateUser(ctx, tx, u); err != nil {
			return err
		}
		return s.outbox.Publish(ctx, tx, contracts.TopicUserCreated, contracts.UserCreatedV1{
			UserID: u.ID, TenantID: u.TenantID, Email: u.Email, Name: u.Name,
			RoleIDs: roleIDsOrEmpty(u.RoleIDs), CreatedBy: p.UserID, At: now,
		})
	})
	if err != nil {
		return domain.User{}, err
	}
	return u, nil
}

type UpdateUserCmd struct {
	Name            *string
	Email           *string
	Password        *string
	CurrentPassword *string
	Active          *bool
}

// UpdateUser: a user may always update self (changing one's own password
// needs current_password, doc 02 §10 bug 4); anyone else needs users_update
// and must not be outranked by the target. A password change or a
// deactivation revokes the target's refresh tokens after commit.
func (s *Service) UpdateUser(ctx context.Context, id string, cmd UpdateUserCmd) (domain.User, error) {
	p, err := authz.RequireTenant(ctx)
	if err != nil {
		return domain.User{}, err
	}
	self := id == p.UserID
	if !self {
		if err := s.authz.Authorize(ctx, p, contracts.PermUsersUpdate, nil); err != nil {
			return domain.User{}, err
		}
	}
	if self && cmd.Active != nil && !*cmd.Active {
		return domain.User{}, domain.ErrCannotDeactivateSelf
	}

	var newHash string
	if cmd.Password != nil {
		if self {
			if cmd.CurrentPassword == nil || *cmd.CurrentPassword == "" {
				return domain.User{}, domain.ErrCurrentPasswordRequired
			}
			current, err := s.repo.UserInTenant(ctx, p.TenantID, id)
			if err != nil {
				return domain.User{}, err
			}
			if !s.pw.Matches(current.PasswordHash, *cmd.CurrentPassword) {
				return domain.User{}, domain.ErrCurrentPasswordWrong
			}
		}
		if newHash, err = s.pw.Hash(*cmd.Password); err != nil {
			return domain.User{}, err
		}
	}

	now := s.clock.Now()
	var (
		out         domain.User
		revoke      bool
		deactivated bool
	)
	err = s.tx(ctx, func(tx *gorm.DB) error {
		u, err := s.repo.UserInTenantForUpdate(ctx, tx, p.TenantID, id)
		if err != nil {
			return err
		}
		if !self {
			if err := domain.CheckCanManage(p.RoleIDs, u.RoleIDs); err != nil {
				return err
			}
		}
		changed := false
		if cmd.Name != nil {
			c, err := u.Rename(*cmd.Name, now)
			if err != nil {
				return err
			}
			changed = changed || c
		}
		if cmd.Email != nil {
			c, err := u.ChangeEmail(*cmd.Email, now)
			if err != nil {
				return err
			}
			changed = changed || c
		}
		if newHash != "" {
			u.SetPasswordHash(newHash, now)
			changed, revoke = true, true
		}
		if cmd.Active != nil && !self {
			if !*cmd.Active {
				t, err := s.repo.TenantByID(ctx, p.TenantID)
				if err != nil {
					return err
				}
				if t.OwnedBy(u.ID) {
					return domain.ErrOwnerCannotBeDeactivated
				}
			}
			if u.SetActive(*cmd.Active, now) {
				changed = true
				deactivated = !u.Active
			}
		}
		out = u
		if !changed {
			return nil
		}
		if err := s.repo.SaveUser(ctx, tx, u); err != nil {
			return err
		}
		out.Version++
		return s.outbox.Publish(ctx, tx, contracts.TopicUserUpdated, contracts.UserUpdatedV1{
			UserID: u.ID, TenantID: u.TenantID, Email: u.Email, Name: u.Name, Active: u.Active, At: now,
		})
	})
	if err != nil {
		return domain.User{}, err
	}
	if revoke || deactivated {
		s.revokeSessions(ctx, out.ID)
	}
	return out, nil
}

// DeleteUser hard-deletes a member (roles cascade). The tenant owner cannot
// be deleted (fixes doc 02 §10 bug 16); ownership must be transferred first.
func (s *Service) DeleteUser(ctx context.Context, id string) error {
	p, err := authz.RequireTenant(ctx)
	if err != nil {
		return err
	}
	if err := s.authz.Authorize(ctx, p, contracts.PermUsersDelete, nil); err != nil {
		return err
	}
	now := s.clock.Now()
	err = s.tx(ctx, func(tx *gorm.DB) error {
		u, err := s.repo.UserInTenantForUpdate(ctx, tx, p.TenantID, id)
		if err != nil {
			return err
		}
		if err := domain.CheckCanManage(p.RoleIDs, u.RoleIDs); err != nil {
			return err
		}
		t, err := s.repo.TenantByIDForUpdate(ctx, tx, p.TenantID)
		if err != nil {
			return err
		}
		if t.OwnedBy(u.ID) {
			return domain.ErrOwnerCannotBeDeleted
		}
		if err := s.repo.DeleteUser(ctx, tx, p.TenantID, u.ID); err != nil {
			return err
		}
		return s.outbox.Publish(ctx, tx, contracts.TopicUserDeleted, contracts.UserDeletedV1{
			UserID: u.ID, TenantID: u.TenantID, At: now,
		})
	})
	if err != nil {
		return err
	}
	s.revokeSessions(ctx, id)
	return nil
}

// AssignRoles replaces a member's role set. Same rules as create (tenant
// roles only, no escalation, never the platform role) plus: the target must
// not outrank the caller, and the tenant owner keeps the owner role. A real
// change revokes the target's refresh tokens so new roles apply within one
// access TTL (ADR-0015).
func (s *Service) AssignRoles(ctx context.Context, id string, roleIDs []int64) (domain.User, error) {
	p, err := authz.RequireTenant(ctx)
	if err != nil {
		return domain.User{}, err
	}
	if err := s.authz.Authorize(ctx, p, contracts.PermUsersAssignRoles, nil); err != nil {
		return domain.User{}, err
	}
	roles := domain.NormalizeRoleIDs(roleIDs)
	if err := domain.CheckGrantable(p.RoleIDs, roles); err != nil {
		return domain.User{}, err
	}
	now := s.clock.Now()
	var (
		out     domain.User
		changed bool
	)
	err = s.tx(ctx, func(tx *gorm.DB) error {
		u, err := s.repo.UserInTenantForUpdate(ctx, tx, p.TenantID, id)
		if err != nil {
			return err
		}
		if err := domain.CheckCanManage(p.RoleIDs, u.RoleIDs); err != nil {
			return err
		}
		if !slices.Contains(roles, contracts.RoleOwner) {
			t, err := s.repo.TenantByID(ctx, p.TenantID)
			if err != nil {
				return err
			}
			if t.OwnedBy(u.ID) {
				return domain.ErrOwnerRoleRequired
			}
		}
		out = u
		if changed = u.ReplaceRoles(roles, now); !changed {
			return nil
		}
		if err := s.repo.SetUserRoles(ctx, tx, u.ID, u.RoleIDs, now); err != nil {
			return err
		}
		if err := s.repo.SaveUser(ctx, tx, u); err != nil {
			return err
		}
		out = u
		out.Version++
		return s.outbox.Publish(ctx, tx, contracts.TopicUserRolesChanged, contracts.UserRolesChangedV1{
			UserID: u.ID, TenantID: u.TenantID, RoleIDs: roleIDsOrEmpty(u.RoleIDs), At: now,
		})
	})
	if err != nil {
		return domain.User{}, err
	}
	if changed {
		s.revokeSessions(ctx, out.ID)
	}
	return out, nil
}

func roleIDsOrEmpty(ids []int64) []int64 {
	if ids == nil {
		return []int64{}
	}
	return ids
}
