package app

import (
	"context"
	"sort"
	"sync"
	"time"

	"gorm.io/gorm"

	"levelup/internal/modules/notifications/contracts"
	"levelup/internal/modules/notifications/internal/domain"
	"levelup/internal/modules/notifications/internal/ports"
	"levelup/internal/platform/authz"
	"levelup/internal/platform/mail"
	"levelup/internal/shared/errs"
)

// fakeRepo is an in-memory Repository. It is not transactional; the service
// tests use a pass-through tx runner.
type fakeRepo struct {
	mu            sync.Mutex
	templates     map[string]domain.Template
	settings      map[string]domain.ChannelSettings
	notifications map[string]domain.Notification
}

func newFakeRepo() *fakeRepo {
	return &fakeRepo{
		templates:     map[string]domain.Template{},
		settings:      map[string]domain.ChannelSettings{},
		notifications: map[string]domain.Notification{},
	}
}

func (f *fakeRepo) CreateTemplate(_ context.Context, _ *gorm.DB, t domain.Template) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	for _, o := range f.templates {
		if o.TenantID == t.TenantID && o.Name == t.Name && !o.Deleted() {
			return domain.ErrTemplateNameTaken
		}
	}
	f.templates[t.ID] = t
	return nil
}

func (f *fakeRepo) live(tenantID, id string) (domain.Template, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	t, ok := f.templates[id]
	if !ok || t.TenantID != tenantID || t.Deleted() {
		return domain.Template{}, domain.ErrTemplateNotFound
	}
	return t, nil
}

func (f *fakeRepo) TemplateByID(_ context.Context, tenantID, id string) (domain.Template, error) {
	return f.live(tenantID, id)
}

func (f *fakeRepo) TemplateByIDForUpdate(_ context.Context, _ *gorm.DB, tenantID, id string) (domain.Template, error) {
	return f.live(tenantID, id)
}

func (f *fakeRepo) SaveTemplate(_ context.Context, _ *gorm.DB, t domain.Template) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	cur, ok := f.templates[t.ID]
	if !ok || cur.Version != t.Version {
		return domain.ErrVersionConflict
	}
	t.Version++
	f.templates[t.ID] = t
	return nil
}

func sortDesc[T any](rows []T, at func(T) time.Time, id func(T) string) {
	sort.Slice(rows, func(i, j int) bool {
		if !at(rows[i]).Equal(at(rows[j])) {
			return at(rows[i]).After(at(rows[j]))
		}
		return id(rows[i]) > id(rows[j])
	})
}

func before(at time.Time, id string, c PageCursor) bool {
	if c.Before.IsZero() {
		return true
	}
	if at.Equal(c.Before) {
		return id < c.BeforeID
	}
	return at.Before(c.Before)
}

func (f *fakeRepo) ListTemplates(_ context.Context, tenantID string, flt TemplateFilter) ([]domain.Template, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	var out []domain.Template
	for _, t := range f.templates {
		if t.TenantID != tenantID || t.Deleted() || (flt.Trigger != "" && t.Trigger != flt.Trigger) ||
			(flt.Active != nil && t.Active != *flt.Active) || !before(t.CreatedAt, t.ID, flt.Cursor) {
			continue
		}
		out = append(out, t)
	}
	sortDesc(out, func(t domain.Template) time.Time { return t.CreatedAt }, func(t domain.Template) string { return t.ID })
	if len(out) > flt.Limit {
		out = out[:flt.Limit]
	}
	return out, nil
}

func (f *fakeRepo) ActiveTemplatesByTrigger(_ context.Context, tenantID, trigger string) ([]domain.Template, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	var out []domain.Template
	for _, t := range f.templates {
		if t.TenantID == tenantID && t.Trigger == trigger && t.Active && !t.Deleted() {
			out = append(out, t)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	return out, nil
}

func (f *fakeRepo) TemplatesByIDs(_ context.Context, tenantID string, ids []string) ([]domain.Template, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	var out []domain.Template
	for _, id := range ids {
		if t, ok := f.templates[id]; ok && t.TenantID == tenantID {
			out = append(out, t)
		}
	}
	return out, nil
}

func (f *fakeRepo) ChannelSettings(_ context.Context, tenantID string) (domain.ChannelSettings, bool, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	s, ok := f.settings[tenantID]
	return s, ok, nil
}

func (f *fakeRepo) UpsertChannelSettings(_ context.Context, _ *gorm.DB, s domain.ChannelSettings) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.settings[s.TenantID] = s
	return nil
}

func (f *fakeRepo) InsertNotification(_ context.Context, _ *gorm.DB, n domain.Notification) (bool, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	for _, o := range f.notifications {
		if o.TemplateID == n.TemplateID && o.EventID == n.EventID && o.Channel == n.Channel {
			return false, nil
		}
	}
	f.notifications[n.ID] = n
	return true, nil
}

func (f *fakeRepo) NotificationByID(_ context.Context, tenantID, id string) (domain.Notification, bool, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	n, ok := f.notifications[id]
	if !ok || n.TenantID != tenantID {
		return domain.Notification{}, false, nil
	}
	return n, true, nil
}

func (f *fakeRepo) ClaimEmail(_ context.Context, _ *gorm.DB, tenantID, id string, now, leaseUntil time.Time) (domain.Notification, bool, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	n, ok := f.notifications[id]
	if !ok || n.TenantID != tenantID || n.Status != contracts.StatusPending || n.Channel != contracts.ChannelEmail ||
		(n.LeaseUntil != nil && !n.LeaseUntil.Before(now)) {
		return domain.Notification{}, false, nil
	}
	n.Attempts++
	n.LeaseUntil = &leaseUntil
	n.UpdatedAt = now
	f.notifications[id] = n
	return n, true, nil
}

func (f *fakeRepo) SettleNotification(_ context.Context, _ *gorm.DB, n domain.Notification) (bool, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	cur, ok := f.notifications[n.ID]
	if !ok || cur.TenantID != n.TenantID || cur.Status != contracts.StatusPending {
		return false, nil
	}
	cur.Status, cur.Reason, cur.LastError, cur.LeaseUntil, cur.DeliveredAt, cur.UpdatedAt =
		n.Status, n.Reason, n.LastError, n.LeaseUntil, n.DeliveredAt, n.UpdatedAt
	f.notifications[n.ID] = cur
	return true, nil
}

func (f *fakeRepo) filterNotifications(keep func(domain.Notification) bool, limit int) []domain.Notification {
	var out []domain.Notification
	for _, n := range f.notifications {
		if keep(n) {
			out = append(out, n)
		}
	}
	sortDesc(out, func(n domain.Notification) time.Time { return n.CreatedAt }, func(n domain.Notification) string { return n.ID })
	if limit > 0 && len(out) > limit {
		out = out[:limit]
	}
	return out
}

func (f *fakeRepo) ListHistory(_ context.Context, tenantID string, flt HistoryFilter) ([]domain.Notification, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.filterNotifications(func(n domain.Notification) bool {
		return n.TenantID == tenantID && (flt.TemplateID == "" || n.TemplateID == flt.TemplateID) &&
			(flt.Status == "" || n.Status == flt.Status) && (flt.Channel == "" || n.Channel == flt.Channel) &&
			(flt.PlayerID == "" || n.PlayerID == flt.PlayerID) && before(n.CreatedAt, n.ID, flt.Cursor)
	}, flt.Limit), nil
}

func inFeed(n domain.Notification, tenantID, playerID string) bool {
	return n.TenantID == tenantID && n.PlayerID == playerID && n.Channel == contracts.ChannelInApp &&
		n.Status == contracts.StatusDelivered
}

func (f *fakeRepo) ListFeed(_ context.Context, tenantID, playerID string, flt FeedFilter) ([]domain.Notification, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.filterNotifications(func(n domain.Notification) bool {
		return inFeed(n, tenantID, playerID) && (!flt.UnreadOnly || n.ReadAt == nil) && before(n.CreatedAt, n.ID, flt.Cursor)
	}, flt.Limit), nil
}

func (f *fakeRepo) CountUnread(_ context.Context, tenantID, playerID string) (int64, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return int64(len(f.filterNotifications(func(n domain.Notification) bool {
		return inFeed(n, tenantID, playerID) && n.ReadAt == nil
	}, 0))), nil
}

func (f *fakeRepo) MarkRead(_ context.Context, _ *gorm.DB, tenantID, playerID, id string, now time.Time) (domain.Notification, bool, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	n, ok := f.notifications[id]
	if !ok || !inFeed(n, tenantID, playerID) {
		return domain.Notification{}, false, nil
	}
	if n.ReadAt == nil {
		n.ReadAt = &now
	}
	f.notifications[id] = n
	return n, true, nil
}

func (f *fakeRepo) MarkAllRead(_ context.Context, _ *gorm.DB, tenantID, playerID string, now time.Time) (int64, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	var count int64
	for id, n := range f.notifications {
		if inFeed(n, tenantID, playerID) && n.ReadAt == nil {
			n.ReadAt = &now
			f.notifications[id] = n
			count++
		}
	}
	return count, nil
}

func (f *fakeRepo) Stats(_ context.Context, tenantID string, from, to time.Time) ([]domain.StatsRow, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	agg := map[[3]string]*domain.StatsRow{}
	for _, n := range f.notifications {
		if n.TenantID != tenantID || (!from.IsZero() && n.CreatedAt.Before(from)) || (!to.IsZero() && !n.CreatedAt.Before(to)) {
			continue
		}
		k := [3]string{n.TemplateID, n.Channel, n.Status}
		if agg[k] == nil {
			agg[k] = &domain.StatsRow{TemplateID: n.TemplateID, Channel: n.Channel, Status: n.Status}
		}
		agg[k].Count++
		if n.ReadAt != nil {
			agg[k].Read++
		}
	}
	var out []domain.StatsRow
	for _, r := range agg {
		out = append(out, *r)
	}
	return out, nil
}

func (f *fakeRepo) FailStalePending(_ context.Context, _ *gorm.DB, cutoff, now time.Time) (int64, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	var count int64
	for id, n := range f.notifications {
		if n.Status == contracts.StatusPending && n.CreatedAt.Before(cutoff) {
			n.Fail(contracts.ReasonStale, n.LastError, now)
			f.notifications[id] = n
			count++
		}
	}
	return count, nil
}

func (f *fakeRepo) PurgeTenant(_ context.Context, _ *gorm.DB, tenantID string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	for id, n := range f.notifications {
		if n.TenantID == tenantID {
			delete(f.notifications, id)
		}
	}
	for id, t := range f.templates {
		if t.TenantID == tenantID {
			delete(f.templates, id)
		}
	}
	delete(f.settings, tenantID)
	return nil
}

func (f *fakeRepo) PurgePlayer(_ context.Context, _ *gorm.DB, tenantID, playerID string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	for id, n := range f.notifications {
		if n.TenantID == tenantID && n.PlayerID == playerID {
			delete(f.notifications, id)
		}
	}
	return nil
}

func (f *fakeRepo) all() []domain.Notification {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.filterNotifications(func(domain.Notification) bool { return true }, 0)
}

// fakeOutbox records topics and payloads.
type recorded struct {
	topic   string
	payload any
}

type fakeOutbox struct {
	mu        sync.Mutex
	published []recorded
}

func (f *fakeOutbox) Publish(_ context.Context, _ *gorm.DB, topic string, payload any) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.published = append(f.published, recorded{topic, payload})
	return nil
}

func (f *fakeOutbox) byTopic(topic string) []any {
	f.mu.Lock()
	defer f.mu.Unlock()
	var out []any
	for _, r := range f.published {
		if r.topic == topic {
			out = append(out, r.payload)
		}
	}
	return out
}

// fakePlayers is the PlayerReader port.
type fakePlayers struct {
	players map[string]ports.PlayerSnapshot
	err     error
}

func (f *fakePlayers) ByID(ctx context.Context, tenantID, playerID string) (ports.PlayerSnapshot, bool, error) {
	got, err := f.ByIDs(ctx, tenantID, []string{playerID})
	if err != nil {
		return ports.PlayerSnapshot{}, false, err
	}
	snap, ok := got[playerID]
	return snap, ok, nil
}

func (f *fakePlayers) ByIDs(_ context.Context, tenantID string, ids []string) (map[string]ports.PlayerSnapshot, error) {
	if f.err != nil {
		return nil, f.err
	}
	out := map[string]ports.PlayerSnapshot{}
	for _, id := range ids {
		if p, ok := f.players[id]; ok && p.TenantID == tenantID {
			out[id] = p
		}
	}
	return out, nil
}

// flakyMailer fails the first failures sends with err, then records.
type flakyMailer struct {
	mail.Recorder
	failures int
	err      error
	calls    int
}

func (m *flakyMailer) Send(ctx context.Context, msg mail.Message) error {
	m.calls++
	if m.calls <= m.failures {
		return m.err
	}
	return m.Recorder.Send(ctx, msg)
}

// allowKeys enforces only the listed permission keys.
type allowKeys map[string]bool

func (a allowKeys) Authorize(_ context.Context, _ authz.Principal, perm authz.Permission, _ any) error {
	if a[perm.Key()] {
		return nil
	}
	return errs.New(errs.PermissionDenied, "missing "+perm.Key())
}
