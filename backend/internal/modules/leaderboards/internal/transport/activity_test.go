package transport

import (
	"encoding/json"
	"net/http"
	"testing"

	"github.com/stretchr/testify/require"

	"levelup/internal/modules/leaderboards/internal/domain"
)

func TestCreateActivityBoard(t *testing.T) {
	h, _ := newServer(t)
	rec := do(h, http.MethodPost, "/leaderboards",
		`{"name":"Big spenders","type":"activity","reset_frequency":"monthly","config":{"event_type":"purchase","value":"property","property":"amount"}}`, true)
	require.Equal(t, http.StatusCreated, rec.Code, rec.Body.String())
	var created LeaderboardResp
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &created))
	require.Equal(t, "earned", created.Metric)
	require.Equal(t, &ConfigResp{EventType: "purchase", Value: "property", Property: "amount"}, created.Config)

	rec = do(h, http.MethodPost, "/leaderboards", `{"name":"Logins","type":"activity","config":{"event_type":"login","value":"count"}}`, true)
	require.Equal(t, http.StatusCreated, rec.Code, rec.Body.String())
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &created))
	require.Equal(t, "count", created.Metric)

	rec = do(h, http.MethodPost, "/leaderboards", `{"name":"Plain","type":"points"}`, true)
	require.Equal(t, http.StatusCreated, rec.Code)
	require.Contains(t, rec.Body.String(), `"config":null`)
}

func TestCreateActivityBoardValidation(t *testing.T) {
	h, _ := newServer(t)
	for body, code := range map[string]string{
		`{"name":"a","type":"activity"}`:                                                               domain.CodeInvalidConfig,
		`{"name":"b","type":"activity","config":{"event_type":"x","value":"property"}}`:                domain.CodeInvalidConfig,
		`{"name":"c","type":"points","config":{"event_type":"x","value":"count"}}`:                     domain.CodeInvalidConfig,
		`{"name":"d","type":"activity","metric":"earned","config":{"event_type":"x","value":"count"}}`: domain.CodeInvalidMetric,
		`{"name":"e","type":"activity","config":{"event_type":"x","value":"max"}}`:                     "",
		`{"name":"f","type":"activity","config":{"value":"count"}}`:                                    "",
	} {
		rec := do(h, http.MethodPost, "/leaderboards", body, true)
		require.Equal(t, http.StatusUnprocessableEntity, rec.Code, body)
		if code != "" {
			require.Contains(t, rec.Body.String(), code, body)
		}
	}
}
