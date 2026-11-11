package main

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/urfave/cli/v3"

	"github.com/jochumdev/incus-templated/templated"
)

func targetCommand(targets *[]templated.Target, got *[]templated.Target) *cli.Command {
	return &cli.Command{
		Name: "test",
		Flags: []cli.Flag{
			&TargetFlag{
				Name:        "target",
				Destination: targets,
				Sources:     cli.EnvVars("TEST_TARGETS"),
			},
		},
		Action: func(_ context.Context, c *cli.Command) error {
			v, ok := c.Value("target").([]templated.Target)
			if ok {
				*got = v
			}

			return nil
		},
	}
}

func TestTargetFlagCLI(t *testing.T) {
	var targets, got []templated.Target

	err := targetCommand(&targets, &got).Run(context.Background(), []string{
		"test",
		"--target", "edge,preset=caddy,instance=caddy-1,project=default",
		"--target", "prom,path=/etc/prometheus/sd/incus.json,template=file_sd.json.tmpl t3,path=/x",
	})
	require.NoError(t, err)
	require.Len(t, targets, 3)
	require.Equal(t, "edge", targets[0].Label)
	require.Equal(t, "caddy", targets[0].Preset)
	require.Equal(t, "caddy-1", targets[0].Instance)
	require.Equal(t, "prom", targets[1].Label)
	require.Equal(t, "file_sd.json.tmpl", targets[1].Template)
	require.Equal(t, "t3", targets[2].Label)
	require.Equal(t, targets, got)
}

func TestTargetFlagEnvVar(t *testing.T) {
	t.Setenv("TEST_TARGETS", "edge,instance=caddy-1,project=default internal,instance=caddy-2,project=default")

	var targets, got []templated.Target

	err := targetCommand(&targets, &got).Run(context.Background(), []string{"test"})
	require.NoError(t, err)
	require.Len(t, targets, 2)
	require.Equal(t, "edge", targets[0].Label)
	require.Equal(t, "internal", targets[1].Label)
}

func TestTargetFlagInvalid(t *testing.T) {
	var targets, got []templated.Target

	err := targetCommand(&targets, &got).Run(context.Background(), []string{"test", "--target", "caddy1:myproj:caddy-server"})
	require.ErrorContains(t, err, "colon syntax is not supported")

	t.Setenv("TEST_TARGETS", "   ")

	err = targetCommand(&targets, &got).Run(context.Background(), []string{"test"})
	require.Error(t, err)
}

func TestTargetFlagString(t *testing.T) {
	v := targetValue{}
	require.Equal(t, "edge -> p/i:/config/Caddyfile, prom -> /x", v.ToString([]templated.Target{
		{Label: "edge", Project: "p", Instance: "i", Path: "/config/Caddyfile"},
		{Label: "prom", Path: "/x"},
	}))
}
