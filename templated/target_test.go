package templated

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestParseTargets(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want []Target
	}{
		{
			name: "instance target with preset",
			in:   "edge,preset=caddy,instance=caddy-1,project=default",
			want: []Target{{Label: "edge", Preset: "caddy", Instance: "caddy-1", Project: "default", Flags: map[string]string{}}},
		},
		{
			name: "label given as a flag",
			in:   "instance=caddy-1,project=p,label=edge",
			want: []Target{{Label: "edge", Instance: "caddy-1", Project: "p", Flags: map[string]string{}}},
		},
		{
			name: "os target with template and unknown flags",
			in:   "prom,path=/etc/prometheus/sd/incus.json,template=file_sd.json.tmpl,job=incus",
			want: []Target{{Label: "prom", Path: "/etc/prometheus/sd/incus.json", Template: "file_sd.json.tmpl", Flags: map[string]string{"job": "incus"}}},
		},
		{
			name: "quoted commands are split into argv",
			in:   `edge,path=/etc/caddy/Caddyfile,format='caddy fmt --overwrite {staging}',reload="caddy reload --config {path}"`,
			want: []Target{{
				Label:  "edge",
				Path:   "/etc/caddy/Caddyfile",
				Format: []string{"caddy", "fmt", "--overwrite", "{staging}"},
				Reload: []string{"caddy", "reload", "--config", "{path}"},
				Flags:  map[string]string{},
			}},
		},
		{
			name: "duplicate flag: last wins",
			in:   "edge,instance=first,instance=second,project=p",
			want: []Target{{Label: "edge", Instance: "second", Project: "p", Flags: map[string]string{}}},
		},
		{
			name: "comma- and space-separated targets",
			in:   "t1,instance=i1,project=p1,t2,instance=i2,project=p2 t3,path=/x",
			want: []Target{
				{Label: "t1", Instance: "i1", Project: "p1", Flags: map[string]string{}},
				{Label: "t2", Instance: "i2", Project: "p2", Flags: map[string]string{}},
				{Label: "t3", Path: "/x", Flags: map[string]string{}},
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, err := ParseTargets(tc.in)
			require.NoError(t, err)
			require.Equal(t, tc.want, got)
		})
	}
}

func TestParseTargetsRejects(t *testing.T) {
	for _, in := range []string{"", "   ", "caddy1:myproj:caddy-server"} {
		_, err := ParseTargets(in)
		require.Error(t, err, in)
	}
}

func TestTargetKey(t *testing.T) {
	require.Equal(t, "p/i:/config/Caddyfile", Target{Instance: "i", Project: "p", Path: "/config/Caddyfile"}.Key())
	require.Equal(t, "os:/etc/caddy/Caddyfile", Target{Path: "/etc/caddy/Caddyfile"}.Key())
}
