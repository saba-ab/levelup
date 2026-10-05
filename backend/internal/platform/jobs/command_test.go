package jobs_test

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/require"

	"levelup/internal/platform/bus"
	"levelup/internal/platform/jobs"
	"levelup/internal/shared/errs"
)

func TestDecodeCommand(t *testing.T) {
	type cmd struct {
		Amount int64 `json:"amount"`
	}
	body, err := json.Marshal(bus.Envelope{EventID: "e1", Topic: "job.points.credit", Payload: json.RawMessage(`{"amount":5}`)})
	require.NoError(t, err)

	var c cmd
	env, err := jobs.DecodeCommand(body, &c)
	require.NoError(t, err)
	require.Equal(t, "e1", env.EventID)
	require.Equal(t, int64(5), c.Amount)

	_, err = jobs.DecodeCommand([]byte("nope"), &c)
	require.Equal(t, errs.Invalid, errs.KindOf(err))
}
