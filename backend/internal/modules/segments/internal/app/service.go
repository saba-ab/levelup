// Package app holds segments' use cases: definitions, previews and the
// refresh runs that materialize membership. Every outbox publish is here.
package app

import (
	"context"
	"time"

	"gorm.io/gorm"

	"levelup/internal/modules/segments/contracts"
	"levelup/internal/modules/segments/internal/domain"
	"levelup/internal/modules/segments/internal/ports"
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
	previewSample   = 10
)

// Page is a keyset position: rows strictly older than (Before, BeforeID).
type Page struct {
	Before   time.Time
	BeforeID string
	Limit    int
}

// Member is one materialized membership row.
type Member struct {
	SegmentID string
	PlayerID  string
	AddedAt   time.Time
}

// SegmentRef names a live segment.
type SegmentRef struct {
	TenantID string
	ID       string
}

// PageResult is what applying one evaluated page changed.
type PageResult struct {
	Added   []string
	Removed []string
}

// Repository is implemented by internal/repo. Write methods take the tx.
type Repository interface {
	CreateSegment(ctx context.Context, tx *gorm.DB, s domain.Segment) error
	SaveSegment(ctx context.Context, tx *gorm.DB, s domain.Segment) error
	SoftDeleteSegment(ctx context.Context, tx *gorm.DB, tenantID, id string, at time.Time) error
	SegmentByID(ctx context.Context, tenantID, id string) (domain.Segment, error)
	ListSegments(ctx context.Context, tenantID string, p Page) ([]domain.Segment, error)
	LiveSegmentsAfter(ctx context.Context, afterID string, limit int) ([]SegmentRef, error)

	// AcquireRefresh takes the segment's refresh lease for runID when no
	// unexpired lease is held; false when another run holds it.
	AcquireRefresh(ctx context.Context, tenantID, segmentID, runID string, now, until time.Time) (domain.Segment, bool, error)
	// ExtendLease renews runID's lease; false when the run lost it.
	ExtendLease(ctx context.Context, tx *gorm.DB, segmentID, runID string, until time.Time) (bool, error)
	// ApplyPage makes matched members (stamped with runID) and removes the
	// page's other players from the segment.
	ApplyPage(ctx context.Context, tx *gorm.DB, tenantID, segmentID, runID string, pageIDs, matched []string, now time.Time) (PageResult, error)
	// SweepStale removes up to limit members not stamped by runID: players
	// the scan no longer lists.
	SweepStale(ctx context.Context, tx *gorm.DB, segmentID, runID string, limit int) ([]string, error)
	FinishRefresh(ctx context.Context, segmentID, runID string, now time.Time) error

	ListMembers(ctx context.Context, tenantID, segmentID string, p Page) ([]Member, error)
	RemovePlayer(ctx context.Context, tx *gorm.DB, tenantID, playerID string) ([]string, error)
	SegmentsOfPlayers(ctx context.Context, tenantID string, playerIDs []string) (map[string][]string, error)

	MarkRun(ctx context.Context, job string, at time.Time) error
	PurgeTenant(ctx context.Context, tx *gorm.DB, tenantID string) error
}

// Readers groups the ports a refresh reads.
type Readers struct {
	Players  ports.PlayerReader
	Progress ports.ProgressReader
	Wallets  ports.WalletReader
	Badges   ports.BadgeReader
	Activity ports.ActivityReader
}

// Settings are the refresh knobs from Config.
type Settings struct {
	PageSize      int
	Lease         time.Duration
	PreviewLimit  int
	MaxRefreshRun int // re-runs when the definition changed mid-run
}

type Service struct {
	repo   Repository
	rd     Readers
	outbox outbox.Store
	authz  authz.Enforcer
	clock  clock.Clock
	set    Settings

	tx func(ctx context.Context, fn func(tx *gorm.DB) error) error
}

func NewService(repo Repository, rd Readers, ob outbox.Store, enf authz.Enforcer, db *gorm.DB, c clock.Clock, set Settings) *Service {
	if set.PageSize <= 0 {
		set.PageSize = 500
	}
	if set.Lease <= 0 {
		set.Lease = 15 * time.Minute
	}
	if set.PreviewLimit <= 0 {
		set.PreviewLimit = 1000
	}
	if set.MaxRefreshRun <= 0 {
		set.MaxRefreshRun = 3
	}
	s := &Service{repo: repo, rd: rd, outbox: ob, authz: enf, clock: c, set: set}
	s.tx = func(ctx context.Context, fn func(tx *gorm.DB) error) error {
		return postgres.InTx(ctx, db, fn)
	}
	return s
}

func (s *Service) guard(ctx context.Context, perm authz.Permission, resource any) (authz.Principal, error) {
	p, err := authz.RequireTenant(ctx)
	if err != nil {
		return authz.Principal{}, err
	}
	if err := s.authz.Authorize(ctx, p, perm, resource); err != nil {
		return authz.Principal{}, err
	}
	return p, nil
}

// load resolves the tenant, the segment and the permission on it.
func (s *Service) load(ctx context.Context, perm authz.Permission, id string) (authz.Principal, domain.Segment, error) {
	p, err := authz.RequireTenant(ctx)
	if err != nil {
		return authz.Principal{}, domain.Segment{}, err
	}
	seg, err := s.repo.SegmentByID(ctx, p.TenantID, id)
	if err != nil {
		return authz.Principal{}, domain.Segment{}, err
	}
	if err := s.authz.Authorize(ctx, p, perm, seg); err != nil {
		return authz.Principal{}, domain.Segment{}, err
	}
	return p, seg, nil
}

func (s *Service) requestRefresh(ctx context.Context, tx *gorm.DB, seg domain.Segment) error {
	return s.outbox.Publish(ctx, tx, contracts.Topic(contracts.JobRefreshSegment), contracts.RefreshSegmentCmdV1{
		TenantID: seg.TenantID, SegmentID: seg.ID, RequestedAt: s.clock.Now(),
	})
}

// Create stores the definition and queues its first refresh.
func (s *Service) Create(ctx context.Context, in domain.NewSegmentInput) (domain.Segment, error) {
	p, err := s.guard(ctx, contracts.PermManage, nil)
	if err != nil {
		return domain.Segment{}, err
	}
	seg, err := domain.NewSegment(p.TenantID, p.UserID, in, s.clock.Now())
	if err != nil {
		return domain.Segment{}, err
	}
	if err := s.tx(ctx, func(tx *gorm.DB) error {
		if err := s.repo.CreateSegment(ctx, tx, seg); err != nil {
			return err
		}
		return s.requestRefresh(ctx, tx, seg)
	}); err != nil {
		return domain.Segment{}, err
	}
	return seg, nil
}

func (s *Service) Get(ctx context.Context, id string) (domain.Segment, error) {
	_, seg, err := s.load(ctx, contracts.PermView, id)
	return seg, err
}

// List pages newest-first by (created_at, id).
func (s *Service) List(ctx context.Context, cursor string, limit int) ([]domain.Segment, string, error) {
	p, err := s.guard(ctx, contracts.PermView, nil)
	if err != nil {
		return nil, "", err
	}
	page, err := pageOf(cursor, limit)
	if err != nil {
		return nil, "", err
	}
	page.Limit++
	rows, err := s.repo.ListSegments(ctx, p.TenantID, page)
	if err != nil {
		return nil, "", err
	}
	next := ""
	if len(rows) == page.Limit {
		rows = rows[:page.Limit-1]
		last := rows[len(rows)-1]
		next = pagination.EncodeCursor(last.CreatedAt, last.ID)
	}
	return rows, next, nil
}

// Update changes the definition; a conditions change queues a refresh.
func (s *Service) Update(ctx context.Context, id string, patch domain.SegmentPatch) (domain.Segment, error) {
	_, seg, err := s.load(ctx, contracts.PermManage, id)
	if err != nil {
		return domain.Segment{}, err
	}
	changed, err := seg.Apply(patch, s.clock.Now())
	if err != nil {
		return domain.Segment{}, err
	}
	if err := s.tx(ctx, func(tx *gorm.DB) error {
		if err := s.repo.SaveSegment(ctx, tx, seg); err != nil {
			return err
		}
		if !changed {
			return nil
		}
		return s.requestRefresh(ctx, tx, seg)
	}); err != nil {
		return domain.Segment{}, err
	}
	seg.Version++
	return seg, nil
}

// Delete soft-deletes the segment, drops its membership and publishes
// segments.deleted.v1 (not one membership_changed.v1 per member).
func (s *Service) Delete(ctx context.Context, id string) error {
	p, seg, err := s.load(ctx, contracts.PermManage, id)
	if err != nil {
		return err
	}
	now := s.clock.Now()
	return s.tx(ctx, func(tx *gorm.DB) error {
		if err := s.repo.SoftDeleteSegment(ctx, tx, p.TenantID, seg.ID, now); err != nil {
			return err
		}
		return s.outbox.Publish(ctx, tx, contracts.TopicSegmentDeleted, contracts.SegmentDeletedV1{
			TenantID: p.TenantID, SegmentID: seg.ID, At: now,
		})
	})
}

// RequestRefresh queues a recomputation (202). Requests while a run holds
// the segment coalesce into that run.
func (s *Service) RequestRefresh(ctx context.Context, id string) (domain.Segment, error) {
	_, seg, err := s.load(ctx, contracts.PermManage, id)
	if err != nil {
		return domain.Segment{}, err
	}
	if err := s.tx(ctx, func(tx *gorm.DB) error { return s.requestRefresh(ctx, tx, seg) }); err != nil {
		return domain.Segment{}, err
	}
	return seg, nil
}

// MemberView is a member with the player's display data (zero when the
// player has gone since the last refresh).
type MemberView struct {
	Member Member
	Player ports.PlayerSnapshot
}

// Members pages a segment's materialized members, newest first.
func (s *Service) Members(ctx context.Context, id, cursor string, limit int) ([]MemberView, string, error) {
	p, seg, err := s.load(ctx, contracts.PermView, id)
	if err != nil {
		return nil, "", err
	}
	page, err := pageOf(cursor, limit)
	if err != nil {
		return nil, "", err
	}
	page.Limit++
	rows, err := s.repo.ListMembers(ctx, p.TenantID, seg.ID, page)
	if err != nil {
		return nil, "", err
	}
	next := ""
	if len(rows) == page.Limit {
		rows = rows[:page.Limit-1]
		last := rows[len(rows)-1]
		next = pagination.EncodeCursor(last.AddedAt, last.PlayerID)
	}
	ids := make([]string, len(rows))
	for i, m := range rows {
		ids[i] = m.PlayerID
	}
	players, err := s.rd.Players.PlayersByIDs(ctx, p.TenantID, ids)
	if err != nil {
		return nil, "", err
	}
	out := make([]MemberView, len(rows))
	for i, m := range rows {
		out[i] = MemberView{Member: m, Player: players[m.PlayerID]}
	}
	return out, next, nil
}

// Preview is the result of evaluating conditions over the first players.
type Preview struct {
	Matched  int
	Scanned  int
	Complete bool // every player of the tenant was scanned
	Sample   []ports.PlayerSnapshot
}

// Preview evaluates unsaved conditions over the tenant's first
// PreviewLimit players (ascending id), without touching any membership.
func (s *Service) Preview(ctx context.Context, conditions map[string]any) (Preview, error) {
	p, err := s.guard(ctx, contracts.PermView, nil)
	if err != nil {
		return Preview{}, err
	}
	g, err := domain.ParseConditions(conditions)
	if err != nil {
		return Preview{}, err
	}
	out := Preview{Sample: []ports.PlayerSnapshot{}}
	after := ""
	for out.Scanned < s.set.PreviewLimit {
		limit := min(s.set.PageSize, s.set.PreviewLimit-out.Scanned)
		ids, err := s.rd.Players.ListPlayerIDs(ctx, p.TenantID, after, limit)
		if err != nil {
			return Preview{}, err
		}
		players, matched, err := s.evaluate(ctx, p.TenantID, g, ids)
		if err != nil {
			return Preview{}, err
		}
		out.Scanned += len(ids)
		out.Matched += len(matched)
		for _, id := range matched {
			if len(out.Sample) < previewSample {
				out.Sample = append(out.Sample, players[id])
			}
		}
		if len(ids) < limit {
			out.Complete = true
			break
		}
		after = ids[len(ids)-1]
	}
	return out, nil
}

// SegmentsOfPlayers backs contracts.Reader.
func (s *Service) SegmentsOfPlayers(ctx context.Context, tenantID string, playerIDs []string) (map[string][]string, error) {
	if len(playerIDs) == 0 {
		return map[string][]string{}, nil
	}
	return s.repo.SegmentsOfPlayers(ctx, tenantID, playerIDs)
}

// PlayerDeleted handles player.deleted.v1: the player leaves every segment,
// one membership_changed.v1 (removed) each. Idempotent: a redelivery finds
// no rows and publishes nothing.
func (s *Service) PlayerDeleted(ctx context.Context, tenantID, playerID string) error {
	if tenantID == "" || playerID == "" {
		return errs.New(errs.Invalid, "player.deleted.v1 without tenant_id or player_id")
	}
	now := s.clock.Now()
	return s.tx(ctx, func(tx *gorm.DB) error {
		segs, err := s.repo.RemovePlayer(ctx, tx, tenantID, playerID)
		if err != nil {
			return err
		}
		for _, segID := range segs {
			if err := s.publishChange(ctx, tx, tenantID, segID, playerID, contracts.ChangeRemoved, "", now); err != nil {
				return err
			}
		}
		return nil
	})
}

// PurgeTenant handles tenant.deleted.v1. Idempotent.
func (s *Service) PurgeTenant(ctx context.Context, tenantID string) error {
	if tenantID == "" {
		return errs.New(errs.Invalid, "tenant.deleted.v1 without tenant_id")
	}
	return s.tx(ctx, func(tx *gorm.DB) error {
		return s.repo.PurgeTenant(ctx, tx, tenantID)
	})
}

func (s *Service) publishChange(ctx context.Context, tx *gorm.DB, tenantID, segmentID, playerID, change, runID string, at time.Time) error {
	return s.outbox.Publish(ctx, tx, contracts.TopicMembershipChanged, contracts.MembershipChangedV1{
		TenantID: tenantID, SegmentID: segmentID, PlayerID: playerID, Change: change, At: at, RunID: runID,
	})
}

func pageOf(cursor string, limit int) (Page, error) {
	switch {
	case limit <= 0:
		limit = defaultPageSize
	case limit > maxPageSize:
		limit = maxPageSize
	}
	p := Page{Limit: limit}
	if cursor != "" {
		before, beforeID, err := pagination.DecodeCursor(cursor)
		if err != nil {
			return Page{}, err
		}
		p.Before, p.BeforeID = before, beforeID
	}
	return p, nil
}
