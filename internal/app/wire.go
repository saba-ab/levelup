//go:build wireinject
// +build wireinject

package app

import (
	"context"

	"github.com/google/wire"

	"myapp/internal/config"
)

// InitializePlatform builds everything below Deps (PRD §7.2): Wire owns the
// infrastructure graph and the reverse-order cleanup chain; module selection
// stays hand-written in registry.go.
func InitializePlatform(ctx context.Context, cfg config.Config) (*Platform, func(), error) {
	wire.Build(
		provideTelemetry,
		provideDB,
		provideGorm,
		provideRedisCore,
		provideRedisCache,
		provideBus,
		provideOutbox,
		provideAuthn,
		provideIdempotency,
		provideLimiter,
		provideEnforcer,
		provideClock,
		provideValidator,
		wire.Struct(new(Platform), "*"),
	)
	return nil, nil, nil
}
