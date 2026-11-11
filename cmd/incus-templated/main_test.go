package main

import (
	"context"
	"io"
	"log/slog"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/lxc/incus-compose/incustrust"

	"github.com/jochumdev/incus-templated/templated"
)

func TestVersionCommand(t *testing.T) {
	cmd := command()
	err := cmd.Run(context.Background(), []string{"incus-templated", "version"})
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
		"--server-fingerprint", "a1b2c3d4e5f60718293a4b5c6d7e8f90a1b2c3d4e5f60718293a4b5c6d7e8f90",
		"--token", "my-secret-token",
		"--data-dir", "/tmp/caddy-data",
		"--secrets-dir", "/tmp/caddy-secrets",
		"--client-cert", "/tmp/cert.pem",
		"--client-key", "/tmp/key.pem",
		"--restricted",
		"--project", "alpha",
		"--project", "beta",
		"--target", "external,preset=caddy,instance=caddy-prod,project=default,tier=public",
		"--target", "edge,path=/var/caddy/Caddyfile,template=edge.tmpl",
		"--config", "/etc/incus-templated/config.yaml",
		"--templates-dir", "/etc/caddy/templates",
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
	require.Equal(t, "a1b2c3d4e5f60718293a4b5c6d7e8f90a1b2c3d4e5f60718293a4b5c6d7e8f90", cfg.ServerFingerprint)
	require.Equal(t, "my-secret-token", cfg.Token)
	require.Equal(t, "/tmp/caddy-data", cfg.DataDir)
	require.Equal(t, "/tmp/caddy-secrets", cfg.SecretsDir)
	require.Equal(t, "/tmp/cert.pem", cfg.ClientCert)
	require.Equal(t, "/tmp/key.pem", cfg.ClientKey)
	require.True(t, cfg.Restricted)
	require.Equal(t, []string{"alpha", "beta"}, cfg.Projects)
	require.Len(t, cfg.Targets, 2)
	require.Equal(t, "external", cfg.Targets[0].Label)
	require.Equal(t, "caddy", cfg.Targets[0].Preset)
	require.Equal(t, "caddy-prod", cfg.Targets[0].Instance)
	require.Equal(t, "default", cfg.Targets[0].Project)
	require.Equal(t, map[string]string{"tier": "public"}, cfg.Targets[0].Flags)
	require.Equal(t, "edge", cfg.Targets[1].Label)
	require.Equal(t, "/var/caddy/Caddyfile", cfg.Targets[1].Path)
	require.Equal(t, "edge.tmpl", cfg.Targets[1].Template)
	require.Equal(t, "/etc/incus-templated/config.yaml", cfg.ConfigFile)
	require.Equal(t, "/etc/caddy/templates", cfg.TemplatesDir)
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
		Targets: []templated.Target{
			{Label: "caddy", Project: "default", Instance: "caddy"},
		},
	}

	err := run(context.Background(), logger, args)
	require.ErrorIs(t, err, incustrust.ErrNoCredentials)
}

func TestRunAssembleError(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	args := &mainActionArgs{
		Targets: []templated.Target{
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
		Targets: []templated.Target{
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

	// Running without any --target must fail validation.
	err := cmd.Run(context.Background(), []string{"run"})
	require.ErrorContains(t, err, "at least one --target must be specified")
}

func TestMainAction(t *testing.T) {
	args := &mainActionArgs{
		Targets: []templated.Target{
			{Label: "caddy", Project: "default", Instance: "caddy"},
		},
		Log: "INFO",
	}

	err := mainAction(context.Background(), args)
	require.ErrorIs(t, err, incustrust.ErrNoCredentials)
}

func TestRunCommandPluralEnvVars(t *testing.T) {
	t.Setenv("INCUS_TEMPLATED_SERVER_FINGERPRINT", "fp-from-env")
	t.Setenv("INCUS_TEMPLATED_PROJECTS", "p1,p2")
	t.Setenv("INCUS_TEMPLATED_TARGETS", "edge,instance=caddy-1,project=default internal,path=/etc/caddy/Caddyfile")
	t.Setenv("INCUS_TEMPLATED_CONFIG", "/etc/incus-templated/config.yaml")
	t.Setenv("INCUS_TEMPLATED_EXCLUDES", "http,debounce")

	cfg := newConfig()
	cmd := runCommand(cfg)

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	_ = cmd.Run(ctx, []string{"run"})

	require.Equal(t, "fp-from-env", cfg.ServerFingerprint)
	require.Equal(t, []string{"p1", "p2"}, cfg.Projects)
	require.Len(t, cfg.Targets, 2)
	require.Equal(t, "edge", cfg.Targets[0].Label)
	require.Equal(t, "internal", cfg.Targets[1].Label)
	require.Equal(t, "/etc/incus-templated/config.yaml", cfg.ConfigFile)
	require.Equal(t, []string{"http", "debounce"}, cfg.Exclude)
}
