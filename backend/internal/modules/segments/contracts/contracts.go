// Package contracts is segments' public surface. A segment is a named,
// tenant-defined set of conditions over player data; its membership is
// materialized and recomputed by refresh runs, and every change is
// published as segments.membership_changed.v1.
package contracts

import (
	"context"
	"time"

	"levelup/internal/platform/authz"
)

const Module = "segments"

const (
	// TopicMembershipChanged is published once per player entering or
	// leaving a segment, in the transaction that changes the membership row.
	TopicMembershipChanged = "segments.membership_changed.v1"
	// TopicSegmentDeleted is published when a segment is deleted. Its
	// members are dropped without one membership_changed.v1 per player:
	// consumers drop every membership of the segment.
	TopicSegmentDeleted = "segments.deleted.v1"
)

// Jobs.
const (
	// JobRefresh is the cron that recomputes every live segment.
	JobRefresh = "segments.refresh"
	// JobRefreshSegment recomputes one segment (RefreshSegmentCmdV1); issued
	// through the outbox on create, on a conditions change and by
	// POST /segments/{id}/refresh.
	JobRefreshSegment = "segments.refresh_segment"
)

func Topic(job string) string { return "job." + job }

// MembershipChangedV1.Change values.
const (
	ChangeAdded   = "added"
	ChangeRemoved = "removed"
)

type MembershipChangedV1 struct {
	TenantID  string    `json:"tenant_id"`
	SegmentID string    `json:"segment_id"`
	PlayerID  string    `json:"player_id"`
	Change    string    `json:"change"` // added | removed
	At        time.Time `json:"at"`
	// RunID is the refresh run that made the change ("" when the change
	// came from a player deletion).
	RunID string `json:"run_id,omitempty"`
}

type SegmentDeletedV1 struct {
	TenantID  string    `json:"tenant_id"`
	SegmentID string    `json:"segment_id"`
	At        time.Time `json:"at"`
}

type RefreshSegmentCmdV1 struct {
	TenantID    string    `json:"tenant_id"`
	SegmentID   string    `json:"segment_id"`
	RequestedAt time.Time `json:"requested_at"`
}

var (
	PermView   = authz.Permission{Module: Module, Action: "view"}
	PermManage = authz.Permission{Module: Module, Action: "manage"} // admin roles
)

var AllPermissions = []authz.Permission{PermView, PermManage}

// Reader is segments' offered synchronous read surface.
type Reader interface {
	// SegmentsOfPlayers returns the ids of the live segments each player
	// currently belongs to. Players in no segment are absent.
	SegmentsOfPlayers(ctx context.Context, tenantID string, playerIDs []string) (map[string][]string, error)
}
