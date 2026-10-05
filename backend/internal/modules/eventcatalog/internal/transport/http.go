// Package transport is eventcatalog's HTTP layer: DTO shape validation
// here, invariants in the domain, authorization in the service (PRD §7.5).
package transport

import (
	"net/http"
	"net/url"
	"strconv"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"

	"levelup/internal/modules/eventcatalog/internal/app"
	"levelup/internal/modules/eventcatalog/internal/domain"
	"levelup/internal/platform/httpx"
	"levelup/internal/shared/validate"
)

type Handler struct {
	svc *app.Service
	val *validate.Validator
}

func NewHandler(svc *app.Service, val *validate.Validator) *Handler {
	return &Handler{svc: svc, val: val}
}

// Mount registers the tenant surface (/events keeps the Laravel path for
// client compatibility, /event-categories) and the platform surface for the
// global catalogue.
func (h *Handler) Mount(r chi.Router) {
	r.Route("/events", func(r chi.Router) {
		r.Use(httpx.RequireAuth)
		r.Get("/", h.listTypes)
		r.Post("/", h.createType)
		r.Get("/{id}", h.getType)
		r.Patch("/{id}", h.updateType)
		r.Delete("/{id}", h.deleteType)
	})
	r.Route("/event-categories", func(r chi.Router) {
		r.Use(httpx.RequireAuth)
		r.Get("/", h.listCategories)
	})
	r.Route("/platform/event-types", func(r chi.Router) {
		r.Use(httpx.RequireAuth)
		r.Get("/", h.platformListTypes)
		r.Post("/", h.platformCreateType)
		r.Get("/{id}", h.platformGetType)
		r.Patch("/{id}", h.platformUpdateType)
		r.Delete("/{id}", h.platformDeleteType)
	})
	r.Route("/platform/event-categories", func(r chi.Router) {
		r.Use(httpx.RequireAuth)
		r.Get("/", h.platformListCategories)
		r.Post("/", h.platformCreateCategory)
		r.Patch("/{id}", h.platformUpdateCategory)
		r.Delete("/{id}", h.platformDeleteCategory)
	})
}

// --- helpers -------------------------------------------------------------------

// pathID returns the {id} param; a malformed id cannot name a row → 404.
func pathID(r *http.Request, notFound error) (string, error) {
	v := chi.URLParam(r, "id")
	if _, err := uuid.Parse(v); err != nil {
		return "", notFound
	}
	return v, nil
}

func parseBoolParam(q url.Values, name string, def bool) (*bool, error) {
	raw := q.Get(name)
	if raw == "" {
		if !q.Has(name) {
			return &def, nil
		}
		return nil, fieldErr(name, "must be a boolean")
	}
	v, err := strconv.ParseBool(raw)
	if err != nil {
		// Laravel's FILTER_VALIDATE_BOOLEAN read garbage as false.
		return nil, fieldErr(name, "must be a boolean")
	}
	return &v, nil
}

// parseListQuery reads limit, cursor, category, active, include_global and
// search. A malformed value is a 422, never silently coerced.
func parseListQuery(r *http.Request) (app.ListTypesQuery, error) {
	q := r.URL.Query()
	out := app.ListTypesQuery{
		Category: q.Get("category"),
		Search:   q.Get("search"),
		Cursor:   q.Get("cursor"),
	}
	if raw := q.Get("limit"); raw != "" {
		n, err := strconv.Atoi(raw)
		if err != nil || n < 1 || n > app.MaxPageSize {
			return out, fieldErr("limit", "must be between 1 and 100")
		}
		out.Limit = n
	}
	if q.Has("active") {
		v, err := parseBoolParam(q, "active", false)
		if err != nil {
			return out, err
		}
		out.Active = v
	}
	include, err := parseBoolParam(q, "include_global", true)
	if err != nil {
		return out, err
	}
	out.IncludeGlobal = *include
	return out, nil
}

func writeTypePage(w http.ResponseWriter, page app.Page[domain.EventType]) {
	out := EventTypeListResp{Data: make([]EventTypeResp, len(page.Items)), NextCursor: page.NextCursor}
	for i, et := range page.Items {
		out.Data[i] = toEventTypeResp(et)
	}
	httpx.JSON(w, http.StatusOK, out)
}

func writeCategories(w http.ResponseWriter, cs []domain.Category) {
	out := CategoryListResp{Data: make([]CategoryResp, len(cs))}
	for i, c := range cs {
		out.Data[i] = toCategoryResp(c)
	}
	httpx.JSON(w, http.StatusOK, out)
}

func (h *Handler) decodeCreateType(r *http.Request) (app.CreateTypeCmd, error) {
	var req CreateEventTypeReq
	if err := httpx.Decode(r, &req, h.val); err != nil {
		return app.CreateTypeCmd{}, err
	}
	return app.CreateTypeCmd{
		Name:           req.Name,
		Slug:           req.Slug,
		Description:    req.Description,
		CategoryID:     req.CategoryID,
		PropertySchema: req.PropertySchema,
		Active:         req.IsActive,
	}, nil
}

func (h *Handler) decodeUpdateType(r *http.Request) (domain.EventTypePatch, error) {
	var req UpdateEventTypeReq
	if err := httpx.Decode(r, &req, h.val); err != nil {
		return domain.EventTypePatch{}, err
	}
	return req.toPatch()
}

// --- tenant: event types ---------------------------------------------------------

// @Summary      List event types visible to the tenant
// @Description  Own event types plus, unless include_global=false, the platform-global catalogue. A global type shadowed by an own type with the same slug is hidden.
// @Tags         eventcatalog
// @Produce      json
// @Security     BearerAuth
// @Param        limit          query int    false "Page size (default 25, max 100)"
// @Param        cursor         query string false "Cursor from the previous page"
// @Param        category       query string false "Category id or slug"
// @Param        active         query bool   false "Filter by is_active"
// @Param        include_global query bool   false "Include global types (default true)"
// @Param        search         query string false "Substring of name, slug or description"
// @Success      200 {object} EventTypeListResp
// @Failure      401 {object} httpx.Problem
// @Failure      403 {object} httpx.Problem
// @Failure      422 {object} httpx.Problem
// @Router       /events [get]
func (h *Handler) listTypes(w http.ResponseWriter, r *http.Request) {
	q, err := parseListQuery(r)
	if err != nil {
		httpx.Error(w, r, err)
		return
	}
	page, err := h.svc.ListTypes(r.Context(), q)
	if err != nil {
		httpx.Error(w, r, err)
		return
	}
	writeTypePage(w, page)
}

// @Summary      Create a tenant event type
// @Tags         eventcatalog
// @Accept       json
// @Produce      json
// @Security     BearerAuth
// @Param        body body CreateEventTypeReq true "Event type"
// @Success      201 {object} EventTypeResp
// @Failure      401 {object} httpx.Problem
// @Failure      403 {object} httpx.Problem
// @Failure      409 {object} httpx.Problem "event_type_slug_taken"
// @Failure      422 {object} httpx.Problem
// @Router       /events [post]
func (h *Handler) createType(w http.ResponseWriter, r *http.Request) {
	cmd, err := h.decodeCreateType(r)
	if err != nil {
		httpx.Error(w, r, err)
		return
	}
	et, err := h.svc.CreateType(r.Context(), cmd)
	if err != nil {
		httpx.Error(w, r, err)
		return
	}
	httpx.JSON(w, http.StatusCreated, toEventTypeResp(et))
}

// @Summary      Get an own or global event type
// @Tags         eventcatalog
// @Produce      json
// @Security     BearerAuth
// @Param        id path string true "Event type id"
// @Success      200 {object} EventTypeResp
// @Failure      401 {object} httpx.Problem
// @Failure      403 {object} httpx.Problem
// @Failure      404 {object} httpx.Problem
// @Router       /events/{id} [get]
func (h *Handler) getType(w http.ResponseWriter, r *http.Request) {
	id, err := pathID(r, domain.ErrNotFound)
	if err != nil {
		httpx.Error(w, r, err)
		return
	}
	et, err := h.svc.GetType(r.Context(), id)
	if err != nil {
		httpx.Error(w, r, err)
		return
	}
	httpx.JSON(w, http.StatusOK, toEventTypeResp(et))
}

// @Summary      Update a tenant event type (partial)
// @Description  Global types are read-only for tenants (403 global_event_type_read_only). The slug is immutable.
// @Tags         eventcatalog
// @Accept       json
// @Produce      json
// @Security     BearerAuth
// @Param        id   path string             true "Event type id"
// @Param        body body UpdateEventTypeReq true "Fields to change"
// @Success      200 {object} EventTypeResp
// @Failure      401 {object} httpx.Problem
// @Failure      403 {object} httpx.Problem
// @Failure      404 {object} httpx.Problem
// @Failure      422 {object} httpx.Problem
// @Router       /events/{id} [patch]
func (h *Handler) updateType(w http.ResponseWriter, r *http.Request) {
	id, err := pathID(r, domain.ErrNotFound)
	if err != nil {
		httpx.Error(w, r, err)
		return
	}
	patch, err := h.decodeUpdateType(r)
	if err != nil {
		httpx.Error(w, r, err)
		return
	}
	et, err := h.svc.UpdateType(r.Context(), id, patch)
	if err != nil {
		httpx.Error(w, r, err)
		return
	}
	httpx.JSON(w, http.StatusOK, toEventTypeResp(et))
}

// @Summary      Delete a tenant event type (soft)
// @Tags         eventcatalog
// @Security     BearerAuth
// @Param        id path string true "Event type id"
// @Success      204
// @Failure      401 {object} httpx.Problem
// @Failure      403 {object} httpx.Problem
// @Failure      404 {object} httpx.Problem
// @Router       /events/{id} [delete]
func (h *Handler) deleteType(w http.ResponseWriter, r *http.Request) {
	id, err := pathID(r, domain.ErrNotFound)
	if err != nil {
		httpx.Error(w, r, err)
		return
	}
	if err := h.svc.DeleteType(r.Context(), id); err != nil {
		httpx.Error(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// --- tenant: categories --------------------------------------------------------------

// @Summary      List event categories visible to the tenant
// @Description  Own plus global categories ordered by sort_order, name. Not paged; next_cursor is always empty.
// @Tags         eventcatalog
// @Produce      json
// @Security     BearerAuth
// @Success      200 {object} CategoryListResp
// @Failure      401 {object} httpx.Problem
// @Failure      403 {object} httpx.Problem
// @Router       /event-categories [get]
func (h *Handler) listCategories(w http.ResponseWriter, r *http.Request) {
	cs, err := h.svc.ListCategories(r.Context())
	if err != nil {
		httpx.Error(w, r, err)
		return
	}
	writeCategories(w, cs)
}

// --- platform: global event types ----------------------------------------------------

// @Summary      List global event types (platform)
// @Tags         eventcatalog-platform
// @Produce      json
// @Security     BearerAuth
// @Param        limit    query int    false "Page size (default 25, max 100)"
// @Param        cursor   query string false "Cursor from the previous page"
// @Param        category query string false "Category id or slug"
// @Param        active   query bool   false "Filter by is_active"
// @Param        search   query string false "Substring of name, slug or description"
// @Success      200 {object} EventTypeListResp
// @Failure      401 {object} httpx.Problem
// @Failure      403 {object} httpx.Problem
// @Failure      422 {object} httpx.Problem
// @Router       /platform/event-types [get]
func (h *Handler) platformListTypes(w http.ResponseWriter, r *http.Request) {
	q, err := parseListQuery(r)
	if err != nil {
		httpx.Error(w, r, err)
		return
	}
	page, err := h.svc.PlatformListTypes(r.Context(), q)
	if err != nil {
		httpx.Error(w, r, err)
		return
	}
	writeTypePage(w, page)
}

// @Summary      Create a global event type (platform)
// @Tags         eventcatalog-platform
// @Accept       json
// @Produce      json
// @Security     BearerAuth
// @Param        body body CreateEventTypeReq true "Event type"
// @Success      201 {object} EventTypeResp
// @Failure      401 {object} httpx.Problem
// @Failure      403 {object} httpx.Problem
// @Failure      409 {object} httpx.Problem
// @Failure      422 {object} httpx.Problem
// @Router       /platform/event-types [post]
func (h *Handler) platformCreateType(w http.ResponseWriter, r *http.Request) {
	cmd, err := h.decodeCreateType(r)
	if err != nil {
		httpx.Error(w, r, err)
		return
	}
	et, err := h.svc.PlatformCreateType(r.Context(), cmd)
	if err != nil {
		httpx.Error(w, r, err)
		return
	}
	httpx.JSON(w, http.StatusCreated, toEventTypeResp(et))
}

// @Summary      Get a global event type (platform)
// @Tags         eventcatalog-platform
// @Produce      json
// @Security     BearerAuth
// @Param        id path string true "Event type id"
// @Success      200 {object} EventTypeResp
// @Failure      401 {object} httpx.Problem
// @Failure      403 {object} httpx.Problem
// @Failure      404 {object} httpx.Problem
// @Router       /platform/event-types/{id} [get]
func (h *Handler) platformGetType(w http.ResponseWriter, r *http.Request) {
	id, err := pathID(r, domain.ErrNotFound)
	if err != nil {
		httpx.Error(w, r, err)
		return
	}
	et, err := h.svc.PlatformGetType(r.Context(), id)
	if err != nil {
		httpx.Error(w, r, err)
		return
	}
	httpx.JSON(w, http.StatusOK, toEventTypeResp(et))
}

// @Summary      Update a global event type (platform, partial)
// @Tags         eventcatalog-platform
// @Accept       json
// @Produce      json
// @Security     BearerAuth
// @Param        id   path string             true "Event type id"
// @Param        body body UpdateEventTypeReq true "Fields to change"
// @Success      200 {object} EventTypeResp
// @Failure      401 {object} httpx.Problem
// @Failure      403 {object} httpx.Problem
// @Failure      404 {object} httpx.Problem
// @Failure      422 {object} httpx.Problem
// @Router       /platform/event-types/{id} [patch]
func (h *Handler) platformUpdateType(w http.ResponseWriter, r *http.Request) {
	id, err := pathID(r, domain.ErrNotFound)
	if err != nil {
		httpx.Error(w, r, err)
		return
	}
	patch, err := h.decodeUpdateType(r)
	if err != nil {
		httpx.Error(w, r, err)
		return
	}
	et, err := h.svc.PlatformUpdateType(r.Context(), id, patch)
	if err != nil {
		httpx.Error(w, r, err)
		return
	}
	httpx.JSON(w, http.StatusOK, toEventTypeResp(et))
}

// @Summary      Delete a global event type (platform, soft)
// @Tags         eventcatalog-platform
// @Security     BearerAuth
// @Param        id path string true "Event type id"
// @Success      204
// @Failure      401 {object} httpx.Problem
// @Failure      403 {object} httpx.Problem
// @Failure      404 {object} httpx.Problem
// @Router       /platform/event-types/{id} [delete]
func (h *Handler) platformDeleteType(w http.ResponseWriter, r *http.Request) {
	id, err := pathID(r, domain.ErrNotFound)
	if err != nil {
		httpx.Error(w, r, err)
		return
	}
	if err := h.svc.PlatformDeleteType(r.Context(), id); err != nil {
		httpx.Error(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// --- platform: global categories --------------------------------------------------------

// @Summary      List global event categories (platform)
// @Tags         eventcatalog-platform
// @Produce      json
// @Security     BearerAuth
// @Success      200 {object} CategoryListResp
// @Failure      401 {object} httpx.Problem
// @Failure      403 {object} httpx.Problem
// @Router       /platform/event-categories [get]
func (h *Handler) platformListCategories(w http.ResponseWriter, r *http.Request) {
	cs, err := h.svc.PlatformListCategories(r.Context())
	if err != nil {
		httpx.Error(w, r, err)
		return
	}
	writeCategories(w, cs)
}

// @Summary      Create a global event category (platform)
// @Tags         eventcatalog-platform
// @Accept       json
// @Produce      json
// @Security     BearerAuth
// @Param        body body CreateCategoryReq true "Category"
// @Success      201 {object} CategoryResp
// @Failure      401 {object} httpx.Problem
// @Failure      403 {object} httpx.Problem
// @Failure      409 {object} httpx.Problem "event_category_slug_taken"
// @Failure      422 {object} httpx.Problem
// @Router       /platform/event-categories [post]
func (h *Handler) platformCreateCategory(w http.ResponseWriter, r *http.Request) {
	var req CreateCategoryReq
	if err := httpx.Decode(r, &req, h.val); err != nil {
		httpx.Error(w, r, err)
		return
	}
	c, err := h.svc.PlatformCreateCategory(r.Context(), app.CreateCategoryCmd{
		Slug:        req.Slug,
		Name:        req.Name,
		Description: req.Description,
		SortOrder:   req.SortOrder,
	})
	if err != nil {
		httpx.Error(w, r, err)
		return
	}
	httpx.JSON(w, http.StatusCreated, toCategoryResp(c))
}

// @Summary      Update a global event category (platform, partial)
// @Tags         eventcatalog-platform
// @Accept       json
// @Produce      json
// @Security     BearerAuth
// @Param        id   path string            true "Category id"
// @Param        body body UpdateCategoryReq true "Fields to change"
// @Success      200 {object} CategoryResp
// @Failure      401 {object} httpx.Problem
// @Failure      403 {object} httpx.Problem
// @Failure      404 {object} httpx.Problem
// @Failure      409 {object} httpx.Problem
// @Failure      422 {object} httpx.Problem
// @Router       /platform/event-categories/{id} [patch]
func (h *Handler) platformUpdateCategory(w http.ResponseWriter, r *http.Request) {
	id, err := pathID(r, domain.ErrCategoryNotFound)
	if err != nil {
		httpx.Error(w, r, err)
		return
	}
	var req UpdateCategoryReq
	if err := httpx.Decode(r, &req, h.val); err != nil {
		httpx.Error(w, r, err)
		return
	}
	c, err := h.svc.PlatformUpdateCategory(r.Context(), id, domain.CategoryPatch{
		Slug:        req.Slug,
		Name:        req.Name,
		Description: req.Description,
		SortOrder:   req.SortOrder,
	})
	if err != nil {
		httpx.Error(w, r, err)
		return
	}
	httpx.JSON(w, http.StatusOK, toCategoryResp(c))
}

// @Summary      Delete a global event category (platform)
// @Description  Event types using it become uncategorised.
// @Tags         eventcatalog-platform
// @Security     BearerAuth
// @Param        id path string true "Category id"
// @Success      204
// @Failure      401 {object} httpx.Problem
// @Failure      403 {object} httpx.Problem
// @Failure      404 {object} httpx.Problem
// @Router       /platform/event-categories/{id} [delete]
func (h *Handler) platformDeleteCategory(w http.ResponseWriter, r *http.Request) {
	id, err := pathID(r, domain.ErrCategoryNotFound)
	if err != nil {
		httpx.Error(w, r, err)
		return
	}
	if err := h.svc.PlatformDeleteCategory(r.Context(), id); err != nil {
		httpx.Error(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
