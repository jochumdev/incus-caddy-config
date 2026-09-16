package main

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/jochumdev/incus-caddy-config/caddy"
)

func TestConfigDefaults(t *testing.T) {
	cfg := newConfig()
	require.Equal(t, ":9153", cfg.HTTPAddr)
	require.Equal(t, "/var/lib/caddy-config", cfg.DataDir)
	require.Equal(t, "/run/secrets", cfg.SecretsDir)
	require.Equal(t, "/config/Caddyfile", cfg.CaddyfilePath)
	require.Equal(t, 250*time.Millisecond, cfg.DebounceWindow)
}

func TestConfigValidate(t *testing.T) {
	cfg := newConfig()
	_, err := cfg.validate()
	require.ErrorContains(t, err, "at least one --caddy-instance or --os-path must be specified")

	// Valid OS target with path flag.
	cfg.OSTargets = []caddy.Target{
		caddy.NewTarget("caddy", map[string]string{"path": "/etc/caddy/Caddyfile"}),
		caddy.NewTarget("custom", map[string]string{"path": "/var/caddy/Caddyfile"}),
	}
	args, err := cfg.validate()
	require.NoError(t, err)
	require.Empty(t, args.Targets)
	require.Len(t, args.OSTargets, 2)
	require.Equal(t, "caddy", args.OSTargets[0].Label)
	p0, _ := args.OSTargets[0].Flag("path")
	require.Equal(t, "/etc/caddy/Caddyfile", p0)
	require.Equal(t, "custom", args.OSTargets[1].Label)
	p1, _ := args.OSTargets[1].Flag("path")
	require.Equal(t, "/var/caddy/Caddyfile", p1)

	// Both Targets and OSTargets.
	cfg.Targets = []caddy.Target{
		caddy.NewTarget("external", map[string]string{"project": "default", "instance": "caddy-prod"}),
		caddy.NewTarget("internal", map[string]string{"project": "infra", "instance": "caddy-dev"}),
	}
	args, err = cfg.validate()
	require.NoError(t, err)
	require.Len(t, args.Targets, 2)
	require.Equal(t, "external", args.Targets[0].Label)
	proj0, _ := args.Targets[0].Flag("project")
	require.Equal(t, "default", proj0)
	inst0, _ := args.Targets[0].Flag("instance")
	require.Equal(t, "caddy-prod", inst0)
	require.Equal(t, "internal", args.Targets[1].Label)
	proj1, _ := args.Targets[1].Flag("project")
	require.Equal(t, "infra", proj1)
	inst1, _ := args.Targets[1].Flag("instance")
	require.Equal(t, "caddy-dev", inst1)
	require.Len(t, args.OSTargets, 2)

	// Missing instance in Targets.
	cfgBadTarget := newConfig()
	cfgBadTarget.Targets = []caddy.Target{
		caddy.NewTarget("caddy", map[string]string{"project": "default"}),
	}
	_, err = cfgBadTarget.validate()
	require.ErrorContains(t, err, "missing required flag \"instance\"")

	// Missing project in Targets.
	cfgBadTarget = newConfig()
	cfgBadTarget.Targets = []caddy.Target{
		caddy.NewTarget("caddy", map[string]string{"instance": "caddy"}),
	}
	_, err = cfgBadTarget.validate()
	require.ErrorContains(t, err, "missing required flag \"project\"")

	// Missing path in OSTargets.
	cfgBadOS := newConfig()
	cfgBadOS.OSTargets = []caddy.Target{
		caddy.NewTarget("caddy", map[string]string{}),
	}
	_, err = cfgBadOS.validate()
	require.ErrorContains(t, err, "missing required flag \"path\"")
}

func TestConfigEndpoint(t *testing.T) {
	cfg := newConfig()
	cfg.Targets = []caddy.Target{caddy.NewTarget("caddy", map[string]string{"project": "default", "instance": "caddy"})}
	args, err := cfg.validate()
	require.NoError(t, err)
	require.Empty(t, args.endpoint())

	args.IncusURL = "https://127.0.0.1:8443"
	require.Equal(t, "https://127.0.0.1:8443", args.endpoint())

	args.IncusURL = ""
	args.UseRemote = true
	require.Equal(t, "remote:default", args.endpoint())

	args.Remote = "my-cluster"
	require.Equal(t, "remote:my-cluster", args.endpoint())
}

func TestConfigRedactedToken(t *testing.T) {
	cfg := newConfig()
	cfg.Targets = []caddy.Target{caddy.NewTarget("caddy", map[string]string{"project": "default", "instance": "caddy"})}
	args, err := cfg.validate()
	require.NoError(t, err)
	require.Empty(t, args.redactedToken())

	args.Token = "secret-token"
	require.Equal(t, "<redacted-(12)>", args.redactedToken())
}
