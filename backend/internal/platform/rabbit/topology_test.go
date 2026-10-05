package rabbit_test

import (
	"context"
	"testing"

	amqp "github.com/rabbitmq/amqp091-go"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"

	"levelup/internal/platform/rabbit"
	"levelup/internal/platform/rabbit/rabbittest"
)

func topo(t *testing.T) (*rabbit.Conn, *rabbit.Topology) {
	t.Helper()
	eps := rabbittest.Get(t)
	conn, err := rabbit.Dial(eps.AMQPURL, zap.NewNop(), nil)
	require.NoError(t, err)
	t.Cleanup(func() { _ = conn.Close() })

	return conn, rabbit.NewTopology(conn, rabbit.MgmtConfig{
		URL: eps.MgmtURL, User: eps.User, Password: eps.Password,
	}, zap.NewNop())
}

func TestDeclareCreatesQuorumQueuesEverywhere(t *testing.T) {
	conn, topology := topo(t)
	ctx := context.Background()

	require.NoError(t, topology.DeclareCore(ctx))
	require.NoError(t, topology.DeclareEventQueue(ctx, "wallet", "user.registered.v1"))
	require.NoError(t, topology.DeclareJobQueue(ctx, "wallet.reconcile"))

	types, err := topology.QueueTypes(ctx)
	require.NoError(t, err)

	require.Equal(t, "quorum", types["evt.wallet.user.registered.v1"])
	require.Equal(t, "quorum", types["evt.wallet.user.registered.v1.dlq"])
	require.Equal(t, "quorum", types["job.wallet.reconcile"])
	require.Equal(t, "quorum", types["job.wallet.reconcile.dlq"])
	for _, tier := range []string{"5s", "30s", "2m"} {
		require.Equal(t, "quorum", types["evt.wallet.user.registered.v1.retry."+tier],
			"retry queues hold messages you could not process — they replicate too (PRD §7.9)")
	}

	// The single sanctioned classic exception: the priority queue (PRD §7.9).
	require.Equal(t, "classic", types["job.priority"])

	require.NoError(t, topology.Verify(ctx), "fresh topology must verify clean")
	_ = conn
}

func TestVerifyRefusesClassicQueues(t *testing.T) {
	conn, topology := topo(t)
	ctx := context.Background()
	require.NoError(t, topology.DeclareCore(ctx))

	// Simulate the 2am incident: someone hand-declares a queue from the
	// management UI and it is classic forever after (PRD §7.9).
	ch, err := conn.Channel()
	require.NoError(t, err)
	_, err = ch.QueueDeclare("evt.rogue.hand-made", true, false, false, false, amqp.Table{})
	require.NoError(t, err)
	_ = ch.Close()

	err = topology.Verify(ctx)
	require.Error(t, err, "a classic queue in the vhost must fail verification (R45)")
	require.Contains(t, err.Error(), "evt.rogue.hand-made")
}
