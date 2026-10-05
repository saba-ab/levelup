package app

import (
	"context"

	"myapp/internal/platform/rabbit"
)

// NewTopology builds the topology handle from config. Only binaries touch
// this (dispatcher, worker, scheduler) — modules never see the broker.
func (a *App) NewTopology(conn *rabbit.Conn) *rabbit.Topology {
	return rabbit.NewTopology(conn, rabbit.MgmtConfig{
		URL:      a.P.Cfg.Rabbit.MgmtURL,
		User:     a.P.Cfg.Rabbit.MgmtUser,
		Password: a.P.Cfg.Rabbit.MgmtPass,
	}, a.P.Tel.Log)
}

// DeclareTopology provisions every queue the enabled modules need —
// subscriptions and jobs — then verifies the vhost is quorum-only (R45).
// Both dispatcher and worker run this at boot so bindings exist before the
// first publish, whichever binary starts first.
func (a *App) DeclareTopology(ctx context.Context, topo *rabbit.Topology) error {
	if err := topo.DeclareCore(ctx); err != nil {
		return err
	}
	for _, m := range a.Modules {
		for _, s := range m.Subscriptions() {
			if err := topo.DeclareEventQueue(ctx, s.Group, s.Topic); err != nil {
				return err
			}
		}
		for _, j := range m.Jobs() {
			if err := topo.DeclareJobQueue(ctx, j.Name); err != nil {
				return err
			}
		}
	}
	return topo.Verify(ctx)
}
