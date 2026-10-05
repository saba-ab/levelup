// Package rabbittest starts one throwaway RabbitMQ (management-enabled)
// container per test package. Guard callers with testing.Short (R13).
package rabbittest

import (
	"context"
	"net"
	"net/netip"
	"strconv"
	"sync"
	"testing"
	"time"

	"github.com/moby/moby/api/types/container"
	"github.com/moby/moby/api/types/network"
	"github.com/testcontainers/testcontainers-go"
	tcrabbit "github.com/testcontainers/testcontainers-go/modules/rabbitmq"
)

// freePort grabs an ephemeral port and releases it for the container to
// bind. A racing process could steal it; acceptable in tests.
func freePort(t *testing.T) string {
	t.Helper()
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("free port: %v", err)
	}
	defer l.Close()
	return strconv.Itoa(l.Addr().(*net.TCPAddr).Port)
}

type Endpoints struct {
	AMQPURL  string
	MgmtURL  string
	User     string
	Password string
}

var (
	once sync.Once
	eps  Endpoints
	err  error
)

// Fresh starts a dedicated container the test may stop and restart freely
// (chaos tests, R46). Host ports are pinned explicitly: a real broker keeps
// its address across restarts, and random testcontainers mappings do not.
func Fresh(t *testing.T) (e Endpoints, stop, start, terminate func()) {
	t.Helper()
	if testing.Short() {
		t.Skip("integration test: needs Docker, skipped with -short")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 180*time.Second)
	defer cancel()

	amqpPort, mgmtPort := freePort(t), freePort(t)
	ctr, err := tcrabbit.Run(ctx, "rabbitmq:4-management-alpine",
		tcrabbit.WithAdminUsername("guest"),
		tcrabbit.WithAdminPassword("guest"),
		testcontainers.CustomizeRequest(testcontainers.GenericContainerRequest{
			ContainerRequest: testcontainers.ContainerRequest{
				HostConfigModifier: func(hc *container.HostConfig) {
					loopback := netip.MustParseAddr("127.0.0.1")
					hc.PortBindings = network.PortMap{
						network.MustParsePort("5672/tcp"):  {{HostIP: loopback, HostPort: amqpPort}},
						network.MustParsePort("15672/tcp"): {{HostIP: loopback, HostPort: mgmtPort}},
					}
				},
			},
		}),
	)
	if err != nil {
		t.Fatalf("starting rabbitmq container: %v", err)
	}
	amqpURL, err := ctr.AmqpURL(ctx)
	if err != nil {
		t.Fatalf("amqp url: %v", err)
	}
	mgmtURL, err := ctr.HttpURL(ctx)
	if err != nil {
		t.Fatalf("mgmt url: %v", err)
	}
	e = Endpoints{AMQPURL: amqpURL, MgmtURL: mgmtURL, User: "guest", Password: "guest"}
	stop = func() {
		timeout := 10 * time.Second
		if err := ctr.Stop(context.Background(), &timeout); err != nil {
			t.Fatalf("stopping rabbitmq: %v", err)
		}
	}
	start = func() {
		if err := ctr.Start(context.Background()); err != nil {
			t.Fatalf("restarting rabbitmq: %v", err)
		}
	}
	terminate = func() { _ = ctr.Terminate(context.Background()) }
	return e, stop, start, terminate
}

// Get returns a package-scoped RabbitMQ 4.x with the management plugin.
func Get(t *testing.T) Endpoints {
	t.Helper()
	if testing.Short() {
		t.Skip("integration test: needs Docker, skipped with -short")
	}
	once.Do(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 180*time.Second)
		defer cancel()
		ctr, e := tcrabbit.Run(ctx, "rabbitmq:4-management-alpine",
			tcrabbit.WithAdminUsername("guest"),
			tcrabbit.WithAdminPassword("guest"),
		)
		if e != nil {
			err = e
			return
		}
		amqpURL, e := ctr.AmqpURL(ctx)
		if e != nil {
			err = e
			return
		}
		mgmtURL, e := ctr.HttpURL(ctx)
		if e != nil {
			err = e
			return
		}
		eps = Endpoints{AMQPURL: amqpURL, MgmtURL: mgmtURL, User: "guest", Password: "guest"}
	})
	if err != nil {
		t.Fatalf("starting rabbitmq container: %v", err)
	}
	return eps
}
