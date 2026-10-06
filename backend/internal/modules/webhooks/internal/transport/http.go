// Package transport is webhooks' HTTP layer: DTO shape validation here,
// invariants in the domain and service.
package transport

import (
	"net/http"
	"strconv"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"

	"levelup/internal/modules/webhooks/internal/app"
	"levelup/internal/modules/webhooks/internal/domain"
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
	r.Route("/webhooks", func(r chi.Router) {
		r.Use(httpx.RequireAuth)
		r.Get("/", h.list)
		r.Post("/", h.create)
		r.Get("/event-types", h.eventTypes)
		r.Get("/deliveries", h.listDeliveries)
		r.Get("/deliveries/{deliveryID}", h.getDelivery)
		r.Post("/deliveries/{deliveryID}/redeliver", h.redeliver)
		r.Get("/{id}", h.get)
		r.Patch("/{id}", h.update)
		r.Delete("/{id}", h.delete)
		r.Post("/{id}/rotate-secret", h.rotateSecret)
		r.Post("/{id}/test", h.test)
	})
}

// pathUUID maps a malformed id to the resource's 404.
func pathUUID(r *http.Request, name string, notFound error) (string, error) {
	v := chi.URLParam(r, name)
	if _, err := uuid.Parse(v); err != nil {
		return "", notFound
	}
	return v, nil
}

func limitParam(r *http.Request) (int, error) {
	raw := r.URL.Query().Get("limit")
	if raw == "" {
		return 0, nil
	}
	n, err := strconv.Atoi(raw)
	if err != nil || n < 1 {
		return 0, errs.WithFields(errs.New(errs.Invalid, "validation failed"),
			map[string]string{"limit": "must be a positive integer"})
	}
	return n, nil
}

// @Summary      List webhook endpoints
// @Tags         webhooks
// @Produce      json
// @Security     BearerAuth
// @Param        limit  query int    false "Page size (default 25, max 100)"
// @Param        cursor query string false "Opaque cursor from next_cursor"
// @Success      200 {object} EndpointListResp
// @Failure      401 {object} httpx.Problem
// @Failure      403 {object} httpx.Problem
// @Failure      422 {object} httpx.Problem
// @Router       /webhooks [get]
func (h *Handler) list(w http.ResponseWriter, r *http.Request) {
	limit, err := limitParam(r)
	if err != nil {
		httpx.Error(w, r, err)
		return
	}
	rows, next, err := h.svc.ListEndpoints(r.Context(), r.URL.Query().Get("cursor"), limit)
	if err != nil {
		httpx.Error(w, r, err)
		return
	}
	out := EndpointListResp{Data: make([]EndpointResp, len(rows)), NextCursor: next}
	for i, e := range rows {
		out.Data[i] = toEndpointResp(e)
	}
	httpx.JSON(w, http.StatusOK, out)
}

// @Summary      Register a webhook endpoint
// @Description  The URL must be https (http://localhost only when WEBHOOKS_ALLOW_INSECURE) and must not
// @Description  point at private, loopback, link-local or reserved addresses. The response carries the
// @Description  signing secret; it is never shown again (rotate it to get a new one).
// @Tags         webhooks
// @Accept       json
// @Produce      json
// @Security     BearerAuth
// @Param        body body CreateEndpointReq true "Endpoint"
// @Success      201 {object} EndpointWithSecretResp
// @Failure      401 {object} httpx.Problem
// @Failure      403 {object} httpx.Problem
// @Failure      409 {object} httpx.Problem
// @Failure      422 {object} httpx.Problem
// @Router       /webhooks [post]
func (h *Handler) create(w http.ResponseWriter, r *http.Request) {
	var req CreateEndpointReq
	if err := httpx.Decode(r, &req, h.val); err != nil {
		httpx.Error(w, r, err)
		return
	}
	active := true
	if req.IsActive != nil {
		active = *req.IsActive
	}
	e, err := h.svc.CreateEndpoint(r.Context(), domain.NewEndpointInput{
		URL:         req.URL,
		Description: req.Description,
		EventTypes:  req.EventTypes,
		Active:      active,
	})
	if err != nil {
		httpx.Error(w, r, err)
		return
	}
	httpx.JSON(w, http.StatusCreated, toEndpointWithSecret(e))
}

// @Summary      List the subscribable webhook event types
// @Tags         webhooks
// @Produce      json
// @Security     BearerAuth
// @Success      200 {object} EventTypeListResp
// @Failure      401 {object} httpx.Problem
// @Failure      403 {object} httpx.Problem
// @Router       /webhooks/event-types [get]
func (h *Handler) eventTypes(w http.ResponseWriter, r *http.Request) {
	types, err := h.svc.EventTypes(r.Context())
	if err != nil {
		httpx.Error(w, r, err)
		return
	}
	out := EventTypeListResp{Data: make([]EventTypeResp, len(types))}
	for i, t := range types {
		out.Data[i] = EventTypeResp{Event: t.Event, Description: t.Description}
	}
	httpx.JSON(w, http.StatusOK, out)
}

// @Summary      Get a webhook endpoint
// @Tags         webhooks
// @Produce      json
// @Security     BearerAuth
// @Param        id path string true "Endpoint id"
// @Success      200 {object} EndpointResp
// @Failure      401 {object} httpx.Problem
// @Failure      403 {object} httpx.Problem
// @Failure      404 {object} httpx.Problem
// @Router       /webhooks/{id} [get]
func (h *Handler) get(w http.ResponseWriter, r *http.Request) {
	id, err := pathUUID(r, "id", domain.ErrEndpointNotFound)
	if err != nil {
		httpx.Error(w, r, err)
		return
	}
	e, err := h.svc.GetEndpoint(r.Context(), id)
	if err != nil {
		httpx.Error(w, r, err)
		return
	}
	httpx.JSON(w, http.StatusOK, toEndpointResp(e))
}

// @Summary      Update a webhook endpoint (partial)
// @Tags         webhooks
// @Accept       json
// @Produce      json
// @Security     BearerAuth
// @Param        id   path string            true "Endpoint id"
// @Param        body body UpdateEndpointReq true "Fields to change"
// @Success      200 {object} EndpointResp
// @Failure      401 {object} httpx.Problem
// @Failure      403 {object} httpx.Problem
// @Failure      404 {object} httpx.Problem
// @Failure      409 {object} httpx.Problem
// @Failure      422 {object} httpx.Problem
// @Router       /webhooks/{id} [patch]
func (h *Handler) update(w http.ResponseWriter, r *http.Request) {
	id, err := pathUUID(r, "id", domain.ErrEndpointNotFound)
	if err != nil {
		httpx.Error(w, r, err)
		return
	}
	var req UpdateEndpointReq
	if err := httpx.Decode(r, &req, h.val); err != nil {
		httpx.Error(w, r, err)
		return
	}
	e, err := h.svc.UpdateEndpoint(r.Context(), id, domain.EndpointPatch{
		URL:         req.URL,
		Description: req.Description,
		EventTypes:  req.EventTypes,
		Active:      req.IsActive,
	})
	if err != nil {
		httpx.Error(w, r, err)
		return
	}
	httpx.JSON(w, http.StatusOK, toEndpointResp(e))
}

// @Summary      Delete a webhook endpoint
// @Description  Pending deliveries to it are failed when their job runs.
// @Tags         webhooks
// @Security     BearerAuth
// @Param        id path string true "Endpoint id"
// @Success      204
// @Failure      401 {object} httpx.Problem
// @Failure      403 {object} httpx.Problem
// @Failure      404 {object} httpx.Problem
// @Router       /webhooks/{id} [delete]
func (h *Handler) delete(w http.ResponseWriter, r *http.Request) {
	id, err := pathUUID(r, "id", domain.ErrEndpointNotFound)
	if err != nil {
		httpx.Error(w, r, err)
		return
	}
	if err := h.svc.DeleteEndpoint(r.Context(), id); err != nil {
		httpx.Error(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// @Summary      Rotate a webhook endpoint's signing secret
// @Description  Returns the new secret once. Deliveries sent from now on are signed with it.
// @Tags         webhooks
// @Produce      json
// @Security     BearerAuth
// @Param        id path string true "Endpoint id"
// @Success      200 {object} EndpointWithSecretResp
// @Failure      401 {object} httpx.Problem
// @Failure      403 {object} httpx.Problem
// @Failure      404 {object} httpx.Problem
// @Failure      409 {object} httpx.Problem
// @Router       /webhooks/{id}/rotate-secret [post]
func (h *Handler) rotateSecret(w http.ResponseWriter, r *http.Request) {
	id, err := pathUUID(r, "id", domain.ErrEndpointNotFound)
	if err != nil {
		httpx.Error(w, r, err)
		return
	}
	e, err := h.svc.RotateSecret(r.Context(), id)
	if err != nil {
		httpx.Error(w, r, err)
		return
	}
	httpx.JSON(w, http.StatusOK, toEndpointWithSecret(e))
}

// @Summary      Send a test event to a webhook endpoint
// @Description  Synchronously POSTs a "webhook.test" event and returns the recorded delivery (status
// @Description  succeeded or failed, response status, latency, response snippet). Not retried.
// @Tags         webhooks
// @Produce      json
// @Security     BearerAuth
// @Param        id path string true "Endpoint id"
// @Success      200 {object} DeliveryResp
// @Failure      401 {object} httpx.Problem
// @Failure      403 {object} httpx.Problem
// @Failure      404 {object} httpx.Problem
// @Router       /webhooks/{id}/test [post]
func (h *Handler) test(w http.ResponseWriter, r *http.Request) {
	id, err := pathUUID(r, "id", domain.ErrEndpointNotFound)
	if err != nil {
		httpx.Error(w, r, err)
		return
	}
	d, err := h.svc.SendTest(r.Context(), id)
	if err != nil {
		httpx.Error(w, r, err)
		return
	}
	httpx.JSON(w, http.StatusOK, toDeliveryResp(d, true))
}

// @Summary      List webhook deliveries
// @Tags         webhooks
// @Produce      json
// @Security     BearerAuth
// @Param        endpoint_id query string false "Filter by endpoint id"
// @Param        status      query string false "Filter by status (pending|succeeded|failed)"
// @Param        event       query string false "Filter by event name, e.g. badges.awarded"
// @Param        limit       query int    false "Page size (default 25, max 100)"
// @Param        cursor      query string false "Opaque cursor from next_cursor"
// @Success      200 {object} DeliveryListResp
// @Failure      401 {object} httpx.Problem
// @Failure      403 {object} httpx.Problem
// @Failure      422 {object} httpx.Problem
// @Router       /webhooks/deliveries [get]
func (h *Handler) listDeliveries(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	f := app.DeliveryFilter{EndpointID: q.Get("endpoint_id"), Status: q.Get("status"), Event: q.Get("event")}
	switch f.Status {
	case "", domain.StatusPending, domain.StatusSucceeded, domain.StatusFailed:
	default:
		httpx.Error(w, r, errs.WithFields(errs.New(errs.Invalid, "validation failed"),
			map[string]string{"status": "must be pending, succeeded or failed"}))
		return
	}
	if f.EndpointID != "" {
		if _, err := uuid.Parse(f.EndpointID); err != nil {
			httpx.Error(w, r, errs.WithFields(errs.New(errs.Invalid, "validation failed"),
				map[string]string{"endpoint_id": "must be a uuid"}))
			return
		}
	}
	limit, err := limitParam(r)
	if err != nil {
		httpx.Error(w, r, err)
		return
	}
	rows, next, err := h.svc.ListDeliveries(r.Context(), f, q.Get("cursor"), limit)
	if err != nil {
		httpx.Error(w, r, err)
		return
	}
	out := DeliveryListResp{Data: make([]DeliveryResp, len(rows)), NextCursor: next}
	for i, d := range rows {
		out.Data[i] = toDeliveryResp(d, false)
	}
	httpx.JSON(w, http.StatusOK, out)
}

// @Summary      Get a webhook delivery
// @Tags         webhooks
// @Produce      json
// @Security     BearerAuth
// @Param        deliveryID path string true "Delivery id"
// @Success      200 {object} DeliveryResp
// @Failure      401 {object} httpx.Problem
// @Failure      403 {object} httpx.Problem
// @Failure      404 {object} httpx.Problem
// @Router       /webhooks/deliveries/{deliveryID} [get]
func (h *Handler) getDelivery(w http.ResponseWriter, r *http.Request) {
	id, err := pathUUID(r, "deliveryID", domain.ErrDeliveryNotFound)
	if err != nil {
		httpx.Error(w, r, err)
		return
	}
	d, err := h.svc.GetDelivery(r.Context(), id)
	if err != nil {
		httpx.Error(w, r, err)
		return
	}
	httpx.JSON(w, http.StatusOK, toDeliveryResp(d, true))
}

// @Summary      Redeliver a webhook delivery
// @Description  Re-queues the same payload for a fresh retry cycle (signed with the current secret).
// @Description  Responds 202 with the delivery in status pending.
// @Tags         webhooks
// @Produce      json
// @Security     BearerAuth
// @Param        deliveryID path string true "Delivery id"
// @Success      202 {object} DeliveryResp
// @Failure      401 {object} httpx.Problem
// @Failure      403 {object} httpx.Problem
// @Failure      404 {object} httpx.Problem
// @Failure      409 {object} httpx.Problem
// @Router       /webhooks/deliveries/{deliveryID}/redeliver [post]
func (h *Handler) redeliver(w http.ResponseWriter, r *http.Request) {
	id, err := pathUUID(r, "deliveryID", domain.ErrDeliveryNotFound)
	if err != nil {
		httpx.Error(w, r, err)
		return
	}
	d, err := h.svc.Redeliver(r.Context(), id)
	if err != nil {
		httpx.Error(w, r, err)
		return
	}
	httpx.JSON(w, http.StatusAccepted, toDeliveryResp(d, true))
}
