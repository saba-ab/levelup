package jobs

import (
	"encoding/json"

	"levelup/internal/platform/bus"
	"levelup/internal/shared/errs"
)

// DecodeCommand unpacks a state-change-triggered job (R46). Those travel
// through the outbox, so the body is a bus.Envelope whose payload is the
// command struct. A body that cannot decode will never decode: it is
// errs.Invalid, which the consumer loop parks in the DLQ immediately.
func DecodeCommand(body []byte, dst any) (bus.Envelope, error) {
	var env bus.Envelope
	if err := json.Unmarshal(body, &env); err != nil {
		return bus.Envelope{}, errs.Wrap(errs.Invalid, "decode job envelope", err)
	}
	if err := json.Unmarshal(env.Payload, dst); err != nil {
		return bus.Envelope{}, errs.Wrap(errs.Invalid, "decode job payload", err)
	}
	return env, nil
}
