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

	// Valid OS target with bare path and explicit prefix.
	cfg.OSTargets = []caddy.Target{
		{Label: "caddy", Path: "/etc/caddy/Caddyfile"},
		{Label: "custom", Path: "/var/caddy/Caddyfile"},
	}
	args, err := cfg.validate()
	require.NoError(t, err)
	require.Empty(t, args.Targets)
	require.Len(t, args.OSTargets, 2)
	require.Equal(t, "caddy", args.OSTargets[0].Label)
	require.Equal(t, "/etc/caddy/Caddyfile", args.OSTargets[0].Path)
	require.Equal(t, "custom", args.OSTargets[1].Label)
	require.Equal(t, "/var/caddy/Caddyfile", args.OSTargets[1].Path)

	// Both Targets and OSTargets.
	cfg.Targets = []caddy.Target{
		{Label: "external", Project: "default", Instance: "caddy-prod"},
		{Label: "internal", Project: "infra", Instance: "caddy-dev"},
	}
	args, err = cfg.validate()
	require.NoError(t, err)
	require.Len(t, args.Targets, 2)
	require.Equal(t, "external", args.Targets[0].Label)
	require.Equal(t, "default", args.Targets[0].Project)
	require.Equal(t, "caddy-prod", args.Targets[0].Instance)
	require.Equal(t, "internal", args.Targets[1].Label)
	require.Equal(t, "infra", args.Targets[1].Project)
	require.Equal(t, "caddy-dev", args.Targets[1].Instance)
	require.Len(t, args.OSTargets, 2)

	// Custom global templates with prefix and duplicate overwrite (last one wins).
	cfg.GlobalTemplates = []string{
		"caddy-external:/etc/caddy/ext.caddyfile",
		"internal:{ admin localhost:2019 }",
		"caddy-external:/etc/caddy/override.caddyfile",
	}
	args, err = cfg.validate()
	require.NoError(t, err)
	require.Len(t, args.GlobalTemplates, 2)
	require.Equal(t, "/etc/caddy/override.caddyfile", args.GlobalTemplates["caddy-external"])
	require.Equal(t, "{ admin localhost:2019 }", args.GlobalTemplates["internal"])

	// Missing prefix must error.
	cfg.GlobalTemplates = []string{"/etc/caddy/bare.caddyfile"}
	_, err = cfg.validate()
	require.Error(t, err)
}

func TestConfigEndpoint(t *testing.T) {
	cfg := newConfig()
	cfg.Targets = []caddy.Target{{Label: "caddy", Project: "default", Instance: "caddy"}}
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
	cfg.Targets = []caddy.Target{{Label: "caddy", Project: "default", Instance: "caddy"}}
	args, err := cfg.validate()
	require.NoError(t, err)
	require.Empty(t, args.redactedToken())

	args.Token = "secret-token"
	require.Equal(t, "<redacted-(12)>", args.redactedToken())
}
