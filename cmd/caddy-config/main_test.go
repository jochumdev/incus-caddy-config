package main

import (
	"context"
	"io"
	"log/slog"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/lxc/incus-compose/incustrust"

	"github.com/jochumdev/incus-caddy-config/caddy"
)

func TestVersionCommand(t *testing.T) {
	cmd := command()
	err := cmd.Run(context.Background(), []string{"caddy-config", "version"})
	require.NoError(t, err)
}

func TestRunCommandFlags(t *testing.T) {
	cfg := newConfig()
	cmd := runCommand(cfg)

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	// Simulate CLI flags binding to cfg Destination pointers.
	_ = cmd.Run(ctx, []string{
		"run",
		"--incus", "https://127.0.0.1:8443",
		"--token", "my-secret-token",
		"--data-dir", "/tmp/caddy-data",
		"--secrets-dir", "/tmp/caddy-secrets",
		"--client-cert", "/tmp/cert.pem",
		"--client-key", "/tmp/key.pem",
		"--restricted",
		"--project", "alpha",
		"--project", "beta",
		"--caddy-instance", "external:default:caddy-prod",
		"--os-path", "/etc/caddy/Caddyfile",
		"--os-path", "edge:/var/caddy/Caddyfile",
		"--caddyfile-path", "/etc/caddy/Caddyfile",
		"--templates-dir", "/etc/caddy/templates",
		"--global-template", "caddy:/etc/caddy/templates/global.caddyfile",
		"--global-template", "edge:/etc/caddy/templates/edge.global.caddyfile",
		"--debounce-window", "500ms",
		"--http-address", ":9090",
		"--exclude", "http",
		"--log", "DEBUG",
		"--pprof",
		"--workers", "32",
		"--read-timeout", "15s",
		"--sweep-project-delay", "45s",
		"--sweep-read-delay", "10s",
	})
	// In urfave, Action executes mainAction which attempts to connect to Incus.
	// Since there is no Incus daemon running at https://127.0.0.1:8443 in this unit test,
	// it will error on connection (or ctx cancel).
	// But before that, all flags are parsed and written directly to cfg Destination pointers!

	require.Equal(t, "https://127.0.0.1:8443", cfg.IncusURL)
	require.Equal(t, "my-secret-token", cfg.Token)
	require.Equal(t, "/tmp/caddy-data", cfg.DataDir)
	require.Equal(t, "/tmp/caddy-secrets", cfg.SecretsDir)
	require.Equal(t, "/tmp/cert.pem", cfg.ClientCert)
	require.Equal(t, "/tmp/key.pem", cfg.ClientKey)
	require.True(t, cfg.Restricted)
	require.Equal(t, []string{"alpha", "beta"}, cfg.Projects)
	require.Equal(t, []string{"external:default:caddy-prod"}, cfg.CaddyInstances)
	require.Equal(t, []string{"/etc/caddy/Caddyfile", "edge:/var/caddy/Caddyfile"}, cfg.OSTargets)
	require.Equal(t, "/etc/caddy/Caddyfile", cfg.CaddyfilePath)
	require.Equal(t, "/etc/caddy/templates", cfg.TemplatesDir)
	require.Equal(t, []string{"caddy:/etc/caddy/templates/global.caddyfile", "edge:/etc/caddy/templates/edge.global.caddyfile"}, cfg.GlobalTemplates)
	require.Equal(t, 500*time.Millisecond, cfg.DebounceWindow)
	require.Equal(t, ":9090", cfg.HTTPAddr)
	require.Equal(t, []string{"http"}, cfg.Exclude)
	require.Equal(t, "DEBUG", cfg.Log)
	require.True(t, cfg.Pprof)
	require.Equal(t, 32, cfg.Workers)
	require.Equal(t, 15*time.Second, cfg.ReadTimeout)
	require.Equal(t, 45*time.Second, cfg.ProjectDelay)
	require.Equal(t, 10*time.Second, cfg.ReadDelay)
}

func TestDrainContext(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	dCtx := drainContext(ctx)
	// Must not be canceled even though parent was canceled.
	require.Nil(t, dCtx.Err())

	deadline, ok := dCtx.Deadline()
	require.True(t, ok)
	require.True(t, time.Until(deadline) > 0)
	require.True(t, time.Until(deadline) <= drainTimeout)
}

func TestRunNoCredentials(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	args := &mainActionArgs{
		Targets: []caddy.Target{
			{Label: "caddy", Project: "default", Instance: "caddy"},
		},
	}

	err := run(context.Background(), logger, args)
	require.ErrorIs(t, err, incustrust.ErrNoCredentials)
}

func TestRunAssembleError(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	args := &mainActionArgs{
		Targets: []caddy.Target{
			{Label: "caddy", Project: "default", Instance: "caddy"},
		},
		Exclude: []string{"caddy"},
	}

	err := run(context.Background(), logger, args)
	require.ErrorContains(t, err, "cannot exclude \"caddy\"")
}

func TestRunContextCanceled(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	args := &mainActionArgs{
		Targets: []caddy.Target{
			{Label: "caddy", Project: "default", Instance: "caddy"},
		},
		IncusURL: "https://127.0.0.1:1",
		Token:    "dummy-token",
	}

	err := run(ctx, logger, args)
	require.Error(t, err)
	require.ErrorContains(t, err, "connecting to Incus")
}

func TestRunCommandValidationFailure(t *testing.T) {
	cfg := newConfig()
	cmd := runCommand(cfg)

	// Running without --caddy-instance or --os-path must fail validation.
	err := cmd.Run(context.Background(), []string{"run"})
	require.ErrorContains(t, err, "at least one --caddy-instance or --os-path must be specified")
}

func TestMainAction(t *testing.T) {
	args := &mainActionArgs{
		Targets: []caddy.Target{
			{Label: "caddy", Project: "default", Instance: "caddy"},
		},
		Log: "INFO",
	}

	err := mainAction(context.Background(), args)
	require.ErrorIs(t, err, incustrust.ErrNoCredentials)
}
