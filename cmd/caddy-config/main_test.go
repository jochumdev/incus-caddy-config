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
		"--caddy-instance", "external,instance=caddy-prod,project=default,global_template=/etc/caddy/templates/global.caddyfile",
		"--os-path", "path=/etc/caddy/Caddyfile",
		"--os-path", "edge,path=/var/caddy/Caddyfile",
		"--caddyfile-path", "/etc/caddy/Caddyfile",
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
	require.Equal(t, "my-secret-token", cfg.Token)
	require.Equal(t, "/tmp/caddy-data", cfg.DataDir)
	require.Equal(t, "/tmp/caddy-secrets", cfg.SecretsDir)
	require.Equal(t, "/tmp/cert.pem", cfg.ClientCert)
	require.Equal(t, "/tmp/key.pem", cfg.ClientKey)
	require.True(t, cfg.Restricted)
	require.Equal(t, []string{"alpha", "beta"}, cfg.Projects)
	require.Len(t, cfg.Targets, 1)
	require.Equal(t, "external", cfg.Targets[0].Label)
	inst, _ := cfg.Targets[0].Flag("instance")
	require.Equal(t, "caddy-prod", inst)
	proj, _ := cfg.Targets[0].Flag("project")
	require.Equal(t, "default", proj)
	tmpl, _ := cfg.Targets[0].Flag("global_template")
	require.Equal(t, "/etc/caddy/templates/global.caddyfile", tmpl)

	require.Len(t, cfg.OSTargets, 2)
	require.Equal(t, "caddy", cfg.OSTargets[0].Label)
	p0, _ := cfg.OSTargets[0].Flag("path")
	require.Equal(t, "/etc/caddy/Caddyfile", p0)
	require.Equal(t, "edge", cfg.OSTargets[1].Label)
	p1, _ := cfg.OSTargets[1].Flag("path")
	require.Equal(t, "/var/caddy/Caddyfile", p1)
	require.Equal(t, "/etc/caddy/Caddyfile", cfg.CaddyfilePath)
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
		Targets: []caddy.Target{
			caddy.NewTarget("caddy", map[string]string{"project": "default", "instance": "caddy"}),
		},
	}

	err := run(context.Background(), logger, args)
	require.ErrorIs(t, err, incustrust.ErrNoCredentials)
}

func TestRunAssembleError(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	args := &mainActionArgs{
		Targets: []caddy.Target{
			caddy.NewTarget("caddy", map[string]string{"project": "default", "instance": "caddy"}),
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
			caddy.NewTarget("caddy", map[string]string{"project": "default", "instance": "caddy"}),
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
			caddy.NewTarget("caddy", map[string]string{"project": "default", "instance": "caddy"}),
		},
		Log: "INFO",
	}

	err := mainAction(context.Background(), args)
	require.ErrorIs(t, err, incustrust.ErrNoCredentials)
}

func TestRunCommandPluralEnvVars(t *testing.T) {
	t.Setenv("INCUS_CADDY_PROJECTS", "p1,p2")
	t.Setenv("INCUS_CADDY_INSTANCES", "edge,instance=caddy-1,project=default internal,instance=caddy-2,project=default")
	t.Setenv("INCUS_CADDY_OS_PATHS", "path=/etc/caddy/Caddyfile,edge,path=/var/caddy/Caddyfile")
	t.Setenv("INCUS_CADDY_EXCLUDES", "http,debounce")

	cfg := newConfig()
	cmd := runCommand(cfg)

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	_ = cmd.Run(ctx, []string{"run"})

	require.Equal(t, []string{"p1", "p2"}, cfg.Projects)
	require.Len(t, cfg.Targets, 2)
	require.Equal(t, "edge", cfg.Targets[0].Label)
	require.Equal(t, "internal", cfg.Targets[1].Label)
	require.Len(t, cfg.OSTargets, 2)
	require.Equal(t, "caddy", cfg.OSTargets[0].Label)
	require.Equal(t, "edge", cfg.OSTargets[1].Label)
	require.Equal(t, []string{"http", "debounce"}, cfg.Exclude)
}
