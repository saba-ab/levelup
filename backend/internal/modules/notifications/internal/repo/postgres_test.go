package repo

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"gorm.io/gorm"

	"levelup/internal/modules/notifications/contracts"
	"levelup/internal/modules/notifications/internal/app"
	"levelup/internal/modules/notifications/internal/domain"
	"levelup/internal/modules/notifications/migrations"
	"levelup/internal/platform/postgres"
	"levelup/internal/platform/postgres/pgtest"
	"levelup/internal/shared/id"
)

func setupRepo(t *testing.T) (*Postgres, *gorm.DB) {
	t.Helper()
	dsn := pgtest.DSN(t) // skips under -short
	ctx := context.Background()
	require.NoError(t, postgres.Apply(ctx, dsn, "notifications", migrations.FS))

	db, cleanup, err := postgres.NewFromDSNs(ctx, dsn, "", 8)
	require.NoError(t, err)
	t.Cleanup(cleanup)
	base, err := db.GormBase(nil)
	require.NoError(t, err)
	moduleDB := postgres.NewModuleDB(base, "notifications")
	return NewPostgres(moduleDB), moduleDB
}

func now() time.Time { return time.Now().UTC().Truncate(time.Microsecond) }

func inTx(t *testing.T, db *gorm.DB, fn func(tx *gorm.DB) error) {
	t.Helper()
	require.NoError(t, postgres.InTx(context.Background(), db, fn))
}

func seedTemplate(t *testing.T, r *Postgres, db *gorm.DB, tenantID string, mut func(*domain.NewTemplateParams)) domain.Template {
	t.Helper()
	p := domain.NewTemplateParams{TenantID: tenantID, CreatedBy: id.NewID(), Name: "tpl " + id.NewID(),
		Trigger: contracts.TriggerBadgeAwarded, Channels: []string{"in_app", "email"},
		TitleTemplate: "Hi {{.Player.DisplayName}}", BodyTemplate: "body", Active: true}
	if mut != nil {
		mut(&p)
	}
	tpl, err := domain.NewTemplate(p, now(), time.Second)
	require.NoError(t, err)
	inTx(t, db, func(tx *gorm.DB) error { return r.CreateTemplate(context.Background(), tx, tpl) })
	return tpl
}

func newNotification(tpl domain.Template, playerID, eventID, channel, status string, at time.Time) domain.Notification {
	return domain.Notification{ID: id.NewID(), TenantID: tpl.TenantID, TemplateID: tpl.ID, PlayerID: playerID, EventID: eventID,
		Trigger: tpl.Trigger, Channel: channel, Status: status, Title: "t", Body: "b", CreatedAt: at, UpdatedAt: at}
}

func TestTemplateRoundTripChannelsVersionAndNames(t *testing.T) {
	r, db := setupRepo(t)
	ctx := context.Background()
	tenant := id.NewID()
	tpl := seedTemplate(t, r, db, tenant, nil)

	got, err := r.TemplateByID(ctx, tenant, tpl.ID)
	require.NoError(t, err)
	require.Equal(t, []string{"in_app", "email"}, got.Channels, "TEXT[] round-trips")
	require.Equal(t, tpl.CreatedBy, got.CreatedBy)
	_, err = r.TemplateByID(ctx, id.NewID(), tpl.ID)
	require.ErrorIs(t, err, domain.ErrTemplateNotFound)

	dup := tpl
	dup.ID = id.NewID()
	err = postgres.InTx(ctx, db, func(tx *gorm.DB) error { return r.CreateTemplate(ctx, tx, dup) })
	require.ErrorIs(t, err, domain.ErrTemplateNameTaken)

	got.Channels = []string{"email"}
	inTx(t, db, func(tx *gorm.DB) error { return r.SaveTemplate(ctx, tx, got) })
	err = postgres.InTx(ctx, db, func(tx *gorm.DB) error { return r.SaveTemplate(ctx, tx, got) })
	require.ErrorIs(t, err, domain.ErrVersionConflict)
	fresh, err := r.TemplateByID(ctx, tenant, tpl.ID)
	require.NoError(t, err)
	require.Equal(t, []string{"email"}, fresh.Channels)
	require.Equal(t, 1, fresh.Version)

	active, err := r.ActiveTemplatesByTrigger(ctx, tenant, contracts.TriggerBadgeAwarded)
	require.NoError(t, err)
	require.Len(t, active, 1)

	// Soft delete frees the name and hides it from fan-out, but not from TemplatesByIDs.
	d := now()
	fresh.DeletedAt = &d
	inTx(t, db, func(tx *gorm.DB) error { return r.SaveTemplate(ctx, tx, fresh) })
	inTx(t, db, func(tx *gorm.DB) error { return r.CreateTemplate(ctx, tx, dup) })
	active, err = r.ActiveTemplatesByTrigger(ctx, tenant, contracts.TriggerBadgeAwarded)
	require.NoError(t, err)
	require.Len(t, active, 1)
	require.Equal(t, dup.ID, active[0].ID)
	byIDs, err := r.TemplatesByIDs(ctx, tenant, []string{tpl.ID, dup.ID})
	require.NoError(t, err)
	require.Len(t, byIDs, 2)

	off := false
	list, err := r.ListTemplates(ctx, tenant, app.TemplateFilter{Active: &off, Limit: 10})
	require.NoError(t, err)
	require.Empty(t, list)
}

func TestInsertNotificationIsIdempotentOnTemplateEventChannel(t *testing.T) {
	r, db := setupRepo(t)
	ctx := context.Background()
	tenant := id.NewID()
	tpl := seedTemplate(t, r, db, tenant, nil)
	player, event := id.NewID(), id.NewID()

	var inserted []bool
	for range 2 {
		n := newNotification(tpl, player, event, contracts.ChannelInApp, contracts.StatusDelivered, now())
		inTx(t, db, func(tx *gorm.DB) error {
			ok, err := r.InsertNotification(ctx, tx, n)
			inserted = append(inserted, ok)
			return err
		})
	}
	require.Equal(t, []bool{true, false}, inserted, "a redelivered fact inserts nothing")
	inTx(t, db, func(tx *gorm.DB) error {
		ok, err := r.InsertNotification(ctx, tx, newNotification(tpl, player, event, contracts.ChannelEmail, contracts.StatusPending, now()))
		require.True(t, ok, "another channel of the same fact is its own row")
		return err
	})
}

func TestClaimSettleAndStaleSweep(t *testing.T) {
	r, db := setupRepo(t)
	ctx := context.Background()
	tenant := id.NewID()
	tpl := seedTemplate(t, r, db, tenant, nil)
	n := newNotification(tpl, id.NewID(), id.NewID(), contracts.ChannelEmail, contracts.StatusPending, now())
	inTx(t, db, func(tx *gorm.DB) error { _, err := r.InsertNotification(ctx, tx, n); return err })

	at := now()
	var claimed domain.Notification
	inTx(t, db, func(tx *gorm.DB) error {
		var ok bool
		var err error
		claimed, ok, err = r.ClaimEmail(ctx, tx, tenant, n.ID, at, at.Add(time.Minute))
		require.True(t, ok)
		return err
	})
	require.Equal(t, 1, claimed.Attempts)
	require.NotNil(t, claimed.LeaseUntil)

	inTx(t, db, func(tx *gorm.DB) error {
		_, ok, err := r.ClaimEmail(ctx, tx, tenant, n.ID, at, at.Add(time.Minute))
		require.False(t, ok, "the lease blocks a concurrent claim")
		return err
	})
	inTx(t, db, func(tx *gorm.DB) error {
		_, ok, err := r.ClaimEmail(ctx, tx, id.NewID(), n.ID, at.Add(2*time.Minute), at.Add(3*time.Minute))
		require.False(t, ok, "another tenant cannot claim")
		return err
	})

	claimed.Deliver(now())
	inTx(t, db, func(tx *gorm.DB) error {
		ok, err := r.SettleNotification(ctx, tx, claimed)
		require.True(t, ok)
		return err
	})
	inTx(t, db, func(tx *gorm.DB) error {
		ok, err := r.SettleNotification(ctx, tx, claimed)
		require.False(t, ok, "a settled row is not settled twice")
		return err
	})
	got, found, err := r.NotificationByID(ctx, tenant, n.ID)
	require.NoError(t, err)
	require.True(t, found)
	require.Equal(t, contracts.StatusDelivered, got.Status)
	require.Nil(t, got.LeaseUntil)
	require.NotNil(t, got.DeliveredAt)

	old := newNotification(tpl, id.NewID(), id.NewID(), contracts.ChannelEmail, contracts.StatusPending, now().Add(-3*time.Hour))
	inTx(t, db, func(tx *gorm.DB) error { _, err := r.InsertNotification(ctx, tx, old); return err })
	inTx(t, db, func(tx *gorm.DB) error {
		_, err := r.FailStalePending(ctx, tx, now().Add(-2*time.Hour), now())
		return err
	})
	got, _, err = r.NotificationByID(ctx, tenant, old.ID)
	require.NoError(t, err)
	require.Equal(t, contracts.StatusFailed, got.Status)
	require.Equal(t, contracts.ReasonStale, got.Reason)
}

func TestFeedReadStateHistoryStatsAndPurge(t *testing.T) {
	r, db := setupRepo(t)
	ctx := context.Background()
	tenant := id.NewID()
	tpl := seedTemplate(t, r, db, tenant, nil)
	player, other := id.NewID(), id.NewID()
	base := now()
	var feedIDs []string
	for i := range 3 {
		n := newNotification(tpl, player, id.NewID(), contracts.ChannelInApp, contracts.StatusDelivered, base.Add(time.Duration(i)*time.Second))
		feedIDs = append(feedIDs, n.ID)
		inTx(t, db, func(tx *gorm.DB) error { _, err := r.InsertNotification(ctx, tx, n); return err })
	}
	email := newNotification(tpl, player, id.NewID(), contracts.ChannelEmail, contracts.StatusSkipped, base)
	otherRow := newNotification(tpl, other, id.NewID(), contracts.ChannelInApp, contracts.StatusDelivered, base)
	for _, n := range []domain.Notification{email, otherRow} {
		inTx(t, db, func(tx *gorm.DB) error { _, err := r.InsertNotification(ctx, tx, n); return err })
	}

	page, err := r.ListFeed(ctx, tenant, player, app.FeedFilter{Limit: 2})
	require.NoError(t, err)
	require.Len(t, page, 2)
	require.Equal(t, feedIDs[2], page[0].ID, "newest first")
	rest, err := r.ListFeed(ctx, tenant, player, app.FeedFilter{Limit: 10,
		Cursor: app.PageCursor{Before: page[1].CreatedAt, BeforeID: page[1].ID}})
	require.NoError(t, err)
	require.Len(t, rest, 1)

	readAt := now()
	var read domain.Notification
	inTx(t, db, func(tx *gorm.DB) error {
		var ok bool
		var err error
		read, ok, err = r.MarkRead(ctx, tx, tenant, player, feedIDs[0], readAt)
		require.True(t, ok)
		return err
	})
	require.NotNil(t, read.ReadAt)
	inTx(t, db, func(tx *gorm.DB) error {
		again, ok, err := r.MarkRead(ctx, tx, tenant, player, feedIDs[0], readAt.Add(time.Hour))
		require.True(t, ok)
		require.True(t, read.ReadAt.Equal(*again.ReadAt), "read_at keeps the first read")
		return err
	})
	inTx(t, db, func(tx *gorm.DB) error {
		_, ok, err := r.MarkRead(ctx, tx, tenant, other, feedIDs[1], readAt)
		require.False(t, ok, "another player's row")
		return err
	})
	inTx(t, db, func(tx *gorm.DB) error {
		_, ok, err := r.MarkRead(ctx, tx, tenant, player, email.ID, readAt)
		require.False(t, ok, "email rows are not in the feed")
		return err
	})

	unread, err := r.CountUnread(ctx, tenant, player)
	require.NoError(t, err)
	require.Equal(t, int64(2), unread)
	onlyUnread, err := r.ListFeed(ctx, tenant, player, app.FeedFilter{UnreadOnly: true, Limit: 10})
	require.NoError(t, err)
	require.Len(t, onlyUnread, 2)

	var changed int64
	inTx(t, db, func(tx *gorm.DB) error {
		var err error
		changed, err = r.MarkAllRead(ctx, tx, tenant, player, now())
		return err
	})
	require.Equal(t, int64(2), changed)
	unread, err = r.CountUnread(ctx, tenant, other)
	require.NoError(t, err)
	require.Equal(t, int64(1), unread, "other players untouched")

	hist, err := r.ListHistory(ctx, tenant, app.HistoryFilter{Channel: contracts.ChannelEmail, Limit: 10})
	require.NoError(t, err)
	require.Len(t, hist, 1)
	hist, err = r.ListHistory(ctx, tenant, app.HistoryFilter{PlayerID: other, TemplateID: tpl.ID, Status: contracts.StatusDelivered, Limit: 10})
	require.NoError(t, err)
	require.Len(t, hist, 1)

	rows, err := r.Stats(ctx, tenant, time.Time{}, time.Time{})
	require.NoError(t, err)
	s := domain.Aggregate(rows)
	require.Equal(t, int64(4), s.Delivered)
	require.Equal(t, int64(1), s.Skipped)
	require.Equal(t, int64(3), s.Read)
	require.InDelta(t, 0.75, s.OpenRate, 1e-9)
	rows, err = r.Stats(ctx, tenant, base.Add(time.Hour), time.Time{})
	require.NoError(t, err)
	require.Empty(t, rows)

	inTx(t, db, func(tx *gorm.DB) error {
		return r.UpsertChannelSettings(ctx, tx, domain.ChannelSettings{TenantID: tenant, EmailEnabled: true, EmailFromName: "Acme", UpdatedAt: now()})
	})
	inTx(t, db, func(tx *gorm.DB) error {
		return r.UpsertChannelSettings(ctx, tx, domain.ChannelSettings{TenantID: tenant, EmailEnabled: false, EmailFromName: "Acme 2", UpdatedAt: now()})
	})
	cs, found, err := r.ChannelSettings(ctx, tenant)
	require.NoError(t, err)
	require.True(t, found)
	require.False(t, cs.EmailEnabled)
	require.Equal(t, "Acme 2", cs.EmailFromName)

	inTx(t, db, func(tx *gorm.DB) error { return r.PurgePlayer(ctx, tx, tenant, other) })
	hist, err = r.ListHistory(ctx, tenant, app.HistoryFilter{Limit: 100})
	require.NoError(t, err)
	require.Len(t, hist, 4)

	for range 2 {
		inTx(t, db, func(tx *gorm.DB) error { return r.PurgeTenant(ctx, tx, tenant) })
	}
	hist, err = r.ListHistory(ctx, tenant, app.HistoryFilter{Limit: 100})
	require.NoError(t, err)
	require.Empty(t, hist)
	_, found, err = r.ChannelSettings(ctx, tenant)
	require.NoError(t, err)
	require.False(t, found)
	_, err = r.TemplateByID(ctx, tenant, tpl.ID)
	require.ErrorIs(t, err, domain.ErrTemplateNotFound)
}
