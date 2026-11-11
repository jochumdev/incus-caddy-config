package main

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/jochumdev/incus-templated/caddy"
	"github.com/jochumdev/incus-templated/templated"
)

func caddyTarget() templated.Target {
	return templated.Target{Label: "caddy", Preset: "caddy", Project: "default", Instance: "caddy"}
}

func TestConfigDefaults(t *testing.T) {
	cfg := newConfig()
	require.Equal(t, ":9153", cfg.HTTPAddr)
	require.Equal(t, "/var/lib/incus-templated", cfg.DataDir)
	require.Equal(t, "/run/secrets", cfg.SecretsDir)
	require.Equal(t, 250*time.Millisecond, cfg.DebounceWindow)
}

func TestConfigValidatePreset(t *testing.T) {
	cfg := newConfig()
	_, err := cfg.validate()
	require.ErrorContains(t, err, "at least one --target must be specified")

	cfg.Targets = []templated.Target{
		caddyTarget(),
		{Label: "edge", Preset: "caddy", Path: "/etc/caddy/Caddyfile", Validate: []string{}, Reload: []string{"systemctl", "reload", "caddy"}},
	}

	args, err := cfg.validate()
	require.NoError(t, err)

	preset := caddy.Preset()

	inst := args.Targets[0]
	require.Equal(t, "/config/Caddyfile", inst.Path)
	require.Equal(t, preset.Text, inst.Text)
	require.Equal(t, preset.Format, inst.Format)
	require.Equal(t, preset.Validate, inst.Validate)
	require.Equal(t, preset.Reload, inst.Reload)

	host := args.Targets[1]
	require.Equal(t, "/etc/caddy/Caddyfile", host.Path)
	require.Equal(t, preset.Format, host.Format)
	require.Empty(t, host.Validate, "an empty list turns the preset step off")
	require.Equal(t, []string{"systemctl", "reload", "caddy"}, host.Reload)
}

func TestConfigValidateTemplateFile(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, "file_sd.json.tmpl"), []byte("[]"), 0o600))

	cfg := newConfig()
	cfg.TemplatesDir = dir
	cfg.Targets = []templated.Target{
		{Label: "prom", Path: "/etc/prometheus/sd/incus.json", Template: "file_sd.json.tmpl"},
		{Label: "abs", Path: "/x", Template: filepath.Join(dir, "file_sd.json.tmpl")},
	}

	args, err := cfg.validate()
	require.NoError(t, err)
	require.Equal(t, "[]", args.Targets[0].Text)
	require.Equal(t, "[]", args.Targets[1].Text)
	require.Empty(t, args.Targets[0].Format)
	require.Empty(t, args.Targets[0].Reload)
}

func TestConfigValidateRejects(t *testing.T) {
	tests := []struct {
		name   string
		target templated.Target
		err    string
	}{
		{"unknown preset", templated.Target{Label: "a", Preset: "nginx", Path: "/x"}, "unknown preset"},
		{"missing label", templated.Target{Preset: "caddy", Path: "/x"}, "missing label"},
		{"instance without project", templated.Target{Label: "a", Preset: "caddy", Instance: "caddy"}, "instance needs a project"},
		{"os preset without path", templated.Target{Label: "a", Preset: "caddy"}, "missing path"},
		{"no template", templated.Target{Label: "a", Path: "/x"}, "missing template or preset"},
		{"template file missing", templated.Target{Label: "a", Path: "/x", Template: "/nonexistent.tmpl"}, "reading template"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			cfg := newConfig()
			cfg.Targets = []templated.Target{tc.target}

			_, err := cfg.validate()
			require.ErrorContains(t, err, tc.err)
		})
	}
}

func TestConfigValidateRejectsSameFile(t *testing.T) {
	cfg := newConfig()
	cfg.Targets = []templated.Target{caddyTarget(), {Label: "edge", Preset: "caddy", Project: "default", Instance: "caddy"}}

	_, err := cfg.validate()
	require.ErrorContains(t, err, "writes the same file as")

	cfg.Targets[1].Path = "/config/Other"
	_, err = cfg.validate()
	require.NoError(t, err, "another file in the same instance is fine")
}

func TestConfigFile(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, "file_sd.json.tmpl"), []byte("[]"), 0o600))

	file := filepath.Join(dir, "config.yaml")
	require.NoError(t, os.WriteFile(file, []byte(`
templates_dir: `+dir+`
targets:
  - label: prom
    path: /etc/prometheus/sd/incus.json
    template: file_sd.json.tmpl
    flags:
      job: incus
  - label: edge
    preset: caddy
    instance: caddy
    project: default
    reload: [caddy, reload, --config, "{path}"]
`), 0o600))

	cfg := newConfig()
	cfg.ConfigFile = file
	cfg.Targets = []templated.Target{{Label: "caddy", Preset: "caddy", Project: "default", Instance: "caddy-2"}}

	args, err := cfg.validate()
	require.NoError(t, err)
	require.Equal(t, dir, args.TemplatesDir)
	require.Len(t, args.Targets, 3, "file targets come first, then flag targets")

	require.Equal(t, "prom", args.Targets[0].Label)
	require.Equal(t, "[]", args.Targets[0].Text)
	require.Equal(t, map[string]string{"job": "incus"}, args.Targets[0].Flags)

	require.Equal(t, "edge", args.Targets[1].Label)
	require.Equal(t, []string{"caddy", "reload", "--config", "{path}"}, args.Targets[1].Reload)
	require.Equal(t, caddy.Preset().Format, args.Targets[1].Format)

	require.Equal(t, "caddy", args.Targets[2].Label)

	cfg.ConfigFile = filepath.Join(dir, "missing.yaml")
	_, err = cfg.validate()
	require.ErrorContains(t, err, "reading --config")

	require.NoError(t, os.WriteFile(file, []byte("targets: {"), 0o600))
	cfg.ConfigFile = file
	_, err = cfg.validate()
	require.ErrorContains(t, err, "parsing --config")
}

func TestConfigEndpoint(t *testing.T) {
	cfg := newConfig()
	cfg.Targets = []templated.Target{caddyTarget()}
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
	cfg.Targets = []templated.Target{caddyTarget()}
	args, err := cfg.validate()
	require.NoError(t, err)
	require.Empty(t, args.redactedToken())

	args.Token = "secret-token"
	require.Equal(t, "<redacted-(12)>", args.redactedToken())
}
