package integration

import (
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/go-chi/chi/v5"
	"github.com/stretchr/testify/require"

	"myapp/internal/platform/rabbit/rabbittest"
)

// R41: every public chi route must exist in the generated OpenAPI spec.
// Catches undocumented AND deleted routes; a wrong @Success body it cannot
// catch — the accepted limitation of code-first docs (PRD §7.12). Runs in
// the integration package because building the real router needs live Deps;
// `task check` still catches STALE generated files via generate:verify.
func TestEveryRouteIsDocumented(t *testing.T) {
	eps := rabbittest.Get(t)
	p := startPipeline(t, eps.AMQPURL, eps.MgmtURL)

	spec := loadSpec(t)
	router := p.router

	err := chi.Walk(router, func(method, route string, _ http.Handler, _ ...func(http.Handler) http.Handler) error {
		if isInternalRoute(route) {
			return nil
		}
		norm := normalizeRoute(route)
		require.Truef(t, spec.has(method, norm),
			"route %s %s has no swagger annotation — document the handler or delete the route (R41)", method, norm)
		return nil
	})
	require.NoError(t, err)
}

func isInternalRoute(route string) bool {
	return strings.HasPrefix(route, "/health") ||
		strings.Contains(route, "/internal/") // module-to-module surface, not public API
}

// normalizeRoute strips the mount prefix (spec BasePath is /api/v1) and
// chi's trailing-slash artifacts.
func normalizeRoute(route string) string {
	r := strings.TrimPrefix(route, "/api/v1")
	r = strings.TrimSuffix(r, "/")
	if r == "" {
		r = "/"
	}
	return r
}

type spec struct {
	Paths map[string]map[string]json.RawMessage `json:"paths"`
}

func (s spec) has(method, route string) bool {
	ops, ok := s.Paths[route]
	if !ok {
		return false
	}
	_, ok = ops[strings.ToLower(method)]
	return ok
}

func loadSpec(t *testing.T) spec {
	t.Helper()
	b, err := os.ReadFile(filepath.Join(repoRootDir(t), "api", "docs", "swagger.json"))
	require.NoError(t, err, "run `task docs` first")
	var s spec
	require.NoError(t, json.Unmarshal(b, &s))
	return s
}

func repoRootDir(t *testing.T) string {
	t.Helper()
	dir, err := os.Getwd()
	require.NoError(t, err)
	for {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return dir
		}
		parent := filepath.Dir(dir)
		require.NotEqual(t, dir, parent, "go.mod not found")
		dir = parent
	}
}
