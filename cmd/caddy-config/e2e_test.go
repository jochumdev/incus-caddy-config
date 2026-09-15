package main

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func skipE2E(t *testing.T) {
	t.Helper()

	if os.Getenv("TEST_LOCAL") != "" || os.Getenv("INCUS_CADDY_TEST_LOCAL") != "" || os.Getenv("INCUS_COMPOSE_TEST_LOCAL") != "" {
		t.Skip("skipping E2E test in local mode")
	}

	if os.Getenv("TEST_E2E") == "" && os.Getenv("INCUS_CADDY_TEST_E2E") == "" && os.Getenv("INCUS_COMPOSE_TEST_E2E") == "" {
		t.Skip("skipping E2E test: run with just test-e2e or set TEST_E2E=1")
	}
}

func repoRoot(t *testing.T) string {
	t.Helper()

	cmd := exec.CommandContext(context.Background(), "git", "rev-parse", "--show-toplevel")
	out, err := cmd.Output()
	require.NoError(t, err)

	return strings.TrimSpace(string(out))
}

func incusRemote() string {
	remote := os.Getenv("INCUS_REMOTE")
	if remote != "" {
		return remote
	}

	return "ict-daily-dev01-main"
}

func runCompose(ctx context.Context, t *testing.T, project, fixture string, args ...string) (string, error) {
	t.Helper()

	cmdArgs := []string{
		"--remote", incusRemote(),
		"--project-name", project,
		"--file", fixture,
	}
	cmdArgs = append(cmdArgs, args...)

	cmd := exec.CommandContext(ctx, "incus-compose", cmdArgs...)
	cmd.Env = append(os.Environ(), "INCUS_REMOTE="+incusRemote())

	out, err := cmd.CombinedOutput()

	return string(out), err
}

func TestE2ECaddyReverseProxy(t *testing.T) {
	skipE2E(t)

	ctx := t.Context()
	project := fmt.Sprintf("test-caddy-e2e-%d", time.Now().UnixNano()%1000000)
	fixture := filepath.Join(repoRoot(t), "test", "fixtures", "e2e", "compose.yaml")

	t.Cleanup(func() {
		downCtx, downCancel := context.WithTimeout(context.Background(), 2*time.Minute)
		defer downCancel()

		_, _ = runCompose(downCtx, t, project, fixture, "down", "--project")
	})

	t.Logf("deploying fixture stack to project %s on remote %s", project, incusRemote())
	out, err := runCompose(ctx, t, project, fixture, "up", "-d")
	require.NoError(t, err, "incus-compose up failed: %s", out)

	configCtx, configCancel := context.WithCancel(ctx)
	defer configCancel()

	cfg := newConfig()
	cfg.CaddyInstances = []string{fmt.Sprintf("edge:%s:caddy", project)}
	cfg.Projects = []string{project}
	cfg.Remote = incusRemote()
	cfg.UseRemote = true
	cfg.Log = "DEBUG"
	cfg.DebounceWindow = 100 * time.Millisecond
	cfg.ReadDelay = 500 * time.Millisecond
	cfg.ProjectDelay = 1 * time.Second

	args, err := cfg.validate()
	require.NoError(t, err)

	runDone := make(chan struct{})
	go func() {
		defer close(runDone)
		_ = mainAction(configCtx, args)
	}()

	defer func() {
		configCancel()
		<-runDone
	}()

	// 1. Verify Caddyfile is deployed with reverse_proxy and both api upstreams.
	t.Log("waiting for caddy-config to deploy Caddyfile with both upstreams")
	require.Eventually(t, func() bool {
		caddyfile, readErr := runCompose(ctx, t, project, fixture, "exec", "-T", "caddy", "cat", "/config/Caddyfile")
		if readErr != nil {
			t.Logf("reading Caddyfile failed: %v, output: %s", readErr, caddyfile)

			return false
		}

		t.Logf("current Caddyfile:\n%s", caddyfile)

		return strings.Contains(caddyfile, "api.example.test") && strings.Count(caddyfile, ":8080") == 2
	}, 45*time.Second, 2*time.Second, "Caddyfile was not reconciled with two upstreams")

	// 2. Verify HTTP reverse proxying works and reaches a busybox API server.
	t.Log("verifying HTTP reverse proxying through Caddy")
	require.Eventually(t, func() bool {
		resp, httpErr := runCompose(ctx, t, project, fixture, "exec", "-T", "caddy",
			"wget", "-q", "-O", "-", "--header", "Host: api.example.test", "http://127.0.0.1:80/")
		if httpErr != nil {
			t.Logf("HTTP query failed: %v, output: %s", httpErr, resp)

			return false
		}

		body := strings.TrimSpace(resp)

		return body == "api1-ok" || body == "api2-ok"
	}, 20*time.Second, time.Second, "HTTP request through Caddy failed")

	// 3. Scale-down: Stop api1. Caddyfile should drop api1 and keep only api2.
	t.Log("stopping api1 to test dynamic scale-down")
	out, err = runCompose(ctx, t, project, fixture, "stop", "api1")
	require.NoError(t, err, "stopping api1 failed: %s", out)

	t.Log("waiting for caddy-config to reconcile to single upstream api2")
	require.Eventually(t, func() bool {
		caddyfile, readErr := runCompose(ctx, t, project, fixture, "exec", "-T", "caddy", "cat", "/config/Caddyfile")
		if readErr != nil {
			return false
		}

		return strings.Contains(caddyfile, "api.example.test") && strings.Count(caddyfile, ":8080") == 1
	}, 45*time.Second, time.Second, "Caddyfile was not updated to 1 upstream after stopping api1")

	// Verify only api2 answers now.
	require.Eventually(t, func() bool {
		resp, httpErr := runCompose(ctx, t, project, fixture, "exec", "-T", "caddy",
			"wget", "-q", "-O", "-", "--header", "Host: api.example.test", "http://127.0.0.1:80/")
		if httpErr != nil {
			return false
		}

		return strings.TrimSpace(resp) == "api2-ok"
	}, 15*time.Second, time.Second, "traffic did not route exclusively to api2")

	// 4. Scale-up: Restart api1. Caddyfile should restore both upstreams.
	t.Log("starting api1 to test dynamic scale-up")
	out, err = runCompose(ctx, t, project, fixture, "start", "api1")
	require.NoError(t, err, "starting api1 failed: %s", out)

	t.Log("waiting for caddy-config to reconcile back to two upstreams")
	require.Eventually(t, func() bool {
		caddyfile, readErr := runCompose(ctx, t, project, fixture, "exec", "-T", "caddy", "cat", "/config/Caddyfile")
		if readErr != nil {
			return false
		}

		return strings.Contains(caddyfile, "api.example.test") && strings.Count(caddyfile, ":8080") == 2
	}, 45*time.Second, time.Second, "Caddyfile was not restored to 2 upstreams after restarting api1")

	// 5. Volume persistence across reboots: Restart Caddy and verify it still proxies traffic.
	t.Log("restarting Caddy to test volume persistence across reboots")
	out, err = runCompose(ctx, t, project, fixture, "restart", "caddy")
	require.NoError(t, err, "restarting caddy failed: %s", out)

	require.Eventually(t, func() bool {
		resp, httpErr := runCompose(ctx, t, project, fixture, "exec", "-T", "caddy",
			"wget", "-q", "-O", "-", "--header", "Host: api.example.test", "http://127.0.0.1:80/")
		if httpErr != nil {
			return false
		}

		body := strings.TrimSpace(resp)

		return body == "api1-ok" || body == "api2-ok"
	}, 15*time.Second, time.Second, "traffic failed after Caddy container restart")
}
