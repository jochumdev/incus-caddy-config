package caddy

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/lxc/incus-compose/ievent/iutil"
)

func TestParseTarget(t *testing.T) {
	// Valid 3-part.
	target, err := ParseTarget("caddy1:myproj:caddy-server")
	require.NoError(t, err)
	require.Equal(t, Target{Label: "caddy1", Project: "myproj", Instance: "caddy-server"}, target)

	// 2-part is not supported (requires explicit label:project:instance).
	_, err = ParseTarget("external:caddy-proxy")
	require.Error(t, err)

	// Invalid empty components.
	_, err = ParseTarget("::")
	require.Error(t, err)

	_, err = ParseTarget("caddy:")
	require.Error(t, err)

	// Invalid component counts.
	_, err = ParseTarget("single")
	require.Error(t, err)

	_, err = ParseTarget("a:b:c:d")
	require.Error(t, err)
}

func TestExtractVhosts(t *testing.T) {
	ifaces1 := []iutil.InstanceInterface{
		iutil.NewInstanceInterface("default", "incusbr0", true, []string{"10.0.1.5"}, nil),
	}
	inst1 := iutil.NewInstance(true, map[string]string{
		"user.label.caddy1.domain":   "git.example.com",
		"user.label.caddy1.upstream": "3000",
	}, ifaces1, nil)
	now := time.Now()
	ev1 := iutil.NewEvent(now, "instance-started", "default", "git-1", "").WithInstance(inst1, true)

	// Replica 2 of git service
	ifaces2 := []iutil.InstanceInterface{
		iutil.NewInstanceInterface("default", "incusbr0", true, []string{"10.0.1.6"}, nil),
	}
	inst2 := iutil.NewInstance(true, map[string]string{
		"user.label.caddy1.domain":   "git.example.com",
		"user.label.caddy1.upstream": "3000",
	}, ifaces2, nil)
	ev2 := iutil.NewEvent(now, "instance-started", "default", "git-2", "").WithInstance(inst2, true)

	// Instance with different label prefix caddy2
	ifaces3 := []iutil.InstanceInterface{
		iutil.NewInstanceInterface("default", "incusbr0", true, []string{"10.0.1.7"}, nil),
	}
	inst3 := iutil.NewInstance(true, map[string]string{
		"user.label.caddy2.domain":   "other.lan",
		"user.label.caddy2.redirect": "https://example.com",
	}, ifaces3, nil)
	ev3 := iutil.NewEvent(now, "instance-started", "default", "other-1", "").WithInstance(inst3, true)

	events := []*iutil.Event{ev1, ev2, ev3}

	// Extract for caddy1.
	vhosts1 := extractVhosts("caddy1", events)
	require.Len(t, vhosts1, 1)
	require.Equal(t, "git.example.com", vhosts1[0].Domain)
	require.Equal(t, []string{"10.0.1.5:3000", "10.0.1.6:3000"}, vhosts1[0].Upstreams)

	// Extract for caddy2.
	vhosts2 := extractVhosts("caddy2", events)
	require.Len(t, vhosts2, 1)
	require.Equal(t, "other.lan", vhosts2[0].Domain)
	require.Equal(t, "https://example.com", vhosts2[0].Redirect)
}

func TestResolveIPv4WithNetwork(t *testing.T) {
	ifaces := []iutil.InstanceInterface{
		iutil.NewInstanceInterface("default", "internal", true, []string{"192.168.1.10"}, nil),
		iutil.NewInstanceInterface("default", "external", true, []string{"10.0.5.20"}, nil),
	}
	inst := iutil.NewInstance(true, nil, ifaces, nil)

	// Explicit network.
	ipInternal := resolveIPv4(inst, "internal")
	require.Equal(t, "192.168.1.10", ipInternal)

	ipExternal := resolveIPv4(inst, "external")
	require.Equal(t, "10.0.5.20", ipExternal)

	// Fallback to first available.
	ipDefault := resolveIPv4(inst, "")
	require.Equal(t, "192.168.1.10", ipDefault)
}

func TestResolveIPv4EdgeCases(t *testing.T) {
	// Nil instance.
	require.Empty(t, resolveIPv4(nil, ""))
	require.Empty(t, resolveIPv4(nil, "eth0"))

	// Empty interfaces.
	instEmpty := iutil.NewInstance(true, nil, nil, nil)
	require.Empty(t, resolveIPv4(instEmpty, ""))

	// Loopback and invalid IP addresses.
	ifacesBad := []iutil.InstanceInterface{
		iutil.NewInstanceInterface("default", "lo", true, []string{"127.0.0.1"}, nil),
		iutil.NewInstanceInterface("default", "eth0", true, []string{"not-an-ip", "256.0.0.1"}, nil),
	}
	instBad := iutil.NewInstance(true, nil, ifacesBad, nil)
	require.Empty(t, resolveIPv4(instBad, ""))
	require.Empty(t, resolveIPv4(instBad, "eth0"))

	// Target network not found falls back to first valid IPv4.
	ifacesFallback := []iutil.InstanceInterface{
		iutil.NewInstanceInterface("default", "eth0", true, []string{"192.168.10.5"}, nil),
	}
	instFallback := iutil.NewInstance(true, nil, ifacesFallback, nil)
	require.Equal(t, "192.168.10.5", resolveIPv4(instFallback, "nonexistent"))
}

func TestExtractVhostsEdgeCases(t *testing.T) {
	now := time.Now()

	// 1. Nil instance in event.
	evNil := iutil.NewEvent(now, "instance-started", "default", "nil-inst", "")

	// 2. Stopped instance.
	instStopped := iutil.NewInstance(false, map[string]string{
		"user.label.caddy.domain": "stopped.com",
	}, nil, nil)
	evStopped := iutil.NewEvent(now, "instance-stopped", "default", "stopped", "").WithInstance(instStopped, true)

	// 3. Instance with missing or whitespace-only domain.
	instNoDomain := iutil.NewInstance(true, map[string]string{
		"user.label.caddy.upstream": "8080",
	}, nil, nil)
	evNoDomain := iutil.NewEvent(now, "instance-started", "default", "no-domain", "").WithInstance(instNoDomain, true)

	instWhitespaceDomain := iutil.NewInstance(true, map[string]string{
		"user.label.caddy.domain": "   ",
	}, nil, nil)
	evWhitespaceDomain := iutil.NewEvent(now, "instance-started", "default", "white-domain", "").WithInstance(instWhitespaceDomain, true)

	// 4. Instance with host:port in upstream label.
	ifaces := []iutil.InstanceInterface{
		iutil.NewInstanceInterface("default", "eth0", true, []string{"10.0.0.1"}, nil),
	}
	instHostPort := iutil.NewInstance(true, map[string]string{
		"user.label.caddy.domain":   "custom.lan",
		"user.label.caddy.upstream": "192.168.50.1:9090",
	}, ifaces, nil)
	evHostPort := iutil.NewEvent(now, "instance-started", "default", "custom", "").WithInstance(instHostPort, true)

	// 5. Instance with empty upstream port -> defaults to IP.
	instBareIP := iutil.NewInstance(true, map[string]string{
		"user.label.caddy.domain": "bare.lan",
	}, ifaces, nil)
	evBareIP := iutil.NewEvent(now, "instance-started", "default", "bare", "").WithInstance(instBareIP, true)

	// 6. Instance with no IP addresses -> upstream remains empty.
	instNoIP := iutil.NewInstance(true, map[string]string{
		"user.label.caddy.domain":   "noip.lan",
		"user.label.caddy.redirect": "https://fallback.lan",
	}, nil, nil)
	evNoIP := iutil.NewEvent(now, "instance-started", "default", "noip", "").WithInstance(instNoIP, true)

	events := []*iutil.Event{evNil, evStopped, evNoDomain, evWhitespaceDomain, evHostPort, evBareIP, evNoIP}
	vhosts := extractVhosts("caddy", events)

	require.Len(t, vhosts, 3)

	require.Equal(t, "custom.lan", vhosts[0].Domain)
	require.Equal(t, []string{"192.168.50.1:9090"}, vhosts[0].Upstreams)

	require.Equal(t, "bare.lan", vhosts[1].Domain)
	require.Equal(t, []string{"10.0.0.1"}, vhosts[1].Upstreams)

	require.Equal(t, "noip.lan", vhosts[2].Domain)
	require.Empty(t, vhosts[2].Upstreams)
	require.Equal(t, "https://fallback.lan", vhosts[2].Redirect)
}

func TestExtractVhostsMerging(t *testing.T) {
	now := time.Now()

	// Instance 1: sets domain and upstream, no redirect, no template.
	ifaces1 := []iutil.InstanceInterface{
		iutil.NewInstanceInterface("default", "eth0", true, []string{"10.0.0.1"}, nil),
	}
	inst1 := iutil.NewInstance(true, map[string]string{
		"user.label.caddy.domain":   "multi.lan",
		"user.label.caddy.upstream": "8080",
	}, ifaces1, nil)
	ev1 := iutil.NewEvent(now, "instance-started", "default", "inst-1", "").WithInstance(inst1, true)

	// Instance 2: same domain, different upstream, adds redirect and template.
	ifaces2 := []iutil.InstanceInterface{
		iutil.NewInstanceInterface("default", "eth0", true, []string{"10.0.0.2"}, nil),
	}
	inst2 := iutil.NewInstance(true, map[string]string{
		"user.label.caddy.domain":   "multi.lan",
		"user.label.caddy.upstream": "8080",
		"user.label.caddy.redirect": "https://multi.com",
		"user.label.caddy.template": "custom_tmpl",
	}, ifaces2, nil)
	ev2 := iutil.NewEvent(now, "instance-started", "default", "inst-2", "").WithInstance(inst2, true)

	// Instance 3: same domain, duplicate upstream of inst1, attempt to overwrite redirect and template.
	inst3 := iutil.NewInstance(true, map[string]string{
		"user.label.caddy.domain":   "multi.lan",
		"user.label.caddy.upstream": "8080",
		"user.label.caddy.redirect": "https://overwrite.com",
		"user.label.caddy.template": "overwrite_tmpl",
	}, ifaces1, nil)
	ev3 := iutil.NewEvent(now, "instance-started", "default", "inst-3", "").WithInstance(inst3, true)

	vhosts := extractVhosts("caddy", []*iutil.Event{ev1, ev2, ev3})
	require.Len(t, vhosts, 1)

	v := vhosts[0]
	require.Equal(t, "multi.lan", v.Domain)
	// Duplicate upstream 10.0.0.1:8080 must not be added twice, upstreams must be sorted.
	require.Equal(t, []string{"10.0.0.1:8080", "10.0.0.2:8080"}, v.Upstreams)
	// First non-empty redirect and template must be preserved.
	require.Equal(t, "https://multi.com", v.Redirect)
	require.Equal(t, "custom_tmpl", v.Template)
}
