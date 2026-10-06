package domain

import (
	"time"

	"levelup/internal/modules/notifications/contracts"
)

// Notification is one rendered message to one player on one channel. The
// natural key (template_id, event_id, channel) makes fan-out idempotent.
type Notification struct {
	ID          string
	TenantID    string
	TemplateID  string
	PlayerID    string
	EventID     string
	Trigger     string
	Channel     string
	Status      string
	Title       string
	Body        string
	Reason      string
	Attempts    int
	LastError   string
	LeaseUntil  *time.Time
	DeliveredAt *time.Time
	ReadAt      *time.Time
	CreatedAt   time.Time
	UpdatedAt   time.Time
}

func (n Notification) Read() bool { return n.ReadAt != nil }

// Pending reports an email row still owned by the send job.
func (n Notification) Pending() bool { return n.Status == contracts.StatusPending }

// Deliver settles the row as delivered.
func (n *Notification) Deliver(now time.Time) {
	n.Status = contracts.StatusDelivered
	n.DeliveredAt = &now
	n.LeaseUntil = nil
	n.LastError = ""
	n.UpdatedAt = now
}

// Fail settles the row as failed with a reason.
func (n *Notification) Fail(reason, lastError string, now time.Time) {
	n.Status = contracts.StatusFailed
	n.Reason = reason
	n.LastError = truncate(lastError, 1000)
	n.LeaseUntil = nil
	n.UpdatedAt = now
}

// Skip settles the row as skipped with a reason.
func (n *Notification) Skip(reason string, now time.Time) {
	n.Status = contracts.StatusSkipped
	n.Reason = reason
	n.LeaseUntil = nil
	n.UpdatedAt = now
}

// Retry keeps the row pending, recording the transient error and releasing
// the lease so the next delivery of the job can claim it.
func (n *Notification) Retry(lastError string, now time.Time) {
	n.LastError = truncate(lastError, 1000)
	n.LeaseUntil = nil
	n.UpdatedAt = now
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n]
}

// ChannelSettings is a tenant's channel configuration. in_app is always on.
type ChannelSettings struct {
	TenantID      string
	EmailEnabled  bool
	EmailFromName string
	UpdatedAt     time.Time
}

// DefaultChannelSettings applies when the tenant never saved settings:
// email is opt-in.
func DefaultChannelSettings(tenantID string) ChannelSettings {
	return ChannelSettings{TenantID: tenantID}
}

// StatsRow is one (template, channel, status) aggregate.
type StatsRow struct {
	TemplateID string
	Channel    string
	Status     string
	Count      int64
	Read       int64
}

// Counts are the per-bucket numbers of a stats response.
type Counts struct {
	Sent      int64
	Delivered int64
	Failed    int64
	Pending   int64
	Skipped   int64
	Read      int64
}

func (c *Counts) add(r StatsRow) {
	switch r.Status {
	case contracts.StatusDelivered:
		c.Delivered += r.Count
	case contracts.StatusFailed:
		c.Failed += r.Count
	case contracts.StatusPending:
		c.Pending += r.Count
	case contracts.StatusSkipped:
		c.Skipped += r.Count
	}
	if r.Status != contracts.StatusSkipped {
		c.Sent += r.Count
	}
	c.Read += r.Read
}

// Stats aggregates rows. Sent counts every attempted notification (all but
// skipped); OpenRate is the in_app read rate (read / delivered in_app).
type Stats struct {
	Counts
	ByChannel  map[string]Counts
	ByTemplate map[string]Counts
	OpenRate   float64
}

func Aggregate(rows []StatsRow) Stats {
	s := Stats{ByChannel: map[string]Counts{
		contracts.ChannelInApp: {},
		contracts.ChannelEmail: {},
	}, ByTemplate: map[string]Counts{}}
	for _, r := range rows {
		s.add(r)
		ch := s.ByChannel[r.Channel]
		ch.add(r)
		s.ByChannel[r.Channel] = ch
		tpl := s.ByTemplate[r.TemplateID]
		tpl.add(r)
		s.ByTemplate[r.TemplateID] = tpl
	}
	if inApp := s.ByChannel[contracts.ChannelInApp]; inApp.Delivered > 0 {
		s.OpenRate = float64(inApp.Read) / float64(inApp.Delivered)
	}
	return s
}
