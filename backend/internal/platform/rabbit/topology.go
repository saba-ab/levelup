package rabbit

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"time"

	amqp "github.com/rabbitmq/amqp091-go"
	"go.uber.org/zap"

	"levelup/internal/shared/errs"
)

// Exchange and queue naming, declared once and only here (PRD §7.9):
//
//	events   topic  exchange — facts; routing key = topic (user.registered.v1)
//	jobs     direct exchange — work;  routing key = job name
//	*.dlx    direct exchanges for dead letters, routing key = source queue
//
//	evt.<group>.<topic>            quorum   consumer queue per (module, topic)
//	evt.<group>.<topic>.retry.<t>  quorum   TTL tier, dead-letters back to source
//	evt.<group>.<topic>.dlq        quorum   parked poison messages
//	job.<name> (+ .retry/.dlq)     quorum   one queue per job type
//	job.priority                   CLASSIC  the single sanctioned exception:
//	         quorum queues do not support x-max-priority; the durability trade
//	         is confined to fire-and-forget work and named in the runbook.
const (
	ExchangeEvents    = "events"
	ExchangeJobs      = "jobs"
	ExchangeEventsDLX = "events.dlx"
	ExchangeJobsDLX   = "jobs.dlx"

	PriorityQueue = "job.priority"
)

type RetryTier struct {
	Suffix string
	TTL    time.Duration
}

// RetryTiers is the bounded backoff ladder (PRD §7.9): attempt n parks in
// tier n; after the last tier the message goes to the DLQ. Quorum queues do
// not support per-message TTL, so backoff is per-queue — a queue per tier.
// A var only so tests can shorten the ladder; production never mutates it.
var RetryTiers = []RetryTier{
	{"5s", 5 * time.Second},
	{"30s", 30 * time.Second},
	{"2m", 2 * time.Minute},
}

func EventQueue(group, topic string) string { return "evt." + group + "." + topic }
func JobQueue(name string) string           { return "job." + name }

type MgmtConfig struct {
	URL      string
	User     string
	Password string
	Vhost    string // default "/"
}

type Topology struct {
	conn *Conn
	mgmt MgmtConfig
	log  *zap.Logger
}

func NewTopology(conn *Conn, mgmt MgmtConfig, log *zap.Logger) *Topology {
	if mgmt.Vhost == "" {
		mgmt.Vhost = "/"
	}
	return &Topology{conn: conn, mgmt: mgmt, log: log}
}

// declareQueue is the ONE path every queue declaration takes. It accepts no
// queue-type argument on purpose: a helper with a quorumMode bool is a
// helper someone eventually passes false to at 2am (PRD §7.9).
func (t *Topology) declareQueue(ch *amqp.Channel, name string, args amqp.Table) error {
	if args == nil {
		args = amqp.Table{}
	}
	args["x-queue-type"] = "quorum" // MANDATORY. No override parameter exists.
	args["x-quorum-initial-group-size"] = 3
	args["x-delivery-limit"] = int32(5) // broker-side poison cap → DLX (R35)
	args["x-max-length"] = int32(1_000_000)
	args["x-overflow"] = "reject-publish" // nack → outbox row stays NULL (PRD §7.9.2)

	_, err := ch.QueueDeclare(name, true, false, false, false, args)
	if err != nil {
		return errs.Wrap(errs.Internal, "declare queue "+name, err)
	}
	return nil
}

// DeclareCore creates the exchanges and the one classic priority queue.
func (t *Topology) DeclareCore(_ context.Context) error {
	ch, err := t.conn.Channel()
	if err != nil {
		return err
	}
	defer ch.Close()

	for _, ex := range []struct{ name, kind string }{
		{ExchangeEvents, "topic"},
		{ExchangeJobs, "direct"},
		{ExchangeEventsDLX, "direct"},
		{ExchangeJobsDLX, "direct"},
	} {
		if err := ch.ExchangeDeclare(ex.name, ex.kind, true, false, false, false, nil); err != nil {
			return errs.Wrap(errs.Internal, "declare exchange "+ex.name, err)
		}
	}

	// The documented classic exception (PRD §7.9): priority ordering needs
	// x-max-priority, which quorum queues do not support.
	if _, err := ch.QueueDeclare(PriorityQueue, true, false, false, false, amqp.Table{
		"x-max-priority": int32(10),
	}); err != nil {
		return errs.Wrap(errs.Internal, "declare priority queue", err)
	}
	if err := ch.QueueBind(PriorityQueue, "priority", ExchangeJobs, false, nil); err != nil {
		return errs.Wrap(errs.Internal, "bind priority queue", err)
	}
	return nil
}

// DeclareEventQueue provisions the consumer queue for one (group, topic)
// pair plus its retry ladder and DLQ — all quorum.
func (t *Topology) DeclareEventQueue(ctx context.Context, group, topic string) error {
	return t.declareConsumerSet(ctx, EventQueue(group, topic), ExchangeEvents, topic, ExchangeEventsDLX)
}

// DeclareJobQueue provisions the work queue for one job type.
func (t *Topology) DeclareJobQueue(ctx context.Context, name string) error {
	return t.declareConsumerSet(ctx, JobQueue(name), ExchangeJobs, name, ExchangeJobsDLX)
}

func (t *Topology) declareConsumerSet(_ context.Context, queue, exchange, routingKey, dlx string) error {
	ch, err := t.conn.Channel()
	if err != nil {
		return err
	}
	defer ch.Close()

	// Main queue: dead-letters (delivery-limit exceeded, rejects) go to the
	// DLX under the queue's own name.
	if err := t.declareQueue(ch, queue, amqp.Table{
		"x-dead-letter-exchange":    dlx,
		"x-dead-letter-routing-key": queue,
	}); err != nil {
		return err
	}
	if err := ch.QueueBind(queue, routingKey, exchange, false, nil); err != nil {
		return errs.Wrap(errs.Internal, "bind "+queue, err)
	}

	// Retry tiers: consumer republishes a failed message to tier n; the TTL
	// expires it back onto the main queue via the default exchange.
	for _, tier := range RetryTiers {
		if err := t.declareQueue(ch, queue+".retry."+tier.Suffix, amqp.Table{
			"x-message-ttl":             int32(tier.TTL / time.Millisecond),
			"x-dead-letter-exchange":    "",
			"x-dead-letter-routing-key": queue,
		}); err != nil {
			return err
		}
	}

	// DLQ: holds precisely the messages you could not afford to lose the
	// first time — it replicates like everything else (PRD §7.9).
	if err := t.declareQueue(ch, queue+".dlq", nil); err != nil {
		return err
	}
	if err := ch.QueueBind(queue+".dlq", queue, dlx, false, nil); err != nil {
		return errs.Wrap(errs.Internal, "bind "+queue+".dlq", err)
	}
	return nil
}

type mgmtQueue struct {
	Name     string `json:"name"`
	Type     string `json:"type"`
	Messages int    `json:"messages"`
}

func (t *Topology) listQueues(ctx context.Context) ([]mgmtQueue, error) {
	u := fmt.Sprintf("%s/api/queues/%s", t.mgmt.URL, url.PathEscape(t.mgmt.Vhost))
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return nil, err
	}
	req.SetBasicAuth(t.mgmt.User, t.mgmt.Password)

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, errs.Wrap(errs.Unavailable, "rabbitmq mgmt api", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, errs.New(errs.Unavailable, fmt.Sprintf("rabbitmq mgmt api: %s", resp.Status))
	}

	var queues []mgmtQueue
	if err := json.NewDecoder(resp.Body).Decode(&queues); err != nil {
		return nil, errs.Wrap(errs.Internal, "decode mgmt response", err)
	}
	return queues, nil
}

// QueueTypes asks the management API for every queue's type.
func (t *Topology) QueueTypes(ctx context.Context) (map[string]string, error) {
	queues, err := t.listQueues(ctx)
	if err != nil {
		return nil, err
	}
	out := make(map[string]string, len(queues))
	for _, q := range queues {
		out[q.Name] = q.Type
	}
	return out, nil
}

// QueueDepths reports ready+unacked message counts per queue (tests,
// operational tooling; the production metric is rabbitmq_queue_depth, R40).
func (t *Topology) QueueDepths(ctx context.Context) (map[string]int, error) {
	queues, err := t.listQueues(ctx)
	if err != nil {
		return nil, err
	}
	out := make(map[string]int, len(queues))
	for _, q := range queues {
		out[q.Name] = q.Messages
	}
	return out, nil
}

// Verify refuses to run against a vhost containing any non-quorum queue
// (R45). Topology drifts — someone declares a queue by hand to debug and it
// is classic forever after. Failing the boot is the correct response: a
// service consuming from a classic queue will lose messages on the next
// node failure and nothing will tell you it happened.
func (t *Topology) Verify(ctx context.Context) error {
	types, err := t.QueueTypes(ctx)
	if err != nil {
		return err
	}
	for name, typ := range types {
		if name == PriorityQueue {
			continue // the one documented exception
		}
		if typ != "quorum" {
			return errs.New(errs.Internal,
				fmt.Sprintf("queue %q is type %q, expected quorum — refusing to run (R45)", name, typ))
		}
	}
	t.log.Info("rabbitmq topology verified: quorum everywhere", zap.Int("queues", len(types)))
	return nil
}
