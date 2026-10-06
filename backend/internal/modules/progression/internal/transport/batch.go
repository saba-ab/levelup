package transport

import (
	"net/http"
	"strings"

	"github.com/google/uuid"

	"levelup/internal/modules/progression/internal/app"
	"levelup/internal/platform/httpx"
	"levelup/internal/shared/errs"
)

// ProgressListResp is a batch of progress views, in request order.
type ProgressListResp struct {
	Data []ProgressResp `json:"data"`
}

// @Summary      Batch-read players' XP and level
// @Description  Same shape as GET /players/{playerID}/progress per player. Unknown or foreign players are omitted. At most 100 ids.
// @Tags         progression
// @Produce      json
// @Security     BearerAuth
// @Param        player_ids query string true "Comma-separated player ids (uuid), max 100"
// @Success      200 {object} ProgressListResp
// @Failure      401 {object} httpx.Problem
// @Failure      403 {object} httpx.Problem
// @Failure      422 {object} httpx.Problem "too_many_ids"
// @Router       /progress [get]
func (h *Handler) batchProgress(w http.ResponseWriter, r *http.Request) {
	ids, err := uuidList(r.URL.Query().Get("player_ids"), "player_ids", app.MaxBatchPlayers)
	if err != nil {
		httpx.Error(w, r, err)
		return
	}
	views, err := h.svc.BatchProgress(r.Context(), ids)
	if err != nil {
		httpx.Error(w, r, err)
		return
	}
	out := ProgressListResp{Data: make([]ProgressResp, len(views))}
	for i, v := range views {
		out.Data[i] = toProgressResp(v)
	}
	httpx.JSON(w, http.StatusOK, out)
}

// uuidList parses a comma-separated list of uuids (required, at most max).
func uuidList(raw, field string, maxIDs int) ([]string, error) {
	var out []string
	for _, part := range strings.Split(raw, ",") {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		if _, err := uuid.Parse(part); err != nil {
			return nil, errs.WithFields(errs.New(errs.Invalid, field+" must be uuids"),
				map[string]string{field: "must be comma-separated UUIDs"})
		}
		out = append(out, part)
	}
	if len(out) == 0 {
		return nil, errs.WithFields(errs.New(errs.Invalid, field+" is required"),
			map[string]string{field: "is required"})
	}
	if len(out) > maxIDs {
		return nil, errs.WithFields(errs.WithCode(errs.New(errs.Invalid, "too many ids"), "too_many_ids"),
			map[string]string{field: "at most 100 ids"})
	}
	return out, nil
}
