// Package rabbit owns the broker connection and topology. NO MODULE MAY
// IMPORT THIS PACKAGE (depguard, PRD §4 P3): the outbox dispatcher is the
// only event publisher, and jobs go through jobs.Queue.
package rabbit

import (
	"sync"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	amqp "github.com/rabbitmq/amqp091-go"
	"go.uber.org/zap"

	"myapp/internal/shared/errs"
)

// Conn wraps one AMQP connection with lazy re-dial and blocked-connection
// visibility (R48): on a disk/memory alarm RabbitMQ blocks publishers
// WITHOUT closing the connection — a silent stall unless someone watches
// NotifyBlocked.
type Conn struct {
	url     string
	log     *zap.Logger
	blocked prometheus.Gauge

	mu   sync.Mutex
	conn *amqp.Connection
}

// Dial connects eagerly once; later calls to Get()/Channel() re-dial as
// needed. reg may be nil (tests).
func Dial(url string, log *zap.Logger, reg *prometheus.Registry) (*Conn, error) {
	c := &Conn{
		url: url,
		log: log,
		blocked: prometheus.NewGauge(prometheus.GaugeOpts{
			Name: "rabbitmq_connection_blocked",
			Help: "1 while the broker is blocking publishers (disk/memory alarm) — R48.",
		}),
	}
	if reg != nil {
		reg.MustRegister(c.blocked)
	}
	if _, err := c.Get(); err != nil {
		return nil, err
	}
	return c, nil
}

// Get returns a live connection, re-dialing with backoff if the previous one
// died. Callers own retry policy above this.
func (c *Conn) Get() (*amqp.Connection, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.conn != nil && !c.conn.IsClosed() {
		return c.conn, nil
	}

	var lastErr error
	for attempt := range 5 {
		conn, err := amqp.Dial(c.url)
		if err == nil {
			c.watch(conn)
			c.conn = conn
			return conn, nil
		}
		lastErr = err
		time.Sleep(time.Duration(attempt+1) * 200 * time.Millisecond)
	}
	return nil, errs.Wrap(errs.Unavailable, "rabbitmq dial", lastErr)
}

func (c *Conn) watch(conn *amqp.Connection) {
	blocked := conn.NotifyBlocked(make(chan amqp.Blocking, 1))
	closed := conn.NotifyClose(make(chan *amqp.Error, 1))
	go func() {
		for {
			select {
			case b, ok := <-blocked:
				if !ok {
					return
				}
				if b.Active {
					c.blocked.Set(1)
					c.log.Warn("rabbitmq is blocking publishers — broker alarm", zap.String("reason", b.Reason))
				} else {
					c.blocked.Set(0)
					c.log.Info("rabbitmq unblocked publishers")
				}
			case err, ok := <-closed:
				if !ok {
					return
				}
				if err != nil {
					c.log.Warn("rabbitmq connection closed", zap.Error(err))
				}
				return
			}
		}
	}()
}

// Channel opens a channel on the live connection, re-dialing if needed.
func (c *Conn) Channel() (*amqp.Channel, error) {
	conn, err := c.Get()
	if err != nil {
		return nil, err
	}
	ch, err := conn.Channel()
	if err != nil {
		return nil, errs.Wrap(errs.Unavailable, "rabbitmq channel", err)
	}
	return ch, nil
}

func (c *Conn) Close() error {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.conn == nil || c.conn.IsClosed() {
		return nil
	}
	return c.conn.Close()
}
