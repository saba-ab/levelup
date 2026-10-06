// Package transport is analytics' HTTP layer: read-only dashboards.
package transport

import (
	"net/http"
	"strconv"
	"time"

	"github.com/go-chi/chi/v5"

	"levelup/internal/modules/analytics/internal/app"
	"levelup/internal/modules/analytics/internal/domain"
	"levelup/internal/platform/httpx"
	"levelup/internal/shared/errs"
)

type Handler struct{ svc *app.Service }

func NewHandler(svc *app.Service) *Handler { return &Handler{svc: svc} }

func (h *Handler) Mount(r chi.Router) {
	r.Route("/analytics", func(r chi.Router) {
		r.Use(httpx.RequireAuth)
		r.Get("/overview", h.overview)
		r.Get("/engagement", h.engagement)
		r.Get("/retention", h.retention)
		r.Get("/funnel", h.funnel)
	})
}

// ---- DTOs ----

// Days are "YYYY-MM-DD" (UTC calendar days).

type OverviewTotals struct {
	Activities        int64 `json:"activities"`
	ActivePlayers     int64 `json:"active_players"`
	NewPlayers        int64 `json:"new_players"`
	PointsCredited    int64 `json:"points_credited"`
	PointsDebited     int64 `json:"points_debited"`
	BadgesAwarded     int64 `json:"badges_awarded"`
	MissionsStarted   int64 `json:"missions_started"`
	MissionsCompleted int64 `json:"missions_completed"`
	LevelsReached     int64 `json:"levels_reached"`
	RewardsClaimed    int64 `json:"rewards_claimed"`
}

type OverviewDay struct {
	Day            string `json:"day"`
	Activities     int64  `json:"activities"`
	ActivePlayers  int64  `json:"active_players"`
	PointsCredited int64  `json:"points_credited"`
	PointsDebited  int64  `json:"points_debited"`
	NewPlayers     int64  `json:"new_players"`
}

type OverviewResp struct {
	From   string         `json:"from"`
	To     string         `json:"to"`
	Totals OverviewTotals `json:"totals"`
	Daily  []OverviewDay  `json:"daily"`
	DAU    int64          `json:"dau"`
	WAU    int64          `json:"wau"`
	MAU    int64          `json:"mau"`
}

type DayCountResp struct {
	Day   string `json:"day"`
	Count int64  `json:"count"`
}

type MissionsResp struct {
	Started   int64 `json:"started"`
	Completed int64 `json:"completed"`
}

type EventTypeCount struct {
	EventType string `json:"event_type"`
	Count     int64  `json:"count"`
}

type BadgeCount struct {
	BadgeID string `json:"badge_id"`
	Count   int64  `json:"count"`
}

type MissionCount struct {
	MissionID string `json:"mission_id"`
	Count     int64  `json:"count"`
}

type LevelCount struct {
	LevelNumber string `json:"level_number"`
	Count       int64  `json:"count"`
}

type EngagementDay struct {
	Day               string `json:"day"`
	BadgesAwarded     int64  `json:"badges_awarded"`
	MissionsStarted   int64  `json:"missions_started"`
	MissionsCompleted int64  `json:"missions_completed"`
	LevelsReached     int64  `json:"levels_reached"`
	RewardsClaimed    int64  `json:"rewards_claimed"`
}

type EngagementResp struct {
	From           string           `json:"from"`
	To             string           `json:"to"`
	BadgesPerDay   []DayCountResp   `json:"badges_per_day"`
	Missions       MissionsResp     `json:"missions"`
	LevelsReached  int64            `json:"levels_reached"`
	RewardsClaimed int64            `json:"rewards_claimed"`
	TopEventTypes  []EventTypeCount `json:"top_event_types"`
	TopBadges      []BadgeCount     `json:"top_badges"`
	TopMissions    []MissionCount   `json:"top_missions"`
	LevelsByNumber []LevelCount     `json:"levels_by_number"`
	Daily          []EngagementDay  `json:"daily"`
}

type CohortResp struct {
	CohortStart string `json:"cohort_start"`
	Size        int64  `json:"size"`
	// Retained[k] is the percentage (0-100, 2 decimals) of the cohort active
	// k weeks after its first week; only weeks that have started are listed.
	Retained []float64 `json:"retained"`
}

type CurvePointResp struct {
	Week    int     `json:"week"`
	Pct     float64 `json:"pct"`
	Cohorts int     `json:"cohorts"`
}

type RetentionResp struct {
	Cohort  string           `json:"cohort"`
	Weeks   int              `json:"weeks"`
	Cohorts []CohortResp     `json:"cohorts"`
	Curve   []CurvePointResp `json:"curve"`
}

type FunnelStepResp struct {
	EventType      string  `json:"event_type"`
	Players        int64   `json:"players"`
	PctOfPrevious  float64 `json:"pct_of_previous"`
	PctOfFirstStep float64 `json:"pct_of_first_step"`
}

type FunnelResp struct {
	From  string           `json:"from"`
	To    string           `json:"to"`
	Steps []FunnelStepResp `json:"steps"`
	// Approximation is "day": step order is resolved per UTC day, so steps
	// done on the same day count as in order.
	Approximation string `json:"approximation"`
}

// ---- handlers ----

// @Summary      Analytics overview
// @Description  Daily activity, active players, points and new players over an inclusive UTC day range (max 366 days; default the 30 days ending today), plus DAU/WAU/MAU ending at "to".
// @Tags         analytics
// @Produce      json
// @Security     BearerAuth
// @Param        from query string false "First day (YYYY-MM-DD or RFC 3339)"
// @Param        to   query string false "Last day (YYYY-MM-DD or RFC 3339), default today"
// @Success      200 {object} OverviewResp
// @Failure      401 {object} httpx.Problem
// @Failure      403 {object} httpx.Problem
// @Failure      422 {object} httpx.Problem
// @Router       /analytics/overview [get]
func (h *Handler) overview(w http.ResponseWriter, r *http.Request) {
	from, to, err := rangeParams(r)
	if err != nil {
		httpx.Error(w, r, err)
		return
	}
	o, err := h.svc.Overview(r.Context(), from, to)
	if err != nil {
		httpx.Error(w, r, err)
		return
	}
	t := o.Totals
	out := OverviewResp{
		From: fmtDay(o.Range.From), To: fmtDay(o.Range.To),
		Totals: OverviewTotals{
			Activities: t.Activities, ActivePlayers: t.ActivePlayers, NewPlayers: t.NewPlayers,
			PointsCredited: t.PointsCredited, PointsDebited: t.PointsDebited, BadgesAwarded: t.BadgesAwarded,
			MissionsStarted: t.MissionsStarted, MissionsCompleted: t.MissionsCompleted,
			LevelsReached: t.LevelsReached, RewardsClaimed: t.RewardsClaimed,
		},
		Daily: make([]OverviewDay, len(o.Daily)),
		DAU:   o.DAU, WAU: o.WAU, MAU: o.MAU,
	}
	for i, d := range o.Daily {
		out.Daily[i] = OverviewDay{
			Day: fmtDay(d.Day), Activities: d.Activities, ActivePlayers: d.ActivePlayers,
			PointsCredited: d.PointsCredited, PointsDebited: d.PointsDebited, NewPlayers: d.NewPlayers,
		}
	}
	httpx.JSON(w, http.StatusOK, out)
}

// @Summary      Analytics engagement
// @Description  Badges per day, missions started/completed, levels reached, rewards claimed and top event types over an inclusive UTC day range (max 366 days).
// @Tags         analytics
// @Produce      json
// @Security     BearerAuth
// @Param        from query string false "First day (YYYY-MM-DD or RFC 3339)"
// @Param        to   query string false "Last day (YYYY-MM-DD or RFC 3339), default today"
// @Success      200 {object} EngagementResp
// @Failure      401 {object} httpx.Problem
// @Failure      403 {object} httpx.Problem
// @Failure      422 {object} httpx.Problem
// @Router       /analytics/engagement [get]
func (h *Handler) engagement(w http.ResponseWriter, r *http.Request) {
	from, to, err := rangeParams(r)
	if err != nil {
		httpx.Error(w, r, err)
		return
	}
	e, err := h.svc.Engagement(r.Context(), from, to)
	if err != nil {
		httpx.Error(w, r, err)
		return
	}
	out := EngagementResp{
		From: fmtDay(e.Range.From), To: fmtDay(e.Range.To),
		BadgesPerDay:   make([]DayCountResp, len(e.BadgesPerDay)),
		Missions:       MissionsResp{Started: e.MissionsStarted, Completed: e.MissionsCompleted},
		LevelsReached:  e.LevelsReached,
		RewardsClaimed: e.RewardsClaimed,
		TopEventTypes:  make([]EventTypeCount, len(e.TopEventTypes)),
		TopBadges:      make([]BadgeCount, len(e.TopBadges)),
		TopMissions:    make([]MissionCount, len(e.TopMissions)),
		LevelsByNumber: make([]LevelCount, len(e.LevelsByNumber)),
		Daily:          make([]EngagementDay, len(e.Daily)),
	}
	for i, d := range e.BadgesPerDay {
		out.BadgesPerDay[i] = DayCountResp{Day: fmtDay(d.Day), Count: d.Count}
	}
	for i, k := range e.TopEventTypes {
		out.TopEventTypes[i] = EventTypeCount{EventType: k.Key, Count: k.Count}
	}
	for i, k := range e.TopBadges {
		out.TopBadges[i] = BadgeCount{BadgeID: k.Key, Count: k.Count}
	}
	for i, k := range e.TopMissions {
		out.TopMissions[i] = MissionCount{MissionID: k.Key, Count: k.Count}
	}
	for i, k := range e.LevelsByNumber {
		out.LevelsByNumber[i] = LevelCount{LevelNumber: k.Key, Count: k.Count}
	}
	for i, d := range e.Daily {
		out.Daily[i] = EngagementDay{
			Day: fmtDay(d.Day), BadgesAwarded: d.BadgesAwarded, MissionsStarted: d.MissionsStarted,
			MissionsCompleted: d.MissionsCompleted, LevelsReached: d.LevelsReached, RewardsClaimed: d.RewardsClaimed,
		}
	}
	httpx.JSON(w, http.StatusOK, out)
}

// @Summary      Retention cohorts
// @Description  Weekly cohorts (ISO weeks starting Monday, UTC) by each player's first activity day, for the last N weeks including the current one. retained[k] is the percentage of the cohort active k weeks later; curve is the size-weighted average per week offset.
// @Tags         analytics
// @Produce      json
// @Security     BearerAuth
// @Param        cohort query string false "Cohort granularity: week (default)"
// @Param        weeks  query int    false "Number of cohorts, 1-52 (default 8)"
// @Success      200 {object} RetentionResp
// @Failure      401 {object} httpx.Problem
// @Failure      403 {object} httpx.Problem
// @Failure      422 {object} httpx.Problem
// @Router       /analytics/retention [get]
func (h *Handler) retention(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	cohort := q.Get("cohort")
	if cohort == "" {
		cohort = "week"
	}
	weeks := 8
	if v := q.Get("weeks"); v != "" {
		n, err := strconv.Atoi(v)
		if err != nil {
			httpx.Error(w, r, domain.ErrBadWeeks)
			return
		}
		weeks = n
	}
	res, err := h.svc.Retention(r.Context(), cohort, weeks)
	if err != nil {
		httpx.Error(w, r, err)
		return
	}
	out := RetentionResp{
		Cohort: cohort, Weeks: weeks,
		Cohorts: make([]CohortResp, len(res.Cohorts)),
		Curve:   make([]CurvePointResp, len(res.Curve)),
	}
	for i, c := range res.Cohorts {
		out.Cohorts[i] = CohortResp{CohortStart: fmtDay(c.Start), Size: c.Size, Retained: c.Retained}
	}
	for i, p := range res.Curve {
		out.Curve[i] = CurvePointResp{Week: p.Week, Pct: p.Pct, Cohorts: p.Cohorts}
	}
	httpx.JSON(w, http.StatusOK, out)
}

// @Summary      Funnel
// @Description  Players who did each event type in order within an inclusive UTC day range (max 366 days). Approximation: order is resolved per UTC day from daily activity, so steps on the same day count as in order.
// @Tags         analytics
// @Produce      json
// @Security     BearerAuth
// @Param        steps query string true  "2-10 comma-separated event types, e.g. signup,first_purchase,repeat_purchase"
// @Param        from  query string false "First day (YYYY-MM-DD or RFC 3339)"
// @Param        to    query string false "Last day (YYYY-MM-DD or RFC 3339), default today"
// @Success      200 {object} FunnelResp
// @Failure      401 {object} httpx.Problem
// @Failure      403 {object} httpx.Problem
// @Failure      422 {object} httpx.Problem
// @Router       /analytics/funnel [get]
func (h *Handler) funnel(w http.ResponseWriter, r *http.Request) {
	from, to, err := rangeParams(r)
	if err != nil {
		httpx.Error(w, r, err)
		return
	}
	f, err := h.svc.Funnel(r.Context(), r.URL.Query().Get("steps"), from, to)
	if err != nil {
		httpx.Error(w, r, err)
		return
	}
	out := FunnelResp{
		From: fmtDay(f.Range.From), To: fmtDay(f.Range.To),
		Steps: make([]FunnelStepResp, len(f.Steps)), Approximation: "day",
	}
	for i, s := range f.Steps {
		out.Steps[i] = FunnelStepResp{
			EventType: s.EventType, Players: s.Players,
			PctOfPrevious: s.PctOfPrevious, PctOfFirstStep: s.PctOfFirstStep,
		}
	}
	httpx.JSON(w, http.StatusOK, out)
}

// ---- helpers ----

func fmtDay(t time.Time) string { return t.UTC().Format(time.DateOnly) }

func rangeParams(r *http.Request) (from, to *time.Time, err error) {
	q := r.URL.Query()
	if from, err = parseDay("from", q.Get("from")); err != nil {
		return nil, nil, err
	}
	if to, err = parseDay("to", q.Get("to")); err != nil {
		return nil, nil, err
	}
	return from, to, nil
}

func parseDay(name, v string) (*time.Time, error) {
	if v == "" {
		return nil, nil
	}
	for _, layout := range []string{time.DateOnly, time.RFC3339} {
		if t, err := time.Parse(layout, v); err == nil {
			return &t, nil
		}
	}
	return nil, errs.WithCode(errs.New(errs.Invalid, name+" must be YYYY-MM-DD or RFC 3339"), domain.CodeInvalidRange)
}
