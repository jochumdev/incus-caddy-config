package main

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/urfave/cli/v3"

	"github.com/jochumdev/incus-caddy-config/caddy"
)

func TestTargetFlagCLI(t *testing.T) {
	var targets []caddy.Target
	var cmdTargets []caddy.Target
	cmd := &cli.Command{
		Name: "test",
		Flags: []cli.Flag{
			&TargetFlag{
				Name:        "caddy-instance",
				Destination: &targets,
			},
		},
		Action: func(_ context.Context, c *cli.Command) error {
			if v, ok := c.Value("caddy-instance").([]caddy.Target); ok {
				cmdTargets = v
			}

			return nil
		},
	}

	err := cmd.Run(context.Background(), []string{
		"test",
		"--caddy-instance", "caddy1:proj1:inst1",
	})
	require.NoError(t, err)
	require.Equal(t, []caddy.Target{
		{Label: "caddy1", Project: "proj1", Instance: "inst1"},
	}, targets)
	require.Equal(t, targets, cmdTargets)
}

func TestTargetFlagRepeated(t *testing.T) {
	var targets []caddy.Target
	cmd := &cli.Command{
		Name: "test",
		Flags: []cli.Flag{
			&TargetFlag{
				Name:        "caddy-instance",
				Destination: &targets,
			},
		},
		Action: func(_ context.Context, _ *cli.Command) error {
			return nil
		},
	}

	err := cmd.Run(context.Background(), []string{
		"test",
		"--caddy-instance", "caddy1:proj1:inst1",
		"--caddy-instance", "caddy2:proj2:inst2",
	})
	require.NoError(t, err)
	require.Equal(t, []caddy.Target{
		{Label: "caddy1", Project: "proj1", Instance: "inst1"},
		{Label: "caddy2", Project: "proj2", Instance: "inst2"},
	}, targets)
}

func TestTargetFlagCommaSeparated(t *testing.T) {
	var targets []caddy.Target
	cmd := &cli.Command{
		Name: "test",
		Flags: []cli.Flag{
			&TargetFlag{
				Name:        "caddy-instance",
				Destination: &targets,
			},
		},
		Action: func(_ context.Context, _ *cli.Command) error {
			return nil
		},
	}

	err := cmd.Run(context.Background(), []string{
		"test",
		"--caddy-instance", "caddy1:proj1:inst1,caddy2:proj2:inst2",
	})
	require.NoError(t, err)
	require.Equal(t, []caddy.Target{
		{Label: "caddy1", Project: "proj1", Instance: "inst1"},
		{Label: "caddy2", Project: "proj2", Instance: "inst2"},
	}, targets)
}

func TestTargetFlagWithFlags(t *testing.T) {
	var targets []caddy.Target
	cmd := &cli.Command{
		Name: "test",
		Flags: []cli.Flag{
			&TargetFlag{
				Name:        "caddy-instance",
				Destination: &targets,
			},
		},
		Action: func(_ context.Context, _ *cli.Command) error {
			return nil
		},
	}

	err := cmd.Run(context.Background(), []string{
		"test",
		"--caddy-instance", "caddy1:proj1:inst1,flag1=val1,flag2=val2",
	})
	require.NoError(t, err)
	require.Equal(t, []caddy.Target{
		{
			Label:    "caddy1",
			Project:  "proj1",
			Instance: "inst1",
			Flags:    map[string]string{"flag1": "val1", "flag2": "val2"},
		},
	}, targets)
}

func TestTargetFlagInvalid(t *testing.T) {
	var targets []caddy.Target
	cmd := &cli.Command{
		Name: "test",
		Flags: []cli.Flag{
			&TargetFlag{
				Name:        "caddy-instance",
				Destination: &targets,
			},
		},
		Action: func(_ context.Context, _ *cli.Command) error {
			return nil
		},
	}

	err := cmd.Run(context.Background(), []string{
		"test",
		"--caddy-instance", "invalid-target",
	})
	require.Error(t, err)
	require.ErrorContains(t, err, "invalid target")
}

func TestTargetFlagEnvVar(t *testing.T) {
	t.Setenv("TEST_CADDY_INSTANCES", "edge:p1:caddy1,flag=1 internal:p2:caddy2")

	var targets []caddy.Target
	cmd := &cli.Command{
		Name: "test",
		Flags: []cli.Flag{
			&TargetFlag{
				Name:        "caddy-instance",
				Destination: &targets,
				Sources:     cli.EnvVars("TEST_CADDY_INSTANCES"),
			},
		},
		Action: func(_ context.Context, _ *cli.Command) error {
			return nil
		},
	}

	err := cmd.Run(context.Background(), []string{"test"})
	require.NoError(t, err)
	require.Equal(t, []caddy.Target{
		{Label: "edge", Project: "p1", Instance: "caddy1", Flags: map[string]string{"flag": "1"}},
		{Label: "internal", Project: "p2", Instance: "caddy2"},
	}, targets)
}

func TestTargetFlagEnvVarInvalid(t *testing.T) {
	t.Setenv("TEST_CADDY_INSTANCES", "invalid-target")

	var targets []caddy.Target
	cmd := &cli.Command{
		Name: "test",
		Flags: []cli.Flag{
			&TargetFlag{
				Name:        "caddy-instance",
				Destination: &targets,
				Sources:     cli.EnvVars("TEST_CADDY_INSTANCES"),
			},
		},
		Action: func(_ context.Context, _ *cli.Command) error {
			return nil
		},
	}

	err := cmd.Run(context.Background(), []string{"test"})
	require.Error(t, err)
	require.ErrorContains(t, err, "could not parse")
}

func TestTargetFlagString(t *testing.T) {
	fl := &TargetFlag{
		Name:  "caddy-instance",
		Value: []caddy.Target{{Label: "caddy", Project: "default", Instance: "caddy"}},
	}
	require.Equal(t, "caddy:default:caddy", fl.GetValue())

	var val targetValue
	created := val.Create(fl.Value, &fl.Value, cli.NoConfig{})
	require.Equal(t, "caddy:default:caddy", created.String())
	require.Equal(t, fl.Value, created.Get())
}

func TestOSTargetFlagCLI(t *testing.T) {
	var targets []caddy.Target
	cmd := &cli.Command{
		Name: "test",
		Flags: []cli.Flag{
			&TargetFlag{
				Name:        "os-path",
				Destination: &targets,
			},
		},
		Action: func(_ context.Context, _ *cli.Command) error {
			return nil
		},
	}

	err := cmd.Run(context.Background(), []string{
		"test",
		"--os-path", "edge:/etc/caddy/Caddyfile,reload=custom",
		"--os-path", "/var/caddy/Caddyfile",
	})
	require.NoError(t, err)
	require.Equal(t, []caddy.Target{
		{Label: "edge", Path: "/etc/caddy/Caddyfile", Flags: map[string]string{"reload": "custom"}},
		{Label: "caddy", Path: "/var/caddy/Caddyfile"},
	}, targets)
}

func TestOSTargetFlagEnvVar(t *testing.T) {
	t.Setenv("TEST_OS_PATH", "/etc/caddy/Caddyfile,edge:/var/caddy/Caddyfile")

	var targets []caddy.Target
	cmd := &cli.Command{
		Name: "test",
		Flags: []cli.Flag{
			&TargetFlag{
				Name:        "os-path",
				Destination: &targets,
				Sources:     cli.EnvVars("TEST_OS_PATH"),
			},
		},
		Action: func(_ context.Context, _ *cli.Command) error {
			return nil
		},
	}

	err := cmd.Run(context.Background(), []string{"test"})
	require.NoError(t, err)
	require.Equal(t, []caddy.Target{
		{Label: "caddy", Path: "/etc/caddy/Caddyfile"},
		{Label: "edge", Path: "/var/caddy/Caddyfile"},
	}, targets)
}

func TestOSTargetFlagString(t *testing.T) {
	fl := &TargetFlag{
		Name:  "os-path",
		Value: []caddy.Target{{Label: "caddy", Path: "/etc/caddy/Caddyfile"}},
	}
	require.Equal(t, "/etc/caddy/Caddyfile", fl.GetValue())

	var val targetValue
	created := val.Create(fl.Value, &fl.Value, cli.NoConfig{})
	require.Equal(t, "/etc/caddy/Caddyfile", created.String())
	require.Equal(t, fl.Value, created.Get())
}
