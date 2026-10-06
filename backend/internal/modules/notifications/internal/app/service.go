// Package app holds notifications' use cases: transaction boundaries, tenant
// and permission checks, every outbox publish, subscription and job handlers.
package app

import (
	"context"
	"time"

	"go.uber.org/zap"
	"gorm.io/gorm"

	"levelup/internal/modules/notifications/internal/domain"
	"levelup/internal/modules/notifications/internal/ports"
	"levelup/internal/platform/authz"
	"levelup/internal/platform/clock"
	"levelup/internal/platform/mail"
	"levelup/internal/platform/outbox"
	"levelup/internal/platform/postgres"
)

// DefaultPageSize and MaxPageSize bound every list (ADR-0016).
const (
	DefaultPageSize = 25
	MaxPageSize     = 100
)

func clampLimit(limit int) int {
	switch {
	case limit <= 0:
		return DefaultPageSize
	case limit > MaxPageSize:
		return MaxPageSize
	default:
		return limit
	}
}

// PageCursor is a decoded (created_at, id) keyset position.
type PageCursor struct {
	Before   time.Time
	BeforeID string
}

// TemplateFilter narrows a template listing.
type TemplateFilter struct {
	Trigger string
	Active  *bool
	Cursor  PageCursor
	Limit   int
}

// HistoryFilter narrows the notification history.
type HistoryFilter struct {
	TemplateID string
	Status     string
	Channel    string
	PlayerID   string
	Cursor     PageCursor
	Limit      int
}

// FeedFilter narrows a player's in-app feed.
type FeedFilter struct {
	UnreadOnly bool
	Cursor     PageCursor
	Limit      int
}

// Repository is implemented by internal/repo. Write methods take the tx
// right after ctx; reads use the repository's own handle.
type Repository interface {
	CreateTemplate(ctx context.Context, tx *gorm.DB, t domain.Template) error
	// TemplateByID returns a live template of the tenant, else ErrTemplateNotFound.
	TemplateByID(ctx context.Context, tenantID, id string) (domain.Template, error)
	TemplateByIDForUpdate(ctx context.Context, tx *gorm.DB, tenantID, id string) (domain.Template, error)
	// SaveTemplate writes back guarded by version; ErrVersionConflict otherwise.
	SaveTemplate(ctx context.Context, tx *gorm.DB, t domain.Template) error
	ListTemplates(ctx context.Context, tenantID string, f TemplateFilter) ([]domain.Template, error)
	ActiveTemplatesByTrigger(ctx context.Context, tenantID, trigger string) ([]domain.Template, error)
	// TemplatesByIDs includes soft-deleted templates (history and stats name them).
	TemplatesByIDs(ctx context.Context, tenantID string, ids []string) ([]domain.Template, error)

	// ChannelSettings returns found=false when the tenant never saved any.
	ChannelSettings(ctx context.Context, tenantID string) (domain.ChannelSettings, bool, error)
	UpsertChannelSettings(ctx context.Context, tx *gorm.DB, s domain.ChannelSettings) error

	// InsertNotification is INSERT ... ON CONFLICT (template_id, event_id,
	// channel) DO NOTHING; inserted=false means a redelivery.
	InsertNotification(ctx context.Context, tx *gorm.DB, n domain.Notification) (bool, error)
	NotificationByID(ctx context.Context, tenantID, id string) (domain.Notification, bool, error)
	// ClaimEmail takes the send lease on a pending row: attempts+1 and
	// lease_until set, only when the row is pending and not leased. claimed
	// is false when it is settled or another worker holds the lease.
	ClaimEmail(ctx context.Context, tx *gorm.DB, tenantID, id string, now, leaseUntil time.Time) (domain.Notification, bool, error)
	// SettleNotification writes status/reason/errors back while the row is
	// still pending; settled=false means someone else settled it first.
	SettleNotification(ctx context.Context, tx *gorm.DB, n domain.Notification) (bool, error)
	ListHistory(ctx context.Context, tenantID string, f HistoryFilter) ([]domain.Notification, error)
	ListFeed(ctx context.Context, tenantID, playerID string, f FeedFilter) ([]domain.Notification, error)
	CountUnread(ctx context.Context, tenantID, playerID string) (int64, error)
	// MarkRead sets read_at once on an in_app row of the player; found=false
	// when there is no such row.
	MarkRead(ctx context.Context, tx *gorm.DB, tenantID, playerID, id string, now time.Time) (domain.Notification, bool, error)
	MarkAllRead(ctx context.Context, tx *gorm.DB, tenantID, playerID string, now time.Time) (int64, error)
	Stats(ctx context.Context, tenantID string, from, to time.Time) ([]domain.StatsRow, error)

	// FailStalePending fails pending email rows created before cutoff.
	FailStalePending(ctx context.Context, tx *gorm.DB, cutoff, now time.Time) (int64, error)
	PurgeTenant(ctx context.Context, tx *gorm.DB, tenantID string) error
	PurgePlayer(ctx context.Context, tx *gorm.DB, tenantID, playerID string) error
}

// Options are the tunables from Config.
type Options struct {
	RenderTimeout    time.Duration
	SendTimeout      time.Duration
	EmailMaxAttempts int
	EmailStaleAfter  time.Duration
}

func (o Options) withDefaults() Options {
	if o.RenderTimeout <= 0 {
		o.RenderTimeout = 250 * time.Millisecond
	}
	if o.SendTimeout <= 0 {
		o.SendTimeout = 30 * time.Second
	}
	if o.EmailMaxAttempts <= 0 {
		o.EmailMaxAttempts = 4
	}
	if o.EmailStaleAfter <= 0 {
		o.EmailStaleAfter = 2 * time.Hour
	}
	return o
}

// Service implements every notifications use case.
type Service struct {
	repo    Repository
	players ports.PlayerReader
	mailer  mail.Mailer
	outbox  outbox.Store
	authz   authz.Enforcer
	db      *gorm.DB
	clock   clock.Clock
	log     *zap.Logger
	opts    Options

	tx func(ctx context.Context, fn func(tx *gorm.DB) error) error
}

// NewService wires the service. log may be nil.
func NewService(repo Repository, players ports.PlayerReader, mailer mail.Mailer, ob outbox.Store, enf authz.Enforcer,
	db *gorm.DB, c clock.Clock, log *zap.Logger, opts Options) *Service {
	if log == nil {
		log = zap.NewNop()
	}
	s := &Service{repo: repo, players: players, mailer: mailer, outbox: ob, authz: enf, db: db, clock: c, log: log,
		opts: opts.withDefaults()}
	s.tx = func(ctx context.Context, fn func(tx *gorm.DB) error) error {
		return postgres.InTx(ctx, s.db, fn)
	}
	return s
}

func (s *Service) now() time.Time { return s.clock.Now().UTC().Truncate(time.Microsecond) }

// requireTenantPerm is the opening of every HTTP-facing method (ADR-0015).
func (s *Service) requireTenantPerm(ctx context.Context, perm authz.Permission) (authz.Principal, error) {
	p, err := authz.RequireTenant(ctx)
	if err != nil {
		return authz.Principal{}, err
	}
	if err := s.authz.Authorize(ctx, p, perm, nil); err != nil {
		return authz.Principal{}, err
	}
	return p, nil
}

// requirePlayer resolves a player of the caller's tenant for HTTP paths:
// unknown → 404 player_not_found.
func (s *Service) requirePlayer(ctx context.Context, tenantID, playerID string) (ports.PlayerSnapshot, error) {
	snap, ok, err := s.players.ByID(ctx, tenantID, playerID)
	if err != nil {
		return ports.PlayerSnapshot{}, err
	}
	if !ok || snap.TenantID != tenantID {
		return ports.PlayerSnapshot{}, domain.ErrPlayerNotFound
	}
	return snap, nil
}

// channelSettings returns the tenant's settings or the defaults.
func (s *Service) channelSettings(ctx context.Context, tenantID string) (domain.ChannelSettings, error) {
	cs, found, err := s.repo.ChannelSettings(ctx, tenantID)
	if err != nil {
		return domain.ChannelSettings{}, err
	}
	if !found {
		return domain.DefaultChannelSettings(tenantID), nil
	}
	return cs, nil
}
