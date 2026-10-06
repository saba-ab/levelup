package domain

import (
	"encoding/json"
	"fmt"
	"regexp"
	"sort"
	"strconv"

	"levelup/internal/modules/ai/contracts"
)

// Bounds mirrored from the target modules' create DTOs and domains. A draft
// that passes here is accepted by the owning module's create endpoint,
// except for state that changed since the client built its context (an id
// deleted meanwhile, a slug taken).
const (
	maxName            = 255
	maxDescription     = 1000
	maxAmount          = 1_000_000_000
	maxXP              = 1_000_000_000_000
	maxLevelNumber     = 10_000
	maxCount           = 1_000_000
	maxPriority        = 1_000_000
	maxActions         = 20
	maxConditions      = 20
	maxListValues      = 500
	maxMissionInValues = 100
	maxStringValue     = 1000
	maxActionDesc      = 255
	maxActivityKey     = 100
	maxBenefits        = 20
	maxBenefitLen      = 200
	maxCooldown        = 10 * 365 * 24 * 3600
)

var (
	BadgeTiers       = []string{"bronze", "silver", "gold", "platinum", "diamond"}
	BadgeCategories  = []string{"achievement", "milestone", "skill", "social", "exploration", "collection", "special", "seasonal"}
	MissionTypes     = []string{"one_time", "daily", "weekly", "repeating"}
	RewardTypes      = []string{"points", "discount", "item", "badge", "level", "custom"}
	ValueTypes       = []string{"percentage", "fixed"}
	RuleSources      = []string{"trigger", "player", "activity"}
	Operators        = []string{"eq", "neq", "gt", "gte", "lt", "lte", "in", "not_in", "contains", "exists", "not_exists"}
	ActionTypes      = []string{"credit_points", "grant_xp", "award_badge", "record_streak", "progress_mission", "grant_reward"}
	BadgeMetrics     = []string{"lifetime_points", "missions_completed", "streak_days", "level", "badges_earned", "activity_count"}
	MissionOperators = []string{"eq", "neq", "gt", "gte", "lt", "lte", "in", "contains", "exists"}
	LimitKeys        = []string{"max_per_player", "max_per_player_per_day", "max_per_player_per_week", "cooldown_seconds"}

	rewardValuePattern = regexp.MustCompile(`^\d{1,8}(\.\d{1,2})?$`)

	playerFields   = map[string]bool{"id": true, "external_id": true, "display_name": true, "email": true, "is_active": true, "level": true, "xp": true, "points": true}
	activityFields = map[string]bool{"event_type": true, "event_id": true, "causation_depth": true}
)

// Rejection explains why the model's draft at Index was dropped.
type Rejection struct {
	Index   int
	Reasons []string
}

// DraftSet is the outcome of validating the model's output. Each draft is
// the exact JSON body of the owning module's create endpoint.
type DraftSet struct {
	Drafts   []map[string]any
	Rejected []Rejection
}

// NormalizeDrafts validates up to req.Count model drafts. Extra drafts are
// ignored; invalid ones are dropped with reasons.
func NormalizeDrafts(req DraftRequest, raw []any) DraftSet {
	out := DraftSet{Drafts: []map[string]any{}, Rejected: []Rejection{}}
	if len(raw) > req.Count {
		raw = raw[:req.Count]
	}
	var accepted []map[string]any
	for i, item := range raw {
		var reasons []string
		obj, ok := item.(map[string]any)
		if !ok {
			out.Rejected = append(out.Rejected, Rejection{Index: i, Reasons: []string{"draft is not an object"}})
			continue
		}
		f := newFields(obj, "", &reasons)
		switch req.Kind {
		case contracts.KindBadge:
			badgeDraft(f, req.Context)
		case contracts.KindLevel:
			levelDraft(f, req.Context, accepted)
		case contracts.KindMission:
			missionDraft(f, req.Context)
		case contracts.KindReward:
			rewardDraft(f, req.Context)
		case contracts.KindRule:
			ruleDraft(f, req.Context)
		case contracts.KindSegment:
			segmentDraft(f)
		}
		if len(reasons) > 0 {
			sort.Strings(reasons)
			out.Rejected = append(out.Rejected, Rejection{Index: i, Reasons: reasons})
			continue
		}
		accepted = append(accepted, f.out)
		out.Drafts = append(out.Drafts, f.out)
	}
	return out
}

// badgeDraft → badges CreateBadgeReq.
func badgeDraft(f *fields, c Context) {
	f.copyStr("name", true, maxName)
	f.copyStr("description", false, maxDescription)
	f.enum("tier", true, BadgeTiers...)
	f.enum("category", true, BadgeCategories...)
	f.copyInt("points_value", false, 0, maxAmount)
	stackable := f.boolean("is_stackable")
	if n, ok := f.integer("max_awards", false, 1, maxCount); ok && stackable {
		f.out["max_awards"] = n // only meaningful for stackable badges
	}
	f.boolean("is_secret")
	if req, ok := f.object("requirements"); ok {
		if out := badgeRequirements(f.sub(req, "requirements."), c); len(out) > 0 {
			f.out["requirements"] = out
		}
	}
}

// badgeRequirements → the badges requirements grammar:
// {"all": [cond], "any": [cond]}, cond = {metric, event_type?, gte}, where
// event_type narrows activity_count only.
func badgeRequirements(r *fields, c Context) map[string]any {
	out := map[string]any{}
	for _, key := range []string{"all", "any"} {
		items, ok := r.list(key)
		if !ok || len(items) == 0 {
			continue
		}
		if len(items) > maxConditions {
			r.fail(key, fmt.Sprintf("must have at most %d conditions", maxConditions))
			continue
		}
		conds := make([]map[string]any, 0, len(items))
		for i, it := range items {
			obj, isObj := it.(map[string]any)
			if !isObj {
				r.fail(fmt.Sprintf("%s[%d]", key, i), "must be an object")
				continue
			}
			cond := r.sub(obj, fmt.Sprintf("%s[%d].", key, i))
			metric := cond.enum("metric", true, BadgeMetrics...)
			cond.copyInt("gte", true, 1, maxXP)
			if metric == "activity_count" {
				cond.eventType("event_type", false, c)
			} else if _, has := cond.present("event_type"); has {
				cond.fail("event_type", "is only allowed with metric activity_count")
			}
			conds = append(conds, cond.out)
		}
		out[key] = conds
	}
	return out
}

// levelDraft → progression CreateLevelReq. xp_required must strictly
// increase with level_number across the existing ladder and earlier drafts.
func levelDraft(f *fields, c Context, accepted []map[string]any) {
	num, numOK := f.copyInt("level_number", true, 1, maxLevelNumber)
	f.copyStr("name", true, maxName)
	f.copyStr("description", false, maxDescription)
	xp, xpOK := f.copyInt("xp_required", true, 0, maxXP)
	f.copyInt("points_reward", false, 0, maxAmount)
	f.ref("badge_reward_id", c.Badges, "badge")
	if perks, ok := f.object("perks"); ok {
		p := f.sub(perks, "perks.")
		if items, ok := p.list("benefits"); ok {
			if len(items) > maxBenefits {
				p.fail("benefits", fmt.Sprintf("must have at most %d items", maxBenefits))
			}
			benefits := make([]string, 0, len(items))
			for i, it := range items {
				s, isStr := it.(string)
				if !isStr || s == "" || len(s) > maxBenefitLen {
					p.fail("benefits["+strconv.Itoa(i)+"]", fmt.Sprintf("must be a non-empty string of at most %d characters", maxBenefitLen))
					continue
				}
				benefits = append(benefits, s)
			}
			if len(benefits) > 0 {
				f.out["perks"] = map[string]any{"benefits": benefits}
			}
		}
	}
	if !numOK || !xpOK {
		return
	}
	type rung struct {
		num int64
		xp  int64
	}
	ladder := make([]rung, 0, len(c.Levels)+len(accepted))
	for _, l := range c.Levels {
		ladder = append(ladder, rung{int64(l.LevelNumber), l.XPRequired})
	}
	for _, d := range accepted {
		ladder = append(ladder, rung{d["level_number"].(int64), d["xp_required"].(int64)})
	}
	for _, r := range ladder {
		switch {
		case r.num == num:
			f.fail("level_number", fmt.Sprintf("level %d already exists", num))
			return
		case r.num < num && r.xp >= xp, r.num > num && r.xp <= xp:
			f.fail("xp_required", fmt.Sprintf("must increase with level_number (level %d needs %d xp)", r.num, r.xp))
			return
		}
	}
}

// missionDraft → missions CreateMissionReq. Drafts are always created as
// status draft so nothing goes live without review.
func missionDraft(f *fields, c Context) {
	f.copyStr("name", true, maxName)
	f.copyStr("description", false, maxDescription)
	typ := f.enum("type", true, MissionTypes...)
	f.out["status"] = "draft"
	f.copyInt("target", true, 1, maxAmount)
	if crit, ok := f.object("criteria"); ok {
		if out := missionCriteria(f.sub(crit, "criteria."), c); len(out) > 0 {
			f.out["criteria"] = out
		}
	}
	f.copyInt("points_reward", false, 0, maxAmount)
	f.copyInt("xp_reward", false, 0, maxAmount)
	f.ref("badge_reward_id", c.Badges, "badge")
	if n, ok := f.integer("max_completions_per_player", false, 1, maxCount); ok && typ != "one_time" {
		f.out["max_completions_per_player"] = n // one_time missions always carry 1
	}
}

// missionCriteria → the missions criteria grammar: {event_type, where:
// [{field, operator, value}], increment: {by: count} | {by: property,
// field}}. Empty criteria keep the mission manual / rule-driven.
func missionCriteria(cr *fields, c Context) map[string]any {
	eventType := cr.eventType("event_type", false, c)
	where, hasWhere := cr.list("where")
	if hasWhere && len(where) > 0 {
		if len(where) > maxConditions {
			cr.fail("where", fmt.Sprintf("must have at most %d conditions", maxConditions))
		} else {
			conds := make([]map[string]any, 0, len(where))
			for i, it := range where {
				obj, isObj := it.(map[string]any)
				if !isObj {
					cr.fail(fmt.Sprintf("where[%d]", i), "must be an object")
					continue
				}
				conds = append(conds, missionCondition(cr.sub(obj, fmt.Sprintf("where[%d].", i))))
			}
			cr.out["where"] = conds
		}
	}
	if inc, ok := cr.object("increment"); ok {
		in := cr.sub(inc, "increment.")
		switch in.enum("by", true, "count", "property") {
		case "property":
			field, _ := in.str("field", true, maxFieldLen)
			if field != "" && !isPath(field) {
				in.fail("field", "must be a dot path into the activity's properties")
			} else if field != "" {
				in.out["field"] = field
			}
			cr.out["increment"] = in.out
		case "count":
			// The default: omitted.
		}
	}
	if eventType == "" && (cr.out["where"] != nil || cr.out["increment"] != nil) {
		cr.fail("event_type", "is required when where or increment is set")
	}
	return cr.out
}

func missionCondition(l *fields) map[string]any {
	field, _ := l.str("field", true, maxFieldLen)
	if field != "" && !isPath(field) {
		l.fail("field", "must be a dot path into the activity's properties")
	} else if field != "" {
		l.out["field"] = field
	}
	op := l.enum("operator", true, MissionOperators...)
	v, has := l.present("value")
	switch op {
	case "":
	case "exists":
		if b, isBool := v.(bool); has && isBool {
			l.out["value"] = b
		} else if has {
			l.fail("value", "must be a boolean or null")
		}
	case "gt", "gte", "lt", "lte":
		if _, isNum := v.(json.Number); !has || !isNum {
			l.fail("value", "must be a number")
			break
		}
		l.out["value"] = v
	case "in":
		list, isList := v.([]any)
		if !isList || len(list) == 0 || len(list) > maxMissionInValues {
			l.fail("value", fmt.Sprintf("must be a list of 1 to %d values", maxMissionInValues))
			break
		}
		for _, x := range list {
			if !scalarOK(x) {
				l.fail("value", "list items must be strings, numbers or booleans")
				break
			}
		}
		l.out["value"] = list
	default:
		if !has || !scalarOK(v) {
			l.fail("value", "must be a string, number or boolean")
			break
		}
		l.out["value"] = v
	}
	return l.out
}

// rewardDraft → rewards CreateRewardReq, status draft.
func rewardDraft(f *fields, c Context) {
	f.copyStr("name", true, maxName)
	f.copyStr("description", false, maxDescription)
	typ := f.enum("type", true, RewardTypes...)
	f.out["status"] = "draft"
	f.copyInt("points_cost", true, 0, maxAmount)
	value, _ := f.str("value", false, 12)
	if value != "" {
		if rewardValuePattern.MatchString(value) {
			f.out["value"] = value
		} else {
			f.fail("value", "must be a decimal with at most 2 places, e.g. \"10.00\"")
		}
	}
	valueType := f.enum("value_type", false, ValueTypes...)
	if valueType == "percentage" && value != "" {
		if v, err := strconv.ParseFloat(value, 64); err == nil && (v <= 0 || v > 100) {
			f.fail("value", "a percentage must be greater than 0 and at most 100")
		}
	}
	if typ == "discount" && (value == "" || valueType == "") {
		f.fail("value", "discount rewards need value and value_type")
	}
	f.ref("badge_reward_id", c.Badges, "badge")
	if typ == "badge" && f.out["badge_reward_id"] == nil {
		f.fail("badge_reward_id", "badge rewards need a badge listed in context")
	}
	f.copyInt("max_redemptions", false, 1, maxCount)
	f.copyInt("max_per_player", false, 1, maxCount)
	f.copyInt("claim_ttl_days", false, 1, 3650)
	f.copyInt("level_requirement", false, 1, maxLevelNumber)
}

// ruleDraft → rules CreateRuleReq (grammar v1, rules/internal/domain/eval).
func ruleDraft(f *fields, c Context) {
	f.copyStr("name", true, maxName)
	f.copyStr("description", false, maxDescription)
	f.eventType("trigger_event", true, c)
	f.out["priority"] = int64(0)
	f.copyInt("priority", false, 0, maxPriority)
	if cond, ok := f.object("conditions"); ok {
		if out := conditionGroup(f, cond, "conditions", true); out != nil {
			f.out["conditions"] = out
		}
	}
	actions, ok := f.list("actions")
	switch {
	case !ok || len(actions) == 0:
		f.fail("actions", "must be a non-empty list")
	case len(actions) > maxActions:
		f.fail("actions", fmt.Sprintf("must have at most %d actions", maxActions))
	default:
		outActions := make([]map[string]any, 0, len(actions))
		for i, it := range actions {
			obj, isObj := it.(map[string]any)
			if !isObj {
				f.fail(fmt.Sprintf("actions[%d]", i), "must be an object")
				continue
			}
			outActions = append(outActions, ruleAction(f.sub(obj, fmt.Sprintf("actions[%d].", i)), c))
		}
		f.out["actions"] = outActions
	}
	if lim, ok := f.object("limits"); ok {
		l := f.sub(lim, "limits.")
		for _, k := range LimitKeys {
			maxV := int64(maxAmount)
			if k == "cooldown_seconds" {
				maxV = maxCooldown
			}
			l.copyInt(k, false, 1, maxV)
		}
		if len(l.out) > 0 {
			f.out["limits"] = l.out
		}
	}
}

// ruleAction emits only the keys the action type accepts: the rules
// compiler rejects unknown keys.
func ruleAction(a *fields, c Context) map[string]any {
	switch a.enum("type", true, ActionTypes...) {
	case "credit_points", "grant_xp":
		a.copyInt("amount", true, 1, maxAmount)
		a.copyStr("description", false, maxActionDesc)
	case "award_badge":
		requiredRef(a, "badge_id", c.Badges, "badge")
	case "progress_mission":
		requiredRef(a, "mission_id", c.Missions, "mission")
		a.copyInt("increment", false, 1, maxAmount)
	case "grant_reward":
		requiredRef(a, "reward_id", c.Rewards, "reward")
	case "record_streak":
		key := a.copyStr("activity_key", true, maxActivityKey)
		if key != "" && !IsEventSlug(key) {
			a.fail("activity_key", "must be a slug")
		}
	}
	return a.out
}

func requiredRef(a *fields, key string, list []EntityRef, what string) {
	if _, ok := a.present(key); !ok {
		a.fail(key, "is required")
		return
	}
	a.ref(key, list, what)
}

// segmentDraft → segments create: {name, description, conditions} where
// conditions is {all|any: [{field, operator, value}]}.
func segmentDraft(f *fields) {
	f.copyStr("name", true, maxName)
	f.copyStr("description", false, maxDescription)
	cond, ok := f.object("conditions")
	if !ok {
		f.fail("conditions", "is required")
		return
	}
	if out := conditionGroup(f, cond, "conditions", false); out != nil {
		f.out["conditions"] = out
	}
}

// conditionGroup validates {all: [...]} or {any: [...]} (exactly one key,
// one level deep). withSource selects the rules leaf (source + field).
func conditionGroup(f *fields, cond map[string]any, key string, withSource bool) map[string]any {
	g := f.sub(cond, key+".")
	all, hasAll := g.present("all")
	anyOf, hasAny := g.present("any")
	if hasAll == hasAny {
		f.fail(key, "needs exactly one of all or any")
		return nil
	}
	op, items := "all", all
	if hasAny {
		op, items = "any", anyOf
	}
	list, isList := items.([]any)
	if !isList || len(list) == 0 {
		g.fail(op, "must be a non-empty list")
		return nil
	}
	if len(list) > maxConditions {
		g.fail(op, fmt.Sprintf("must have at most %d conditions", maxConditions))
		return nil
	}
	leaves := make([]map[string]any, 0, len(list))
	for i, it := range list {
		obj, isObj := it.(map[string]any)
		if !isObj {
			g.fail(fmt.Sprintf("%s[%d]", op, i), "must be an object")
			continue
		}
		leaves = append(leaves, conditionLeaf(g.sub(obj, fmt.Sprintf("%s[%d].", op, i)), withSource))
	}
	return map[string]any{op: leaves}
}

func conditionLeaf(l *fields, withSource bool) map[string]any {
	field, _ := l.str("field", true, maxFieldLen)
	if withSource {
		src := l.enum("source", true, RuleSources...)
		if field != "" && src != "" && !ruleFieldOK(src, field) {
			l.fail("field", "is not a known "+src+" field")
		}
	} else if field != "" && !isPath(field) {
		l.fail("field", "must be a dot path")
	}
	if field != "" {
		l.out["field"] = field
	}
	op := l.enum("operator", true, Operators...)
	v, has := l.present("value")
	switch op {
	case "":
	case "exists", "not_exists":
	case "in", "not_in":
		if !has {
			l.fail("value", "is required")
			break
		}
		list, isList := v.([]any)
		if !isList {
			list = []any{v}
		}
		if len(list) == 0 || len(list) > maxListValues {
			l.fail("value", fmt.Sprintf("must be a list of 1 to %d values", maxListValues))
			break
		}
		for _, x := range list {
			if !scalarOK(x) {
				l.fail("value", "list items must be strings, numbers or booleans")
				break
			}
		}
		l.out["value"] = list
	case "contains":
		if s, isStr := v.(string); !has || (isStr && s == "") || !scalarOK(v) {
			l.fail("value", "must be a non-empty string, number or boolean")
			break
		}
		l.out["value"] = v
	default:
		if !has || !scalarOK(v) {
			l.fail("value", "must be a string, number or boolean")
			break
		}
		l.out["value"] = v
	}
	return l.out
}

func scalarOK(v any) bool {
	switch x := v.(type) {
	case string:
		return len(x) <= maxStringValue
	case bool, json.Number:
		return true
	default:
		return false
	}
}

// ruleFieldOK mirrors the rules grammar's field table.
func ruleFieldOK(src, field string) bool {
	if !isPath(field) {
		return false
	}
	switch src {
	case "trigger":
		return true
	case "player":
		if playerFields[field] {
			return true
		}
		return hasPathPrefix(field, "attributes.")
	case "activity":
		if activityFields[field] {
			return true
		}
		return hasPathPrefix(field, "properties.") || hasPathPrefix(field, "context.")
	}
	return false
}

func hasPathPrefix(field, prefix string) bool {
	return len(field) > len(prefix) && field[:len(prefix)] == prefix
}
