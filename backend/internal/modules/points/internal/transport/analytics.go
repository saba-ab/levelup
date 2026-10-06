package transport

import (
	"net/http"
	"strings"
	"time"

	"github.com/google/uuid"

	"levelup/internal/modules/points/internal/app"
	"levelup/internal/platform/httpx"
	"levelup/internal/shared/errs"
)

// WalletListResp is a batch of wallets, in request order.
type WalletListResp struct {
	Data []WalletResp `json:"data"`
}

// SummaryResp is the tenant-wide wallet roll-up. The *_last_30d sums count
// ledger entries created in the trailing 30 days (transfers and refunds
// included).
type SummaryResp struct {
	OpenWallets     int64 `json:"open_wallets"`
	TotalBalance    int64 `json:"total_balance"`
	LifetimeEarned  int64 `json:"lifetime_earned"`
	LifetimeSpent   int64 `json:"lifetime_spent"`
	CreditedLast30d int64 `json:"credited_last_30d"`
	DebitedLast30d  int64 `json:"debited_last_30d"`
}

// BucketResp counts wallets whose balance lies in [from, to] (inclusive).
type BucketResp struct {
	From    int64 `json:"from"`
	To      int64 `json:"to"`
	Players int64 `json:"players"`
}

// DistributionResp is the balance histogram: 10 equal-width buckets from
// the lowest to the highest balance, or empty when no wallet exists.
type DistributionResp struct {
	Data []BucketResp `json:"data"`
}

// DailyResp is one UTC day of ledger movement.
type DailyResp struct {
	Day      string `json:"day"` // YYYY-MM-DD
	Credited int64  `json:"credited"`
	Debited  int64  `json:"debited"`
}

// DailyListResp is the zero-filled daily series, oldest first.
type DailyListResp struct {
	Data []DailyResp `json:"data"`
}

// @Summary      Batch-read wallets (zero views for never-opened wallets)
// @Description  Unknown or foreign players are omitted. At most 100 ids.
// @Tags         points
// @Produce      json
// @Security     BearerAuth
// @Param        player_ids query string true "Comma-separated player ids (uuid), max 100"
// @Success      200 {object} WalletListResp
// @Failure      401 {object} httpx.Problem
// @Failure      403 {object} httpx.Problem
// @Failure      422 {object} httpx.Problem "too_many_ids"
// @Router       /wallets [get]
func (h *Handler) listWallets(w http.ResponseWriter, r *http.Request) {
	ids, err := uuidList(r.URL.Query().Get("player_ids"), "player_ids", app.MaxBatchPlayers)
	if err != nil {
		httpx.Error(w, r, err)
		return
	}
	views, err := h.svc.WalletsFor(r.Context(), ids)
	if err != nil {
		httpx.Error(w, r, err)
		return
	}
	out := WalletListResp{Data: make([]WalletResp, len(views))}
	for i, v := range views {
		out.Data[i] = toWalletResp(v.Wallet, v.Opened)
	}
	httpx.JSON(w, http.StatusOK, out)
}

// @Summary      Tenant-wide wallet summary
// @Tags         points
// @Produce      json
// @Security     BearerAuth
// @Success      200 {object} SummaryResp
// @Failure      401 {object} httpx.Problem
// @Failure      403 {object} httpx.Problem
// @Router       /wallets/summary [get]
func (h *Handler) summary(w http.ResponseWriter, r *http.Request) {
	s, err := h.svc.Summary(r.Context())
	if err != nil {
		httpx.Error(w, r, err)
		return
	}
	httpx.JSON(w, http.StatusOK, SummaryResp(s))
}

// @Summary      Balance distribution histogram (10 buckets)
// @Tags         points
// @Produce      json
// @Security     BearerAuth
// @Success      200 {object} DistributionResp
// @Failure      401 {object} httpx.Problem
// @Failure      403 {object} httpx.Problem
// @Router       /wallets/distribution [get]
func (h *Handler) distribution(w http.ResponseWriter, r *http.Request) {
	bs, err := h.svc.Distribution(r.Context())
	if err != nil {
		httpx.Error(w, r, err)
		return
	}
	out := DistributionResp{Data: make([]BucketResp, len(bs))}
	for i, b := range bs {
		out.Data[i] = BucketResp(b)
	}
	httpx.JSON(w, http.StatusOK, out)
}

// @Summary      Daily credited/debited totals from the ledger (UTC days)
// @Tags         points
// @Produce      json
// @Security     BearerAuth
// @Param        from query string false "First day, YYYY-MM-DD (default: to - 29 days)"
// @Param        to   query string false "Last day inclusive, YYYY-MM-DD (default: today)"
// @Success      200 {object} DailyListResp
// @Failure      401 {object} httpx.Problem
// @Failure      403 {object} httpx.Problem
// @Failure      422 {object} httpx.Problem "invalid_range"
// @Router       /wallets/daily [get]
func (h *Handler) daily(w http.ResponseWriter, r *http.Request) {
	from, err := dayParam(r, "from")
	if err != nil {
		httpx.Error(w, r, err)
		return
	}
	to, err := dayParam(r, "to")
	if err != nil {
		httpx.Error(w, r, err)
		return
	}
	rows, err := h.svc.Daily(r.Context(), from, to)
	if err != nil {
		httpx.Error(w, r, err)
		return
	}
	out := DailyListResp{Data: make([]DailyResp, len(rows))}
	for i, d := range rows {
		out.Data[i] = DailyResp{Day: d.Day.Format(time.DateOnly), Credited: d.Credited, Debited: d.Debited}
	}
	httpx.JSON(w, http.StatusOK, out)
}

func dayParam(r *http.Request, name string) (time.Time, error) {
	raw := r.URL.Query().Get(name)
	if raw == "" {
		return time.Time{}, nil
	}
	t, err := time.Parse(time.DateOnly, raw)
	if err != nil {
		return time.Time{}, errs.WithFields(errs.New(errs.Invalid, name+" must be a date"),
			map[string]string{name: "must be YYYY-MM-DD"})
	}
	return t, nil
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
