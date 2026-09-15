package main

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"
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
	require.ErrorContains(t, err, "at least one --caddy-instance must be specified")

	cfg.CaddyInstances = []string{"invalid-target"}
	_, err = cfg.validate()
	require.ErrorContains(t, err, "invalid --caddy-instance")

	cfg.CaddyInstances = []string{"external:default:caddy-prod", "internal:infra:caddy-dev"}
	args, err := cfg.validate()
	require.NoError(t, err)
	require.Len(t, args.Targets, 2)
	require.Equal(t, "external", args.Targets[0].Label)
	require.Equal(t, "default", args.Targets[0].Project)
	require.Equal(t, "caddy-prod", args.Targets[0].Instance)
	require.Equal(t, "internal", args.Targets[1].Label)
	require.Equal(t, "infra", args.Targets[1].Project)
	require.Equal(t, "caddy-dev", args.Targets[1].Instance)
}

func TestConfigEndpoint(t *testing.T) {
	cfg := newConfig()
	cfg.CaddyInstances = []string{"caddy:default:caddy"}
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
	cfg.CaddyInstances = []string{"caddy:default:caddy"}
	args, err := cfg.validate()
	require.NoError(t, err)
	require.Empty(t, args.redactedToken())

	args.Token = "secret-token"
	require.Equal(t, "<redacted-(12)>", args.redactedToken())
}
