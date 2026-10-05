// Package transport is activity's HTTP layer: request shape is validated
// here, invariants in the domain, business checks in the service.
package transport

import (
	"bytes"
	"encoding/json"
	"net/http"
	"strconv"
	"time"

	"github.com/go-chi/chi/v5"

	"levelup/internal/modules/activity/internal/app"
	"levelup/internal/modules/activity/internal/domain"
	"levelup/internal/platform/httpx"
	"levelup/internal/shared/errs"
	"levelup/internal/shared/validate"
)

type Handler struct {
	svc *app.Service
	val *validate.Validator
}

func NewHandler(svc *app.Service, val *validate.Validator) *Handler {
	return &Handler{svc: svc, val: val}
}

func (h *Handler) Mount(r chi.Router) {
	r.Route("/activities", func(r chi.Router) {
		r.Use(httpx.RequireAuth)
		r.Post("/", h.ingest)
		r.Post("/batch", h.ingestBatch)
		r.Get("/", h.list)
		r.Get("/{id}", h.get)
	})
}

// IngestReq is one activity as a tenant system reports it.
type IngestReq struct {
	EventID          string         `json:"event_id"           validate:"required,max=128"`
	EventType        string         `json:"event_type"         validate:"required,max=100"`
	PlayerExternalID string         `json:"player_external_id" validate:"required,max=255"`
	OccurredAt       *time.Time     `json:"occurred_at"`
	Properties       map[string]any `json:"properties"`
	Context          map[string]any `json:"context"`
}

func (r IngestReq) toCmd() app.IngestCmd {
	return app.IngestCmd{
		EventID:          r.EventID,
		EventType:        r.EventType,
		PlayerExternalID: r.PlayerExternalID,
		OccurredAt:       r.OccurredAt,
		Properties:       r.Properties,
		Context:          r.Context,
	}
}

// BatchReq carries raw items so each one is decoded and validated on its
// own: one malformed item fails that item, not the batch.
type BatchReq struct {
	Items []json.RawMessage `json:"items" validate:"required,min=1,max=100" swaggertype:"array,object"`
}

// IngestResp is the acceptance answer. Activity is the stored row and is
// only present for a duplicate.
type IngestResp struct {
	ActivityID string        `json:"activity_id"`
	Status     string        `json:"status"`
	Duplicate  bool          `json:"duplicate"`
	Activity   *ActivityResp `json:"activity,omitempty"`
}

type ActivityResp struct {
	ID               string         `json:"id"`
	EventID          string         `json:"event_id"`
	EventType        string         `json:"event_type"`
	PlayerExternalID string         `json:"player_external_id,omitempty"`
	PlayerID         string         `json:"player_id,omitempty"`
	Properties       map[string]any `json:"properties"`
	Context          map[string]any `json:"context"`
	OccurredAt       time.Time      `json:"occurred_at"`
	ReceivedAt       time.Time      `json:"received_at"`
	Status           string         `json:"status"`
	DecisionID       string         `json:"decision_id,omitempty"`
	Outcome          string         `json:"outcome,omitempty"`
	Reason           string         `json:"reason,omitempty"`
	DecidedAt        *time.Time     `json:"decided_at,omitempty"`
	CausationDepth   int            `json:"causation_depth"`
	SourceEventID    string         `json:"source_event_id,omitempty"`
	CreatedAt        time.Time      `json:"created_at"`
}

type ListResp struct {
	Data       []ActivityResp `json:"data"`
	NextCursor string         `json:"next_cursor"`
}

// ItemError is a per-item failure inside a batch, shaped like the
// problem+json fields a client branches on.
type ItemError struct {
	Status int               `json:"status"`
	Code   string            `json:"code,omitempty"`
	Detail string            `json:"detail"`
	Errors map[string]string `json:"errors,omitempty"`
}

type BatchItemResp struct {
	Index      int        `json:"index"`
	EventID    string     `json:"event_id,omitempty"`
	ActivityID string     `json:"activity_id,omitempty"`
	Status     string     `json:"status,omitempty"`
	Duplicate  bool       `json:"duplicate"`
	Error      *ItemError `json:"error,omitempty"`
}

type BatchResp struct {
	Results    []BatchItemResp `json:"results"`
	Accepted   int             `json:"accepted"`
	Duplicates int             `json:"duplicates"`
	Failed     int             `json:"failed"`
}

// @Summary      Ingest an activity
// @Description  Stores the activity once per (tenant, event_id) and hands it to rules asynchronously. A repeated event_id returns the stored activity with duplicate=true and publishes nothing.
// @Tags         activity
// @Accept       json
// @Produce      json
// @Security     BearerAuth
// @Param        body body IngestReq true "Activity"
// @Success      202 {object} IngestResp "accepted, decision pending"
// @Success      200 {object} IngestResp "duplicate event_id"
// @Failure      401 {object} httpx.Problem
// @Failure      403 {object} httpx.Problem
// @Failure      422 {object} httpx.Problem "validation, unknown_event_type, occurred_at_in_future, occurred_at_too_old, properties_too_large"
// @Router       /activities [post]
func (h *Handler) ingest(w http.ResponseWriter, r *http.Request) {
	var req IngestReq
	if err := httpx.Decode(r, &req, h.val); err != nil {
		httpx.Error(w, r, err)
		return
	}
	res, err := h.svc.Ingest(r.Context(), req.toCmd())
	if err != nil {
		httpx.Error(w, r, err)
		return
	}
	if res.Duplicate {
		resp := toActivityResp(res.Activity)
		httpx.JSON(w, http.StatusOK, IngestResp{
			ActivityID: res.Activity.ID, Status: res.Activity.Status, Duplicate: true, Activity: &resp,
		})
		return
	}
	httpx.JSON(w, http.StatusAccepted, IngestResp{ActivityID: res.Activity.ID, Status: res.Activity.Status})
}

// @Summary      Ingest up to 100 activities
// @Description  Each item is validated and stored independently; per-item results carry either the activity id or an error.
// @Tags         activity
// @Accept       json
// @Produce      json
// @Security     BearerAuth
// @Param        body body BatchReq true "Activities"
// @Success      202 {object} BatchResp
// @Failure      401 {object} httpx.Problem
// @Failure      403 {object} httpx.Problem
// @Failure      422 {object} httpx.Problem
// @Router       /activities/batch [post]
func (h *Handler) ingestBatch(w http.ResponseWriter, r *http.Request) {
	var req BatchReq
	if err := httpx.Decode(r, &req, h.val); err != nil {
		httpx.Error(w, r, err)
		return
	}

	results := make([]BatchItemResp, len(req.Items))
	var cmds []app.IngestCmd
	var cmdIndex []int
	for i, raw := range req.Items {
		results[i].Index = i
		item, err := h.decodeItem(raw)
		results[i].EventID = item.EventID
		if err != nil {
			results[i].Error = toItemError(err)
			continue
		}
		cmds = append(cmds, item.toCmd())
		cmdIndex = append(cmdIndex, i)
	}

	if len(cmds) > 0 {
		items, err := h.svc.IngestBatch(r.Context(), cmds)
		if err != nil {
			httpx.Error(w, r, err)
			return
		}
		for j, it := range items {
			res := &results[cmdIndex[j]]
			if it.Err != nil {
				res.Error = toItemError(it.Err)
				continue
			}
			res.ActivityID = it.Result.Activity.ID
			res.Status = it.Result.Activity.Status
			res.Duplicate = it.Result.Duplicate
		}
	}

	resp := BatchResp{Results: results}
	for _, res := range results {
		switch {
		case res.Error != nil:
			resp.Failed++
		case res.Duplicate:
			resp.Duplicates++
		default:
			resp.Accepted++
		}
	}
	httpx.JSON(w, http.StatusAccepted, resp)
}

func (h *Handler) decodeItem(raw json.RawMessage) (IngestReq, error) {
	var item IngestReq
	dec := json.NewDecoder(bytes.NewReader(raw))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&item); err != nil {
		return item, errs.Wrap(errs.Invalid, "malformed item", err)
	}
	return item, h.val.Struct(item)
}

// @Summary      List activities
// @Tags         activity
// @Produce      json
// @Security     BearerAuth
// @Param        limit              query int    false "Page size (default 25, max 100)"
// @Param        cursor             query string false "Cursor from next_cursor"
// @Param        event_type         query string false "Filter by event type"
// @Param        player_external_id query string false "Filter by player external id"
// @Param        status             query string false "pending | decided | rejected"
// @Success      200 {object} ListResp
// @Failure      401 {object} httpx.Problem
// @Failure      403 {object} httpx.Problem
// @Failure      422 {object} httpx.Problem
// @Router       /activities [get]
func (h *Handler) list(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	limit := 0
	if raw := q.Get("limit"); raw != "" {
		n, err := strconv.Atoi(raw)
		if err != nil {
			httpx.Error(w, r, errs.WithFields(errs.New(errs.Invalid, "validation failed"),
				map[string]string{"limit": "must be an integer"}))
			return
		}
		limit = n
	}
	page, err := h.svc.List(r.Context(), app.ListFilter{
		EventType:        q.Get("event_type"),
		PlayerExternalID: q.Get("player_external_id"),
		Status:           q.Get("status"),
	}, q.Get("cursor"), limit)
	if err != nil {
		httpx.Error(w, r, err)
		return
	}
	out := ListResp{Data: make([]ActivityResp, len(page.Items)), NextCursor: page.NextCursor}
	for i, a := range page.Items {
		out.Data[i] = toActivityResp(a)
	}
	httpx.JSON(w, http.StatusOK, out)
}

// @Summary      Get an activity with its decision status
// @Tags         activity
// @Produce      json
// @Security     BearerAuth
// @Param        id path string true "Activity id"
// @Success      200 {object} ActivityResp
// @Failure      401 {object} httpx.Problem
// @Failure      403 {object} httpx.Problem
// @Failure      404 {object} httpx.Problem
// @Router       /activities/{id} [get]
func (h *Handler) get(w http.ResponseWriter, r *http.Request) {
	a, err := h.svc.Get(r.Context(), chi.URLParam(r, "id"))
	if err != nil {
		httpx.Error(w, r, err)
		return
	}
	httpx.JSON(w, http.StatusOK, toActivityResp(a))
}

func toActivityResp(a domain.Activity) ActivityResp {
	resp := ActivityResp{
		ID:               a.ID,
		EventID:          a.EventID,
		EventType:        a.EventType,
		PlayerExternalID: a.PlayerExternalID,
		PlayerID:         a.PlayerID,
		Properties:       a.Properties,
		Context:          a.Context,
		OccurredAt:       a.OccurredAt,
		ReceivedAt:       a.ReceivedAt,
		Status:           a.Status,
		DecisionID:       a.DecisionID,
		Outcome:          a.Outcome,
		Reason:           a.Reason,
		CausationDepth:   a.CausationDepth,
		SourceEventID:    a.SourceEventID,
		CreatedAt:        a.CreatedAt,
	}
	if !a.DecidedAt.IsZero() {
		at := a.DecidedAt
		resp.DecidedAt = &at
	}
	if resp.Properties == nil {
		resp.Properties = map[string]any{}
	}
	if resp.Context == nil {
		resp.Context = map[string]any{}
	}
	return resp
}

func toItemError(err error) *ItemError {
	return &ItemError{
		Status: itemStatus(errs.KindOf(err)),
		Code:   errs.CodeOf(err),
		Detail: err.Error(),
		Errors: errs.FieldsOf(err),
	}
}

// itemStatus mirrors httpx's kind→status mapping for per-item results.
// Only client-side kinds reach an item; anything else fails the batch.
func itemStatus(k errs.Kind) int {
	switch k {
	case errs.Invalid:
		return http.StatusUnprocessableEntity
	case errs.NotFound:
		return http.StatusNotFound
	case errs.AlreadyExists, errs.Conflict:
		return http.StatusConflict
	default:
		return http.StatusInternalServerError
	}
}
