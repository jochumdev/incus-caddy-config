package templated

import (
	"testing"
	"text/template"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/lxc/incus-compose/ievent/iutil"
)

func TestData(t *testing.T) {
	now := time.Now()
	ev := func(name string, running bool, location string, config map[string]string) *iutil.Event {
		inst := iutil.NewInstance(running, location, config,
			map[string]map[string]string{"eth0": {"type": "nic", "network": "incusbr0"}},
			[]iutil.InstanceInterface{iutil.NewInstanceInterface("default", "incusbr0", true, []string{"10.0.0.5"}, nil)},
			nil)

		return iutil.NewEvent(now, "instance-started", "default", name, "").WithInstance(inst, true)
	}

	events := []*iutil.Event{
		ev("web-2", true, "node2", map[string]string{"user.label.prom": "", "user.label.prom.port": "9100"}),
		ev("web-1", true, "node1", map[string]string{"user.label.prom": "x"}),
		ev("other", true, "node1", map[string]string{"user.label.edge": "x"}),
	}

	d := data(Target{Label: "prom", Flags: map[string]string{"job": "incus"}}, events)
	require.Equal(t, "prom", d.Label)
	require.Equal(t, map[string]string{"job": "incus"}, d.Flags)
	require.Len(t, d.Instances, 2)

	first := d.Instances[0]
	require.Equal(t, "web-1", first.Name)
	require.Equal(t, "node1", first.Location)
	require.Equal(t, "x", first.Label)

	second := d.Instances[1]
	require.Equal(t, map[string]string{"port": "9100"}, second.Labels)
	require.Equal(t, "node2", second.Location)
	require.Equal(t, "incusbr0", second.Devices["eth0"]["network"])
	require.Equal(t, "10.0.0.5", second.IPv4("incusbr0"))
}

func TestInstanceIPv4(t *testing.T) {
	inst := Instance{Interfaces: []Interface{
		{Network: "lo", IPv4: []string{"127.0.0.1"}},
		{Network: "internal", IPv4: []string{"not-an-ip", "192.168.1.10"}},
		{Network: "external", IPv4: []string{"10.0.5.20"}},
	}}

	require.Equal(t, "192.168.1.10", inst.IPv4("internal"))
	require.Equal(t, "10.0.5.20", inst.IPv4("external"))
	require.Equal(t, "192.168.1.10", inst.IPv4(""))
	require.Equal(t, "192.168.1.10", inst.IPv4("nonexistent"))
	require.Empty(t, Instance{}.IPv4(""))
}

func TestRender(t *testing.T) {
	d := Data{Label: "prom", Instances: []Instance{
		{Name: "a", Labels: map[string]string{"port": "9100"}, Interfaces: []Interface{{IPv4: []string{"10.0.0.1"}}}},
		{Name: "b", Labels: map[string]string{"port": "9100"}, Interfaces: []Interface{{IPv4: []string{"10.0.0.2"}}}},
	}}

	fileSD := `[{{ range $i, $inst := .Instances }}{{ if $i }},{{ end }}{"targets":[{{ json (printf "%s:%s" ($inst.IPv4 "") $inst.Labels.port) }}],"labels":{"name":{{ json $inst.Name }}}}{{ end }}]`

	out, err := render(Target{Template: "file_sd", Text: fileSD}, d, nil)
	require.NoError(t, err)
	require.JSONEq(t, `[{"targets":["10.0.0.1:9100"],"labels":{"name":"a"}},{"targets":["10.0.0.2:9100"],"labels":{"name":"b"}}]`, string(out))

	out, err = render(Target{Text: `{{ join (split "a b" " ") "," }}|{{ shout }}`}, d, template.FuncMap{"shout": func() string { return "HI" }})
	require.NoError(t, err)
	require.Equal(t, "a,b|HI", string(out))

	_, err = render(Target{Text: "{{ .Unclosed"}, d, nil)
	require.ErrorContains(t, err, "parsing template")

	_, err = render(Target{Text: "{{ .Missing.Field }}"}, d, nil)
	require.ErrorContains(t, err, "rendering template")
}

func TestRenderMissingKeyIsEmpty(t *testing.T) {
	d := Data{Instances: []Instance{{Name: "a", Labels: map[string]string{}}}}

	out, err := render(Target{Text: `{{ range .Instances }}[{{ .Labels.port }}]{{ end }}[{{ .Flags.job }}]`}, d, nil)
	require.NoError(t, err)
	require.Equal(t, "[][]", string(out), "a missing key must not leak <no value> into the file")
}
