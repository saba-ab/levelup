// Package contracts is ai's public surface. The ai module drafts gamification
// entities (badges, levels, missions, rewards, rules, segments) with an LLM
// for the portal AI Hub. Drafts are never persisted here: the portal creates
// them through the owning modules' normal create endpoints.
package contracts

import "levelup/internal/platform/authz"

const Module = "ai"

// Draft kinds accepted by POST /ai/drafts.
const (
	KindBadge   = "badge"
	KindLevel   = "level"
	KindMission = "mission"
	KindReward  = "reward"
	KindRule    = "rule"
	KindSegment = "segment"
)

// Kinds lists every draft kind in a stable order.
var Kinds = []string{KindBadge, KindLevel, KindMission, KindReward, KindRule, KindSegment}

// Error codes a client may branch on.
const (
	CodeNotConfigured   = "ai_not_configured"
	CodeQuotaExceeded   = "ai_quota_exceeded"
	CodeUnavailable     = "ai_unavailable"
	CodeRefused         = "ai_refused"
	CodeOutputTruncated = "ai_output_truncated"
	CodeBadOutput       = "ai_bad_output"
	CodeUpstreamError   = "ai_upstream_error"
	CodeInvalidContext  = "invalid_context"
)

var (
	// PermUse covers drafting, the template catalogue and the usage view.
	PermUse = authz.Permission{Module: Module, Action: "use"}
)

var AllPermissions = []authz.Permission{PermUse}
