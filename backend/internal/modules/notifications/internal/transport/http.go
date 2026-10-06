// Package transport is notifications' HTTP layer: request shape validation
// here, invariants in the domain and service.
package transport

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"sort"
	"strconv"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"

	"levelup/internal/modules/notifications/contracts"
	"levelup/internal/modules/notifications/internal/app"
	"levelup/internal/modules/notifications/internal/domain"
	"levelup/internal/platform/httpx"
	"levelup/internal/shared/errs"
	"levelup/internal/shared/pagination"
	"levelup/internal/shared/validate"
)

type Handler struct {
	svc *app.Service
	val *validate.Validator
}

func NewHandler(svc *app.Service, val *validate.Validator) *Handler {
	return &Handler{svc: svc, val: val}
}

// Mount registers notifications' routes. The player feed is registered as
// leaf routes (not r.Route("/players")) so it coexists with the player
// module's /players subtree.
func (h *Handler) Mount(r chi.Router) {
	r.Route("/notifications", func(r chi.Router) {
		r.Use(httpx.RequireAuth)
		r.Get("/templates", h.listTemplates)
		r.Post("/templates", h.createTemplate)
		r.Get("/templates/{id}", h.getTemplate)
		r.Patch("/templates/{id}", h.updateTemplate)
		r.Delete("/templates/{id}", h.deleteTemplate)
		r.Post("/templates/{id}/preview", h.previewTemplate)
		r.Get("/channels", h.getChannels)
		r.Patch("/channels", h.updateChannels)
		r.Get("/history", h.history)
		r.Get("/stats", h.stats)
	})
	auth := r.With(httpx.RequireAuth)
	auth.Get("/players/{playerID}/notifications", h.feed)
	auth.Post("/players/{playerID}/notifications/read-all", h.readAll)
	auth.Post("/players/{playerID}/notifications/{id}/read", h.read)
}

func invalidField(field, msg string) error {
	return errs.WithFields(errs.New(errs.Invalid, "validation failed"), map[string]string{field: msg})
}

// pathUUID returns the path param when it is a UUID, else notFound (a
// malformed id is a 404, never a 500).
func pathUUID(r *http.Request, name string, notFound error) (string, error) {
	v := chi.URLParam(r, name)
	if _, err := uuid.Parse(v); err != nil {
		return "", notFound
	}
	return v, nil
}

func pageParams(r *http.Request) (int, app.PageCursor, error) {
	var (
		limit int
		cur   app.PageCursor
		err   error
	)
	q := r.URL.Query()
	if raw := q.Get("limit"); raw != "" {
		limit, err = strconv.Atoi(raw)
		if err != nil || limit < 1 {
			return 0, cur, invalidField("limit", "must be a positive integer")
		}
	}
	if raw := q.Get("cursor"); raw != "" {
		cur.Before, cur.BeforeID, err = pagination.DecodeCursor(raw)
		if err != nil {
			return 0, cur, err
		}
	}
	return limit, cur, nil
}

func nextCursor(more bool, createdAt time.Time, id string) string {
	if !more {
		return ""
	}
	return pagination.EncodeCursor(createdAt, id)
}

// @Summary      List notification templates
// @Tags         notifications
// @Produce      json
// @Security     BearerAuth
// @Param        trigger   query string false "Filter by trigger" Enums(badges.awarded, progression.level_reached, missions.completed, streaks.milestone_reached, streaks.broken, rewards.claimed, points.credited)
// @Param        is_active query bool   false "Filter by is_active"
// @Param        limit     query int    false "Page size (default 25, max 100)"
// @Param        cursor    query string false "Opaque cursor from next_cursor"
// @Success      200 {object} TemplateListResp
// @Failure      401 {object} httpx.Problem
// @Failure      403 {object} httpx.Problem
// @Failure      422 {object} httpx.Problem
// @Router       /notifications/templates [get]
func (h *Handler) listTemplates(w http.ResponseWriter, r *http.Request) {
	limit, cur, err := pageParams(r)
	if err != nil {
		httpx.Error(w, r, err)
		return
	}
	q := r.URL.Query()
	f := app.TemplateFilter{Trigger: q.Get("trigger"), Cursor: cur, Limit: limit}
	if f.Trigger != "" && !domain.IsTrigger(f.Trigger) {
		httpx.Error(w, r, invalidField("trigger", "unknown trigger"))
		return
	}
	if raw := q.Get("is_active"); raw != "" {
		active, perr := strconv.ParseBool(raw)
		if perr != nil {
			httpx.Error(w, r, invalidField("is_active", "must be a boolean"))
			return
		}
		f.Active = &active
	}
	rows, more, err := h.svc.ListTemplates(r.Context(), f)
	if err != nil {
		httpx.Error(w, r, err)
		return
	}
	out := TemplateListResp{Data: make([]TemplateResp, len(rows))}
	for i, t := range rows {
		out.Data[i] = toTemplateResp(t)
	}
	if more {
		last := rows[len(rows)-1]
		out.NextCursor = nextCursor(true, last.CreatedAt, last.ID)
	}
	httpx.JSON(w, http.StatusOK, out)
}

// @Summary      Create a notification template
// @Description  title_template/body_template are Go text/template sources over the documented fields ({{.Player.DisplayName}}, {{.Badge.Name}}, {{.Level.Number}}, {{.Points.Amount}}, {{.Mission.Name}}, {{.Streak.Milestone}}, {{.Reward.Name}}, ...). They must parse and dry-run; range/define/template/printf are refused.
// @Tags         notifications
// @Accept       json
// @Produce      json
// @Security     BearerAuth
// @Param        body body CreateTemplateReq true "Template"
// @Success      201 {object} TemplateResp
// @Failure      401 {object} httpx.Problem
// @Failure      403 {object} httpx.Problem
// @Failure      409 {object} httpx.Problem "notification_template_name_taken"
// @Failure      422 {object} httpx.Problem "notification_template_invalid"
// @Router       /notifications/templates [post]
func (h *Handler) createTemplate(w http.ResponseWriter, r *http.Request) {
	var req CreateTemplateReq
	if err := httpx.Decode(r, &req, h.val); err != nil {
		httpx.Error(w, r, err)
		return
	}
	t, err := h.svc.CreateTemplate(r.Context(), req.params())
	if err != nil {
		httpx.Error(w, r, err)
		return
	}
	httpx.JSON(w, http.StatusCreated, toTemplateResp(t))
}

// @Summary      Get a notification template
// @Tags         notifications
// @Produce      json
// @Security     BearerAuth
// @Param        id path string true "Template id (uuid)"
// @Success      200 {object} TemplateResp
// @Failure      401 {object} httpx.Problem
// @Failure      403 {object} httpx.Problem
// @Failure      404 {object} httpx.Problem "notification_template_not_found"
// @Router       /notifications/templates/{id} [get]
func (h *Handler) getTemplate(w http.ResponseWriter, r *http.Request) {
	templateID, err := pathUUID(r, "id", domain.ErrTemplateNotFound)
	if err != nil {
		httpx.Error(w, r, err)
		return
	}
	t, err := h.svc.GetTemplate(r.Context(), templateID)
	if err != nil {
		httpx.Error(w, r, err)
		return
	}
	httpx.JSON(w, http.StatusOK, toTemplateResp(t))
}

// @Summary      Update a notification template (partial)
// @Tags         notifications
// @Accept       json
// @Produce      json
// @Security     BearerAuth
// @Param        id   path string            true "Template id (uuid)"
// @Param        body body UpdateTemplateReq true "Fields to change"
// @Success      200 {object} TemplateResp
// @Failure      401 {object} httpx.Problem
// @Failure      403 {object} httpx.Problem
// @Failure      404 {object} httpx.Problem "notification_template_not_found"
// @Failure      409 {object} httpx.Problem "notification_template_name_taken, version_conflict"
// @Failure      422 {object} httpx.Problem "notification_template_invalid"
// @Router       /notifications/templates/{id} [patch]
func (h *Handler) updateTemplate(w http.ResponseWriter, r *http.Request) {
	templateID, err := pathUUID(r, "id", domain.ErrTemplateNotFound)
	if err != nil {
		httpx.Error(w, r, err)
		return
	}
	var req UpdateTemplateReq
	if err := httpx.Decode(r, &req, h.val); err != nil {
		httpx.Error(w, r, err)
		return
	}
	t, err := h.svc.UpdateTemplate(r.Context(), templateID, req.patch())
	if err != nil {
		httpx.Error(w, r, err)
		return
	}
	httpx.JSON(w, http.StatusOK, toTemplateResp(t))
}

// @Summary      Delete a notification template (soft)
// @Description  Its notification history stays.
// @Tags         notifications
// @Security     BearerAuth
// @Param        id path string true "Template id (uuid)"
// @Success      204
// @Failure      401 {object} httpx.Problem
// @Failure      403 {object} httpx.Problem
// @Failure      404 {object} httpx.Problem "notification_template_not_found"
// @Router       /notifications/templates/{id} [delete]
func (h *Handler) deleteTemplate(w http.ResponseWriter, r *http.Request) {
	templateID, err := pathUUID(r, "id", domain.ErrTemplateNotFound)
	if err != nil {
		httpx.Error(w, r, err)
		return
	}
	if err := h.svc.DeleteTemplate(r.Context(), templateID); err != nil {
		httpx.Error(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// @Summary      Preview a notification template
// @Description  Renders title/body against sample data; with player_id the player fields are that player's. The body may be empty.
// @Tags         notifications
// @Accept       json
// @Produce      json
// @Security     BearerAuth
// @Param        id   path string     true  "Template id (uuid)"
// @Param        body body PreviewReq false "Optional player"
// @Success      200 {object} PreviewResp
// @Failure      401 {object} httpx.Problem
// @Failure      403 {object} httpx.Problem
// @Failure      404 {object} httpx.Problem "notification_template_not_found, player_not_found"
// @Failure      422 {object} httpx.Problem
// @Router       /notifications/templates/{id}/preview [post]
func (h *Handler) previewTemplate(w http.ResponseWriter, r *http.Request) {
	templateID, err := pathUUID(r, "id", domain.ErrTemplateNotFound)
	if err != nil {
		httpx.Error(w, r, err)
		return
	}
	var req PreviewReq
	raw, err := io.ReadAll(io.LimitReader(r.Body, 1<<16))
	if err != nil {
		httpx.Error(w, r, errs.Wrap(errs.Invalid, "malformed request body", err))
		return
	}
	if len(bytes.TrimSpace(raw)) > 0 {
		dec := json.NewDecoder(bytes.NewReader(raw))
		dec.DisallowUnknownFields()
		if err := dec.Decode(&req); err != nil {
			httpx.Error(w, r, errs.Wrap(errs.Invalid, "malformed request body", err))
			return
		}
		if err := h.val.Struct(req); err != nil {
			httpx.Error(w, r, err)
			return
		}
	}
	p, err := h.svc.PreviewTemplate(r.Context(), templateID, req.PlayerID)
	if err != nil {
		httpx.Error(w, r, err)
		return
	}
	httpx.JSON(w, http.StatusOK, PreviewResp{Title: p.Rendered.Title, Body: p.Rendered.Body, HTML: p.HTML})
}

// @Summary      Get channel settings
// @Description  in_app is always enabled; email is opt-in per tenant.
// @Tags         notifications
// @Produce      json
// @Security     BearerAuth
// @Success      200 {object} ChannelsResp
// @Failure      401 {object} httpx.Problem
// @Failure      403 {object} httpx.Problem
// @Router       /notifications/channels [get]
func (h *Handler) getChannels(w http.ResponseWriter, r *http.Request) {
	cs, err := h.svc.GetChannels(r.Context())
	if err != nil {
		httpx.Error(w, r, err)
		return
	}
	httpx.JSON(w, http.StatusOK, toChannelsResp(cs))
}

// @Summary      Update channel settings (partial)
// @Tags         notifications
// @Accept       json
// @Produce      json
// @Security     BearerAuth
// @Param        body body UpdateChannelsReq true "Email channel fields to change"
// @Success      200 {object} ChannelsResp
// @Failure      401 {object} httpx.Problem
// @Failure      403 {object} httpx.Problem
// @Failure      422 {object} httpx.Problem
// @Router       /notifications/channels [patch]
func (h *Handler) updateChannels(w http.ResponseWriter, r *http.Request) {
	var req UpdateChannelsReq
	if err := httpx.Decode(r, &req, h.val); err != nil {
		httpx.Error(w, r, err)
		return
	}
	cs, err := h.svc.UpdateChannels(r.Context(), req.patch())
	if err != nil {
		httpx.Error(w, r, err)
		return
	}
	httpx.JSON(w, http.StatusOK, toChannelsResp(cs))
}

// @Summary      Notification history
// @Tags         notifications
// @Produce      json
// @Security     BearerAuth
// @Param        template_id query string false "Filter by template (uuid)"
// @Param        status      query string false "Filter by status" Enums(pending, delivered, failed, skipped)
// @Param        channel     query string false "Filter by channel" Enums(in_app, email)
// @Param        player_id   query string false "Filter by player (uuid)"
// @Param        limit       query int    false "Page size (default 25, max 100)"
// @Param        cursor      query string false "Opaque cursor from next_cursor"
// @Success      200 {object} HistoryListResp
// @Failure      401 {object} httpx.Problem
// @Failure      403 {object} httpx.Problem
// @Failure      422 {object} httpx.Problem
// @Router       /notifications/history [get]
func (h *Handler) history(w http.ResponseWriter, r *http.Request) {
	limit, cur, err := pageParams(r)
	if err != nil {
		httpx.Error(w, r, err)
		return
	}
	q := r.URL.Query()
	f := app.HistoryFilter{TemplateID: q.Get("template_id"), Status: q.Get("status"), Channel: q.Get("channel"),
		PlayerID: q.Get("player_id"), Cursor: cur, Limit: limit}
	for name, v := range map[string]string{"template_id": f.TemplateID, "player_id": f.PlayerID} {
		if v == "" {
			continue
		}
		if _, err := uuid.Parse(v); err != nil {
			httpx.Error(w, r, invalidField(name, "must be a valid UUID"))
			return
		}
	}
	switch f.Status {
	case "", contracts.StatusPending, contracts.StatusDelivered, contracts.StatusFailed, contracts.StatusSkipped:
	default:
		httpx.Error(w, r, invalidField("status", "must be pending, delivered, failed or skipped"))
		return
	}
	switch f.Channel {
	case "", contracts.ChannelInApp, contracts.ChannelEmail:
	default:
		httpx.Error(w, r, invalidField("channel", "must be in_app or email"))
		return
	}
	page, err := h.svc.History(r.Context(), f)
	if err != nil {
		httpx.Error(w, r, err)
		return
	}
	out := HistoryListResp{Data: make([]NotificationResp, len(page.Items))}
	for i, n := range page.Items {
		out.Data[i] = toNotificationResp(n, page.TemplateNames[n.TemplateID])
	}
	if page.More {
		last := page.Items[len(page.Items)-1]
		out.NextCursor = nextCursor(true, last.CreatedAt, last.ID)
	}
	httpx.JSON(w, http.StatusOK, out)
}

// @Summary      Notification stats
// @Description  sent = every non-skipped notification; open_rate = in_app read / in_app delivered (0..1). from/to bound created_at (RFC 3339, to exclusive).
// @Tags         notifications
// @Produce      json
// @Security     BearerAuth
// @Param        from query string false "Created at or after (RFC 3339)"
// @Param        to   query string false "Created before (RFC 3339)"
// @Success      200 {object} StatsResp
// @Failure      401 {object} httpx.Problem
// @Failure      403 {object} httpx.Problem
// @Failure      422 {object} httpx.Problem
// @Router       /notifications/stats [get]
func (h *Handler) stats(w http.ResponseWriter, r *http.Request) {
	var from, to time.Time
	for name, dst := range map[string]*time.Time{"from": &from, "to": &to} {
		raw := r.URL.Query().Get(name)
		if raw == "" {
			continue
		}
		v, err := time.Parse(time.RFC3339, raw)
		if err != nil {
			httpx.Error(w, r, invalidField(name, "must be an RFC 3339 timestamp"))
			return
		}
		*dst = v.UTC()
	}
	s, err := h.svc.Stats(r.Context(), from, to)
	if err != nil {
		httpx.Error(w, r, err)
		return
	}
	httpx.JSON(w, http.StatusOK, toStatsResp(s))
}

// @Summary      A player's in-app notifications
// @Description  For tenant backends and apps. Newest first; unread_count is the player's total unread.
// @Tags         notifications
// @Produce      json
// @Security     BearerAuth
// @Param        playerID path  string true  "Player id (uuid)"
// @Param        unread   query bool   false "Only unread"
// @Param        limit    query int    false "Page size (default 25, max 100)"
// @Param        cursor   query string false "Opaque cursor from next_cursor"
// @Success      200 {object} FeedResp
// @Failure      401 {object} httpx.Problem
// @Failure      403 {object} httpx.Problem
// @Failure      404 {object} httpx.Problem "player_not_found"
// @Failure      422 {object} httpx.Problem
// @Router       /players/{playerID}/notifications [get]
func (h *Handler) feed(w http.ResponseWriter, r *http.Request) {
	playerID, err := pathUUID(r, "playerID", domain.ErrPlayerNotFound)
	if err != nil {
		httpx.Error(w, r, err)
		return
	}
	limit, cur, err := pageParams(r)
	if err != nil {
		httpx.Error(w, r, err)
		return
	}
	f := app.FeedFilter{Cursor: cur, Limit: limit}
	if raw := r.URL.Query().Get("unread"); raw != "" {
		unread, perr := strconv.ParseBool(raw)
		if perr != nil {
			httpx.Error(w, r, invalidField("unread", "must be a boolean"))
			return
		}
		f.UnreadOnly = unread
	}
	page, err := h.svc.PlayerFeed(r.Context(), playerID, f)
	if err != nil {
		httpx.Error(w, r, err)
		return
	}
	out := FeedResp{Data: make([]FeedItemResp, len(page.Items)), UnreadCount: page.UnreadCount}
	for i, n := range page.Items {
		out.Data[i] = toFeedItemResp(n)
	}
	if page.More {
		last := page.Items[len(page.Items)-1]
		out.NextCursor = nextCursor(true, last.CreatedAt, last.ID)
	}
	httpx.JSON(w, http.StatusOK, out)
}

// @Summary      Mark a notification read
// @Description  Idempotent: read_at keeps the first read time.
// @Tags         notifications
// @Produce      json
// @Security     BearerAuth
// @Param        playerID path string true "Player id (uuid)"
// @Param        id       path string true "Notification id (uuid)"
// @Success      200 {object} FeedItemResp
// @Failure      401 {object} httpx.Problem
// @Failure      403 {object} httpx.Problem
// @Failure      404 {object} httpx.Problem "player_not_found, notification_not_found"
// @Router       /players/{playerID}/notifications/{id}/read [post]
func (h *Handler) read(w http.ResponseWriter, r *http.Request) {
	playerID, err := pathUUID(r, "playerID", domain.ErrPlayerNotFound)
	if err != nil {
		httpx.Error(w, r, err)
		return
	}
	notificationID, err := pathUUID(r, "id", domain.ErrNotificationNotFound)
	if err != nil {
		httpx.Error(w, r, err)
		return
	}
	n, err := h.svc.MarkRead(r.Context(), playerID, notificationID)
	if err != nil {
		httpx.Error(w, r, err)
		return
	}
	httpx.JSON(w, http.StatusOK, toFeedItemResp(n))
}

// @Summary      Mark all of a player's notifications read
// @Tags         notifications
// @Produce      json
// @Security     BearerAuth
// @Param        playerID path string true "Player id (uuid)"
// @Success      200 {object} ReadAllResp
// @Failure      401 {object} httpx.Problem
// @Failure      403 {object} httpx.Problem
// @Failure      404 {object} httpx.Problem "player_not_found"
// @Router       /players/{playerID}/notifications/read-all [post]
func (h *Handler) readAll(w http.ResponseWriter, r *http.Request) {
	playerID, err := pathUUID(r, "playerID", domain.ErrPlayerNotFound)
	if err != nil {
		httpx.Error(w, r, err)
		return
	}
	n, err := h.svc.MarkAllRead(r.Context(), playerID)
	if err != nil {
		httpx.Error(w, r, err)
		return
	}
	httpx.JSON(w, http.StatusOK, ReadAllResp{Updated: n})
}

func sortTemplateCounts(rows []TemplateCountsResp) {
	sort.Slice(rows, func(i, j int) bool {
		if rows[i].Sent != rows[j].Sent {
			return rows[i].Sent > rows[j].Sent
		}
		return rows[i].TemplateID < rows[j].TemplateID
	})
}
