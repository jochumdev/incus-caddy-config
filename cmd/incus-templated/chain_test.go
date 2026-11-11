package main

import (
	"io"
	"log/slog"
	"testing"

	incusapi "github.com/lxc/incus/v7/shared/api"
	"github.com/stretchr/testify/require"

	"github.com/jochumdev/incus-templated/templated"
)

func TestChainAssemble(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	args := &mainActionArgs{
		Targets: []templated.Target{
			{Label: "caddy", Project: "default", Instance: "caddy"},
		},
		HTTPAddr: ":9153",
	}

	positions := chain(logger, args)
	require.NotEmpty(t, positions)

	// Default assemble should include debounce, enricher, caddy, http.
	plugins, runners, err := assemble(positions, nil)
	require.NoError(t, err)
	require.NotEmpty(t, plugins)
	require.NotEmpty(t, runners)

	names := make([]string, 0, len(plugins))
	for _, p := range plugins {
		names = append(names, p.Name())
	}
	require.Contains(t, names, "debounce")
	require.Contains(t, names, "enricher")
	require.Contains(t, names, "templated")
	require.Contains(t, names, "http")

	// Exclude optional plugin http.
	pluginsNoHTTP, _, err := assemble(positions, []string{"http"})
	require.NoError(t, err)
	namesNoHTTP := make([]string, 0, len(pluginsNoHTTP))
	for _, p := range pluginsNoHTTP {
		namesNoHTTP = append(namesNoHTTP, p.Name())
	}
	require.NotContains(t, namesNoHTTP, "http")

	// Excluding non-optional or unknown plugin fails.
	_, _, err = assemble(positions, []string{"templated"})
	require.ErrorContains(t, err, "cannot exclude \"templated\"")

	_, _, err = assemble(positions, []string{"unknown"})
	require.ErrorContains(t, err, "cannot exclude \"unknown\"")
}

func TestChainServes(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))

	// Nil when projects list is empty.
	require.Nil(t, serves(logger, nil))
	require.Nil(t, serves(logger, []string{}))

	fn := serves(logger, []string{"prod", "staging"})
	require.NotNil(t, fn)
	require.True(t, fn(&incusapi.Project{Name: "prod"}))
	require.True(t, fn(&incusapi.Project{Name: "staging"}))
	require.False(t, fn(&incusapi.Project{Name: "dev"}))
}

func TestChainAssembleWithTrace(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	args := &mainActionArgs{
		Targets: []templated.Target{
			{Label: "caddy", Project: "default", Instance: "caddy"},
		},
		HTTPAddr: ":9153",
		Log:      "TRACE",
	}

	positions := chain(logger, args)
	plugins, _, err := assemble(positions, nil)
	require.NoError(t, err)

	names := make([]string, 0, len(plugins))
	for _, p := range plugins {
		names = append(names, p.Name())
	}
	require.Contains(t, names, "log/arrival")
}

func TestLogAt(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))

	require.Nil(t, logAt(logger, "", "arrival"))
	require.NotEmpty(t, logAt(logger, "DEBUG", "arrival"))
}

func TestChainServesInstance(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))

	// Nil when targets are empty.
	require.Nil(t, servesInstance(logger, nil))
	require.Nil(t, servesInstance(logger, []templated.Target{}))

	targets := []templated.Target{
		{Label: "caddy", Project: "default", Instance: "caddy"},
	}
	osTargets := []templated.Target{
		{Label: "edge", Path: "/tmp/Caddyfile"},
	}

	fn := servesInstance(logger, append(targets, osTargets...))
	require.NotNil(t, fn)

	// Nil instance.
	require.False(t, fn(nil))

	// No labels.
	require.False(t, fn(&incusapi.Instance{Name: "empty"}))

	// Unrelated labels.
	require.False(t, fn(&incusapi.Instance{
		Name: "other",
		InstancePut: incusapi.InstancePut{
			Config: map[string]string{
				"user.other":       "true",
				"user.label.other": "example.com",
			},
		},
	}))

	// Exact target match in Config.
	require.True(t, fn(&incusapi.Instance{
		Name: "web",
		InstancePut: incusapi.InstancePut{
			Config: map[string]string{
				"user.label.caddy": "web.example.com",
			},
		},
	}))

	// Prefix target match in Config.
	require.True(t, fn(&incusapi.Instance{
		Name: "api",
		InstancePut: incusapi.InstancePut{
			Config: map[string]string{
				"user.label.caddy.0.domain": "api.example.com",
			},
		},
	}))

	// OS target match.
	require.True(t, fn(&incusapi.Instance{
		Name: "edge-service",
		InstancePut: incusapi.InstancePut{
			Config: map[string]string{
				"user.label.edge": "edge.example.com",
			},
		},
	}))

	// ExpandedConfig takes precedence over Config.
	require.True(t, fn(&incusapi.Instance{
		Name: "expanded",
		ExpandedConfig: map[string]string{
			"user.label.caddy": "expanded.example.com",
		},
	}))
}
