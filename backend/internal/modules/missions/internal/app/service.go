// Package app holds missions' use cases, transaction boundaries,
// authorization and every outbox publish (ADR-0013).
package app

import (
	"context"
	"time"

	"gorm.io/gorm"

	"levelup/internal/modules/missions/contracts"
	"levelup/internal/modules/missions/internal/domain"
	"levelup/internal/modules/missions/internal/ports"
	"levelup/internal/platform/authz"
	"levelup/internal/platform/clock"
	"levelup/internal/platform/outbox"
	"levelup/internal/platform/postgres"
	"levelup/internal/shared/errs"
	"levelup/internal/shared/pagination"
)

const (
	defaultPageSize = 25
	maxPageSize     = 100
	defaultBatch    = 500
)

type Service struct {
	repo    Repository
	players ports.PlayerReader
	outbox  outbox.Store
	authz   authz.Enforcer
	db      *gorm.DB
	clock   clock.Clock
	batch   int
	// maxDepth caps CausationDepth of activities that progress missions.
	maxDepth int

	tx func(ctx context.Context, fn func(tx *gorm.DB) error) error
}

// NewService wires the service. batch bounds the rows one sweep transaction
// touches (<= 0 means the default, 500).
func NewService(repo Repository, players ports.PlayerReader, ob outbox.Store,
	enf authz.Enforcer, db *gorm.DB, c clock.Clock, batch int) *Service {
	if batch <= 0 {
		batch = defaultBatch
	}
	s := &Service{repo: repo, players: players, outbox: ob, authz: enf, db: db, clock: c, batch: batch,
		maxDepth: DefaultMaxCausationDepth}
	s.tx = func(ctx context.Context, fn func(tx *gorm.DB) error) error {
		return postgres.InTx(ctx, s.db, fn)
	}
	return s
}

// SetMaxCausationDepth overrides the loop guard of the activity subscriber
// (<= 0 keeps the default).
func (s *Service) SetMaxCausationDepth(n int) {
	if n > 0 {
		s.maxDepth = n
	}
}

// Page is one keyset page; NextCursor is "" on the last page.
type Page[T any] struct {
	Items      []T
	NextCursor string
}

// CreateMissionCmd carries the creatable fields. The tenant comes from the
// principal, never from the request.
type CreateMissionCmd struct {
	Slug                    string
	Name                    string
	Description             string
	Type                    string
	Status                  string
	Target                  int64
	Criteria                map[string]any
	PointsReward            int64
	XPReward                int64
	BadgeRewardID           string
	MaxCompletionsPerPlayer *int
	StartsAt                *time.Time
	EndsAt                  *time.Time
}

// UpdateMissionCmd is a partial update: nil leaves a field untouched.
// BadgeRewardID "" clears the badge, MaxCompletionsPerPlayer 0 means
// unlimited, ClearStartsAt/ClearEndsAt remove a window bound.
type UpdateMissionCmd struct {
	Slug                    *string
	Name                    *string
	Description             *string
	Type                    *string
	Status                  *string
	Target                  *int64
	Criteria                map[string]any
	PointsReward            *int64
	XPReward                *int64
	BadgeRewardID           *string
	MaxCompletionsPerPlayer *int
	StartsAt                *time.Time
	EndsAt                  *time.Time
	ClearStartsAt           bool
	ClearEndsAt             bool
}

func (s *Service) CreateMission(ctx context.Context, cmd CreateMissionCmd) (domain.Mission, error) {
	p, err := authz.RequireTenant(ctx)
	if err != nil {
		return domain.Mission{}, err
	}
	if err := s.authz.Authorize(ctx, p, contracts.PermCreate, nil); err != nil {
		return domain.Mission{}, err
	}
	now := s.clock.Now()
	m, err := domain.NewMission(domain.NewMissionParams{
		TenantID:                p.TenantID,
		Slug:                    cmd.Slug,
		Name:                    cmd.Name,
		Description:             cmd.Description,
		Type:                    cmd.Type,
		Status:                  cmd.Status,
		Target:                  cmd.Target,
		Criteria:                cmd.Criteria,
		PointsReward:            cmd.PointsReward,
		XPReward:                cmd.XPReward,
		BadgeRewardID:           cmd.BadgeRewardID,
		MaxCompletionsPerPlayer: cmd.MaxCompletionsPerPlayer,
		StartsAt:                cmd.StartsAt,
		EndsAt:                  cmd.EndsAt,
	}, now)
	if err != nil {
		return domain.Mission{}, err
	}
	err = s.tx(ctx, func(tx *gorm.DB) error {
		if err := s.repo.CreateMission(ctx, tx, m); err != nil {
			return err
		}
		return s.outbox.Publish(ctx, tx, contracts.TopicMissionCreated, changed(m, now))
	})
	if err != nil {
		return domain.Mission{}, err
	}
	return m, nil
}

func (s *Service) GetMission(ctx context.Context, missionID string) (domain.Mission, error) {
	p, err := authz.RequireTenant(ctx)
	if err != nil {
		return domain.Mission{}, err
	}
	m, err := s.repo.MissionByID(ctx, p.TenantID, missionID)
	if err != nil {
		return domain.Mission{}, err
	}
	if err := s.authz.Authorize(ctx, p, contracts.PermView, m); err != nil {
		return domain.Mission{}, err
	}
	return m, nil
}

func (s *Service) ListMissions(ctx context.Context, f MissionFilter, cursor string, limit int) (Page[domain.Mission], error) {
	p, err := authz.RequireTenant(ctx)
	if err != nil {
		return Page[domain.Mission]{}, err
	}
	if err := s.authz.Authorize(ctx, p, contracts.PermViewAny, nil); err != nil {
		return Page[domain.Mission]{}, err
	}
	after, limit, err := pageArgs(cursor, limit)
	if err != nil {
		return Page[domain.Mission]{}, err
	}
	rows, err := s.repo.ListMissions(ctx, p.TenantID, f, after, limit+1)
	if err != nil {
		return Page[domain.Mission]{}, err
	}
	return paginate(rows, limit, func(m domain.Mission) (time.Time, string) { return m.CreatedAt, m.ID }), nil
}

func (s *Service) UpdateMission(ctx context.Context, missionID string, cmd UpdateMissionCmd) (domain.Mission, error) {
	p, err := authz.RequireTenant(ctx)
	if err != nil {
		return domain.Mission{}, err
	}
	now := s.clock.Now()
	var out domain.Mission
	err = s.tx(ctx, func(tx *gorm.DB) error {
		m, err := s.repo.MissionInTx(ctx, tx, p.TenantID, missionID, true)
		if err != nil {
			return err
		}
		if err := s.authz.Authorize(ctx, p, contracts.PermUpdate, m); err != nil {
			return err
		}
		if err := applyUpdate(&m, cmd, now); err != nil {
			return err
		}
		if err := s.repo.SaveMission(ctx, tx, m); err != nil {
			return err
		}
		m.Version++
		out = m
		return s.outbox.Publish(ctx, tx, contracts.TopicMissionUpdated, changed(m, now))
	})
	if err != nil {
		return domain.Mission{}, err
	}
	return out, nil
}

func applyUpdate(m *domain.Mission, cmd UpdateMissionCmd, now time.Time) error {
	if cmd.Type != nil {
		if err := m.ChangeType(*cmd.Type); err != nil {
			return err
		}
	}
	if cmd.Status != nil {
		if err := m.TransitionTo(*cmd.Status, now); err != nil {
			return err
		}
	}
	if cmd.Slug != nil {
		m.Slug = *cmd.Slug
	}
	if cmd.Name != nil {
		m.Name = *cmd.Name
	}
	if cmd.Description != nil {
		m.Description = *cmd.Description
	}
	if cmd.Target != nil {
		m.Target = *cmd.Target
	}
	if cmd.Criteria != nil {
		if _, err := domain.ParseCriteria(cmd.Criteria); err != nil {
			return err
		}
		m.Criteria = cmd.Criteria
	}
	if cmd.PointsReward != nil {
		m.PointsReward = *cmd.PointsReward
	}
	if cmd.XPReward != nil {
		m.XPReward = *cmd.XPReward
	}
	if cmd.BadgeRewardID != nil {
		m.BadgeRewardID = *cmd.BadgeRewardID
	}
	if cmd.MaxCompletionsPerPlayer != nil {
		if *cmd.MaxCompletionsPerPlayer == 0 {
			m.MaxCompletionsPerPlayer = nil
		} else {
			v := *cmd.MaxCompletionsPerPlayer
			m.MaxCompletionsPerPlayer = &v
		}
	}
	switch {
	case cmd.ClearStartsAt:
		m.StartsAt = nil
	case cmd.StartsAt != nil:
		v := cmd.StartsAt.UTC()
		m.StartsAt = &v
	}
	switch {
	case cmd.ClearEndsAt:
		m.EndsAt = nil
	case cmd.EndsAt != nil:
		v := cmd.EndsAt.UTC()
		m.EndsAt = &v
	}
	m.UpdatedAt = now
	return m.Validate()
}

// DeleteMission soft-deletes. Attempts stay as history; open ones are
// expired by the sweep.
func (s *Service) DeleteMission(ctx context.Context, missionID string) error {
	p, err := authz.RequireTenant(ctx)
	if err != nil {
		return err
	}
	now := s.clock.Now()
	return s.tx(ctx, func(tx *gorm.DB) error {
		m, err := s.repo.MissionInTx(ctx, tx, p.TenantID, missionID, true)
		if err != nil {
			return err
		}
		if err := s.authz.Authorize(ctx, p, contracts.PermDelete, m); err != nil {
			return err
		}
		m.DeletedAt = &now
		m.UpdatedAt = now
		if err := s.repo.SaveMission(ctx, tx, m); err != nil {
			return err
		}
		return s.outbox.Publish(ctx, tx, contracts.TopicMissionDeleted, changed(m, now))
	})
}

// AttemptWithMission is an attempt plus its mission's summary.
type AttemptWithMission struct {
	Attempt    domain.Attempt
	Mission    domain.Mission
	HasMission bool
}

// ListMissionAttempts lists a mission's attempts, newest first.
func (s *Service) ListMissionAttempts(ctx context.Context, missionID string, f AttemptFilter, cursor string, limit int) (Page[domain.Attempt], error) {
	p, err := authz.RequireTenant(ctx)
	if err != nil {
		return Page[domain.Attempt]{}, err
	}
	m, err := s.repo.MissionByID(ctx, p.TenantID, missionID)
	if err != nil {
		return Page[domain.Attempt]{}, err
	}
	if err := s.authz.Authorize(ctx, p, contracts.PermViewAny, m); err != nil {
		return Page[domain.Attempt]{}, err
	}
	after, limit, err := pageArgs(cursor, limit)
	if err != nil {
		return Page[domain.Attempt]{}, err
	}
	f.MissionID = m.ID
	rows, err := s.repo.ListAttempts(ctx, p.TenantID, f, after, limit+1)
	if err != nil {
		return Page[domain.Attempt]{}, err
	}
	return paginate(rows, limit, attemptKey), nil
}

// ListPlayerAttempts lists a player's attempts with mission summaries.
func (s *Service) ListPlayerAttempts(ctx context.Context, playerID string, f AttemptFilter, cursor string, limit int) (Page[AttemptWithMission], error) {
	p, err := authz.RequireTenant(ctx)
	if err != nil {
		return Page[AttemptWithMission]{}, err
	}
	if err := s.authz.Authorize(ctx, p, contracts.PermViewAny, nil); err != nil {
		return Page[AttemptWithMission]{}, err
	}
	if _, err := s.requirePlayer(ctx, p.TenantID, playerID, false); err != nil {
		return Page[AttemptWithMission]{}, err
	}
	after, limit, err := pageArgs(cursor, limit)
	if err != nil {
		return Page[AttemptWithMission]{}, err
	}
	f.PlayerID = playerID
	rows, err := s.repo.ListAttempts(ctx, p.TenantID, f, after, limit+1)
	if err != nil {
		return Page[AttemptWithMission]{}, err
	}
	page := paginate(rows, limit, attemptKey)
	ids := make([]string, 0, len(page.Items))
	seen := map[string]bool{}
	for _, a := range page.Items {
		if !seen[a.MissionID] {
			seen[a.MissionID] = true
			ids = append(ids, a.MissionID)
		}
	}
	missions, err := s.repo.MissionsByIDs(ctx, p.TenantID, ids)
	if err != nil {
		return Page[AttemptWithMission]{}, err
	}
	out := Page[AttemptWithMission]{NextCursor: page.NextCursor, Items: make([]AttemptWithMission, len(page.Items))}
	for i, a := range page.Items {
		m, ok := missions[a.MissionID]
		out.Items[i] = AttemptWithMission{Attempt: a, Mission: m, HasMission: ok}
	}
	return out, nil
}

// CompletedCounts implements contracts.Reader: completed attempts per
// player. Players with none are absent.
func (s *Service) CompletedCounts(ctx context.Context, tenantID string, playerIDs []string) (map[string]int, error) {
	if tenantID == "" {
		return nil, errs.New(errs.Invalid, "tenant id required")
	}
	if len(playerIDs) == 0 {
		return map[string]int{}, nil
	}
	return s.repo.CompletedCounts(ctx, tenantID, playerIDs)
}

var _ contracts.Reader = (*Service)(nil)

// requirePlayer validates a player of the tenant: 404 player_not_found when
// absent; 409 player_inactive when inactive and active is required.
func (s *Service) requirePlayer(ctx context.Context, tenantID, playerID string, active bool) (ports.PlayerSnapshot, error) {
	got, err := s.players.PlayersByIDs(ctx, tenantID, []string{playerID})
	if err != nil {
		return ports.PlayerSnapshot{}, err
	}
	snap, ok := got[playerID]
	if !ok {
		return ports.PlayerSnapshot{}, domain.ErrPlayerNotFound
	}
	if active && !snap.Active {
		return ports.PlayerSnapshot{}, domain.ErrPlayerInactive
	}
	return snap, nil
}

func changed(m domain.Mission, at time.Time) contracts.MissionChangedV1 {
	return contracts.MissionChangedV1{MissionID: m.ID, TenantID: m.TenantID, Slug: m.Slug, Status: m.Status, At: at}
}

func attemptKey(a domain.Attempt) (time.Time, string) { return a.CreatedAt, a.ID }

func pageArgs(cursor string, limit int) (Cursor, int, error) {
	if limit <= 0 {
		limit = defaultPageSize
	}
	if limit > maxPageSize {
		limit = maxPageSize
	}
	if cursor == "" {
		return Cursor{}, limit, nil
	}
	ts, id, err := pagination.DecodeCursor(cursor)
	if err != nil {
		return Cursor{}, 0, err
	}
	if !isUUID(id) {
		return Cursor{}, 0, errs.New(errs.Invalid, "malformed cursor")
	}
	return Cursor{CreatedAt: ts, ID: id}, limit, nil
}

// paginate trims the limit+1 probe row and derives the next cursor.
func paginate[T any](rows []T, limit int, key func(T) (time.Time, string)) Page[T] {
	if len(rows) <= limit {
		return Page[T]{Items: rows}
	}
	rows = rows[:limit]
	ts, id := key(rows[len(rows)-1])
	return Page[T]{Items: rows, NextCursor: pagination.EncodeCursor(ts, id)}
}
