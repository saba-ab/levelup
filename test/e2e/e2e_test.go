// Package e2e drives the REAL binaries — cmd/migrate, cmd/api,
// cmd/dispatcher, cmd/worker — against the compose stack, through the public
// HTTP API only. No internal imports, no test doubles: if this passes, the
// thing a developer runs with `task run` works.
//
// The template ships no business modules, so the journey is a boot smoke:
// migrations apply, every binary starts, /health answers with the enabled
// module set. Add your product journey next to it once a module exists;
// docs/examples.md keeps the shape the reference journey had (register →
// login → event-driven reaction → idempotent write → 403 / 401).
//
// It skips unless E2E=1 (needs `task docker:up` and free ports), so it never
// blocks `task test` or a laptop without the stack running.
package e2e

import (
	"context"
	"encoding/json"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

type stack struct {
	baseURL string
	client  *http.Client
}

func TestBinariesBootAndReportHealth(t *testing.T) {
	if os.Getenv("E2E") != "1" {
		t.Skip("e2e: set E2E=1 with the compose stack up (task docker:up)")
	}
	s := bootStack(t)

	var health struct {
		Status  string            `json:"status"`
		Modules map[string]string `json:"modules"`
	}
	resp, err := s.client.Get(s.baseURL + "/health")
	require.NoError(t, err)
	defer resp.Body.Close()
	require.Equal(t, 200, resp.StatusCode)
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&health))
	require.Equal(t, "ok", health.Status)
	for name, status := range health.Modules {
		require.Equal(t, "ok", status, "module %s must report ready (R2)", name)
	}
}

// --- harness ---

func bootStack(t *testing.T) *stack {
	t.Helper()
	root := repoRoot(t)
	env := loadEnv(t, root)

	run(t, root, env, "go", "run", "./cmd/migrate", "up")

	apiPort := freePort(t)
	adminPort := freePort(t)
	apiEnv := append(append([]string{}, env...),
		"HTTP_ADDR=:"+apiPort, "HTTP_ADMIN_ADDR=:"+adminPort)

	start(t, root, apiEnv, "./cmd/api")
	start(t, root, append(append([]string{}, env...),
		"HTTP_ADMIN_ADDR=:"+freePort(t)), "./cmd/dispatcher")
	start(t, root, append(append([]string{}, env...),
		"HTTP_ADMIN_ADDR=:"+freePort(t)), "./cmd/worker")

	s := &stack{baseURL: "http://localhost:" + apiPort, client: &http.Client{Timeout: 10 * time.Second}}
	require.Eventually(t, func() bool {
		resp, err := s.client.Get(s.baseURL + "/health")
		if err != nil {
			return false
		}
		defer resp.Body.Close()
		return resp.StatusCode == 200
	}, 60*time.Second, 500*time.Millisecond, "api never became healthy")
	return s
}

func run(t *testing.T, dir string, env []string, name string, args ...string) {
	t.Helper()
	cmd := exec.Command(name, args...)
	cmd.Dir = dir
	cmd.Env = append(os.Environ(), env...)
	out, err := cmd.CombinedOutput()
	require.NoErrorf(t, err, "%s %v failed: %s", name, args, out)
}

// start launches a long-running binary. It builds first and runs the BINARY,
// not `go run`: `go run` spawns a child that survives the parent's signal,
// leaving the port held after the test ends. Output goes to a log file the
// test names on failure — discarding it makes boot failures invisible.
func start(t *testing.T, dir string, env []string, pkg string) {
	t.Helper()
	bin := filepath.Join(t.TempDir(), filepath.Base(pkg))
	build := exec.Command("go", "build", "-o", bin, pkg)
	build.Dir = dir
	if out, err := build.CombinedOutput(); err != nil {
		t.Fatalf("building %s: %v\n%s", pkg, err, out)
	}

	logPath := filepath.Join(t.TempDir(), filepath.Base(pkg)+".log")
	logFile, err := os.Create(logPath)
	require.NoError(t, err)

	ctx, cancel := context.WithCancel(context.Background())
	cmd := exec.CommandContext(ctx, bin)
	cmd.Dir = dir
	cmd.Env = append(os.Environ(), env...)
	cmd.Stdout = logFile
	cmd.Stderr = logFile
	require.NoError(t, cmd.Start())

	t.Cleanup(func() {
		cancel()
		_ = cmd.Wait()
		_ = logFile.Close()
		if t.Failed() {
			if b, err := os.ReadFile(logPath); err == nil {
				t.Logf("--- %s log ---\n%s", pkg, b)
			}
		}
	})
}

func loadEnv(t *testing.T, root string) []string {
	t.Helper()
	b, err := os.ReadFile(filepath.Join(root, ".env"))
	require.NoError(t, err, "e2e needs .env (cp .env.example .env)")
	var env []string
	for line := range strings.Lines(string(b)) {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		env = append(env, line)
	}
	return env
}

func freePort(t *testing.T) string {
	t.Helper()
	l, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	defer l.Close()
	_, port, err := net.SplitHostPort(l.Addr().String())
	require.NoError(t, err)
	return port
}

func repoRoot(t *testing.T) string {
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
