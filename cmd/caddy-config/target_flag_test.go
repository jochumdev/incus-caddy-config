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
		"--caddy-instance", "caddy1,instance=inst1,project=proj1",
	})
	require.NoError(t, err)
	require.Len(t, targets, 1)
	require.Equal(t, "caddy1", targets[0].Label)
	inst, _ := targets[0].Flag("instance")
	require.Equal(t, "inst1", inst)
	proj, _ := targets[0].Flag("project")
	require.Equal(t, "proj1", proj)
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
		"--caddy-instance", "caddy1,instance=inst1,project=proj1",
		"--caddy-instance", "caddy2,instance=inst2,project=proj2",
	})
	require.NoError(t, err)
	require.Len(t, targets, 2)
	require.Equal(t, "caddy1", targets[0].Label)
	inst, _ := targets[0].Flag("instance")
	require.Equal(t, "inst1", inst)
	require.Equal(t, "caddy2", targets[1].Label)
	inst, _ = targets[1].Flag("instance")
	require.Equal(t, "inst2", inst)
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
		"--caddy-instance", "caddy1,instance=inst1,project=proj1,caddy2,instance=inst2,project=proj2",
	})
	require.NoError(t, err)
	require.Len(t, targets, 2)
	require.Equal(t, "caddy1", targets[0].Label)
	require.Equal(t, "caddy2", targets[1].Label)
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
		"--caddy-instance", "caddy1,instance=inst1,project=proj1,flag1=val1,flag2=val2",
	})
	require.NoError(t, err)
	require.Len(t, targets, 1)
	require.Equal(t, "caddy1", targets[0].Label)
	f1, _ := targets[0].Flag("flag1")
	require.Equal(t, "val1", f1)
	f2, _ := targets[0].Flag("flag2")
	require.Equal(t, "val2", f2)
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
		"--caddy-instance", "caddy1:myproj:caddy-server",
	})
	require.Error(t, err)
	require.ErrorContains(t, err, "colon syntax is not supported")
}

func TestTargetFlagEnvVar(t *testing.T) {
	tests := []struct {
		name     string
		envVal   string
		expected []struct {
			label string
			flags map[string]string
		}
	}{
		{
			name:   "space separated with flags",
			envVal: "edge,instance=caddy1,project=p1,flag=1 internal,instance=caddy2,project=p2",
			expected: []struct {
				label string
				flags map[string]string
			}{
				{label: "edge", flags: map[string]string{"instance": "caddy1", "project": "p1", "flag": "1"}},
				{label: "internal", flags: map[string]string{"instance": "caddy2", "project": "p2"}},
			},
		},
		{
			name:   "comma separated with global template",
			envVal: "caddy-1,instance=caddy-1,project=caddy-config,global_template=mytemplate.caddyfile,caddy-2,instance=caddy-2,project=caddy-config",
			expected: []struct {
				label string
				flags map[string]string
			}{
				{label: "caddy-1", flags: map[string]string{"instance": "caddy-1", "project": "caddy-config", "global_template": "mytemplate.caddyfile"}},
				{label: "caddy-2", flags: map[string]string{"instance": "caddy-2", "project": "caddy-config"}},
			},
		},
		{
			name:   "comma space separated",
			envVal: "caddy-1,instance=caddy-1,project=p1, caddy-2,instance=caddy-2,project=p2",
			expected: []struct {
				label string
				flags map[string]string
			}{
				{label: "caddy-1", flags: map[string]string{"instance": "caddy-1", "project": "p1"}},
				{label: "caddy-2", flags: map[string]string{"instance": "caddy-2", "project": "p2"}},
			},
		},
		{
			name:   "multiline newline separated",
			envVal: "caddy-1,instance=caddy-1,project=p1\ncaddy-2,instance=caddy-2,project=p2",
			expected: []struct {
				label string
				flags map[string]string
			}{
				{label: "caddy-1", flags: map[string]string{"instance": "caddy-1", "project": "p1"}},
				{label: "caddy-2", flags: map[string]string{"instance": "caddy-2", "project": "p2"}},
			},
		},
		{
			name:   "no label comma separated",
			envVal: "instance=caddy-1,project=p1,instance=caddy-2,project=p2",
			expected: []struct {
				label string
				flags map[string]string
			}{
				{label: "caddy", flags: map[string]string{"instance": "caddy-1", "project": "p1"}},
				{label: "caddy", flags: map[string]string{"instance": "caddy-2", "project": "p2"}},
			},
		},
		{
			name:   "no label comma space separated",
			envVal: "instance=caddy-1,project=p1, instance=caddy-2,project=p2",
			expected: []struct {
				label string
				flags map[string]string
			}{
				{label: "caddy", flags: map[string]string{"instance": "caddy-1", "project": "p1"}},
				{label: "caddy", flags: map[string]string{"instance": "caddy-2", "project": "p2"}},
			},
		},
		{
			name:   "semicolon separated",
			envVal: "caddy-1,instance=caddy-1,project=p1;caddy-2,instance=caddy-2,project=p2",
			expected: []struct {
				label string
				flags map[string]string
			}{
				{label: "caddy-1", flags: map[string]string{"instance": "caddy-1", "project": "p1"}},
				{label: "caddy-2", flags: map[string]string{"instance": "caddy-2", "project": "p2"}},
			},
		},
		{
			name:   "double quoted items",
			envVal: "\"caddy-1,instance=caddy-1,project=p1\", \"caddy-2,instance=caddy-2,project=p2\"",
			expected: []struct {
				label string
				flags map[string]string
			}{
				{label: "caddy-1", flags: map[string]string{"instance": "caddy-1", "project": "p1"}},
				{label: "caddy-2", flags: map[string]string{"instance": "caddy-2", "project": "p2"}},
			},
		},
		{
			name:   "single quoted items",
			envVal: "'caddy-1,instance=caddy-1,project=p1' 'caddy-2,instance=caddy-2,project=p2'",
			expected: []struct {
				label string
				flags map[string]string
			}{
				{label: "caddy-1", flags: map[string]string{"instance": "caddy-1", "project": "p1"}},
				{label: "caddy-2", flags: map[string]string{"instance": "caddy-2", "project": "p2"}},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Setenv("TEST_CADDY_INSTANCES", tt.envVal)

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
			require.Len(t, targets, len(tt.expected))
			for i, exp := range tt.expected {
				require.Equal(t, exp.label, targets[i].Label)
				for k, v := range exp.flags {
					actual, ok := targets[i].Flag(k)
					require.True(t, ok, "missing flag %s in target %d", k, i)
					require.Equal(t, v, actual)
				}
			}
		})
	}
}

func TestTargetFlagEnvVarInvalid(t *testing.T) {
	t.Setenv("TEST_CADDY_INSTANCES", "caddy1:myproj:caddy-server")

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
	require.ErrorContains(t, err, "colon syntax is not supported")
}

func TestTargetFlagString(t *testing.T) {
	fl := &TargetFlag{
		Name:  "caddy-instance",
		Value: []caddy.Target{caddy.NewTarget("caddy", map[string]string{"project": "default", "instance": "caddy"})},
	}
	require.Equal(t, "caddy,instance=caddy,project=default", fl.GetValue())

	var val targetValue
	created := val.Create(fl.Value, &fl.Value, cli.NoConfig{})
	require.Equal(t, "caddy,instance=caddy,project=default", created.String())
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
		"--os-path", "edge,path=/etc/caddy/Caddyfile,reload=custom",
		"--os-path", "path=/var/caddy/Caddyfile",
	})
	require.NoError(t, err)
	require.Len(t, targets, 2)
	require.Equal(t, "edge", targets[0].Label)
	p0, _ := targets[0].Flag("path")
	require.Equal(t, "/etc/caddy/Caddyfile", p0)
	rel, _ := targets[0].Flag("reload")
	require.Equal(t, "custom", rel)
	require.Equal(t, "caddy", targets[1].Label)
	p1, _ := targets[1].Flag("path")
	require.Equal(t, "/var/caddy/Caddyfile", p1)
}

func TestOSTargetFlagEnvVar(t *testing.T) {
	tests := []struct {
		name     string
		envVal   string
		expected []struct {
			label string
			flags map[string]string
		}
	}{
		{
			name:   "mixed label and unlabelled comma separated",
			envVal: "path=/etc/caddy/Caddyfile,edge,path=/var/caddy/Caddyfile",
			expected: []struct {
				label string
				flags map[string]string
			}{
				{label: "caddy", flags: map[string]string{"path": "/etc/caddy/Caddyfile"}},
				{label: "edge", flags: map[string]string{"path": "/var/caddy/Caddyfile"}},
			},
		},
		{
			name:   "unlabelled comma separated",
			envVal: "path=/etc/caddy/Caddyfile,path=/var/caddy/Caddyfile",
			expected: []struct {
				label string
				flags map[string]string
			}{
				{label: "caddy", flags: map[string]string{"path": "/etc/caddy/Caddyfile"}},
				{label: "caddy", flags: map[string]string{"path": "/var/caddy/Caddyfile"}},
			},
		},
		{
			name:   "unlabelled comma space separated",
			envVal: "path=/etc/caddy/Caddyfile, path=/var/caddy/Caddyfile",
			expected: []struct {
				label string
				flags map[string]string
			}{
				{label: "caddy", flags: map[string]string{"path": "/etc/caddy/Caddyfile"}},
				{label: "caddy", flags: map[string]string{"path": "/var/caddy/Caddyfile"}},
			},
		},
		{
			name:   "space separated with flags",
			envVal: "edge,path=/etc/caddy/Caddyfile,reload=custom internal,path=/var/caddy/Caddyfile",
			expected: []struct {
				label string
				flags map[string]string
			}{
				{label: "edge", flags: map[string]string{"path": "/etc/caddy/Caddyfile", "reload": "custom"}},
				{label: "internal", flags: map[string]string{"path": "/var/caddy/Caddyfile"}},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Setenv("TEST_OS_PATH", tt.envVal)

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
			require.Len(t, targets, len(tt.expected))
			for i, exp := range tt.expected {
				require.Equal(t, exp.label, targets[i].Label)
				for k, v := range exp.flags {
					actual, ok := targets[i].Flag(k)
					require.True(t, ok, "missing flag %s in target %d", k, i)
					require.Equal(t, v, actual)
				}
			}
		})
	}
}

func TestOSTargetFlagString(t *testing.T) {
	fl := &TargetFlag{
		Name:  "os-path",
		Value: []caddy.Target{caddy.NewTarget("caddy", map[string]string{"path": "/etc/caddy/Caddyfile"})},
	}
	require.Equal(t, "caddy,path=/etc/caddy/Caddyfile", fl.GetValue())

	var val targetValue
	created := val.Create(fl.Value, &fl.Value, cli.NoConfig{})
	require.Equal(t, "caddy,path=/etc/caddy/Caddyfile", created.String())
	require.Equal(t, fl.Value, created.Get())
}
