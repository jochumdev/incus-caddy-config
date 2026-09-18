package caddy

import (
	"bytes"
	"log/slog"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/lxc/incus-compose/ievent/iutil"
)

func TestParseTarget(t *testing.T) {
	// Valid with explicit label and instance, project flags.
	target, err := ParseTarget("caddy1,instance=caddy-server,project=myproj")
	require.NoError(t, err)
	require.Equal(t, "caddy1", target.Label)
	inst, ok := target.Flag("instance")
	require.True(t, ok)
	require.Equal(t, "caddy-server", inst)
	proj, ok := target.Flag("project")
	require.True(t, ok)
	require.Equal(t, "myproj", proj)

	// Valid with omitted label (defaults to "caddy").
	target, err = ParseTarget("instance=caddy-server,project=myproj")
	require.NoError(t, err)
	require.Equal(t, "caddy", target.Label)
	inst, ok = target.Flag("instance")
	require.True(t, ok)
	require.Equal(t, "caddy-server", inst)
	proj, ok = target.Flag("project")
	require.True(t, ok)
	require.Equal(t, "myproj", proj)

	// Valid with label flag.
	target, err = ParseTarget("instance=caddy-server,project=myproj,label=caddy1")
	require.NoError(t, err)
	require.Equal(t, "caddy1", target.Label)

	// Valid with additional flags.
	targetWithFlags, err := ParseTarget("caddy1,instance=caddy-server,project=myproj,flag1=val1,flag2=val2")
	require.NoError(t, err)
	require.Equal(t, "caddy1", targetWithFlags.Label)
	f1, ok := targetWithFlags.Flag("flag1")
	require.True(t, ok)
	require.Equal(t, "val1", f1)
	f2, ok := targetWithFlags.Flag("flag2")
	require.True(t, ok)
	require.Equal(t, "val2", f2)
	require.Equal(t, "caddy1,instance=caddy-server,project=myproj,flag1=val1,flag2=val2", targetWithFlags.String())

	// Duplicate flags: last input wins.
	dupTarget, err := ParseTarget("edge,instance=first,instance=second,project=p")
	require.NoError(t, err)
	require.Equal(t, "edge", dupTarget.Label)
	inst, ok = dupTarget.Flag("instance")
	require.True(t, ok)
	require.Equal(t, "second", inst)
	proj, ok = dupTarget.Flag("project")
	require.True(t, ok)
	require.Equal(t, "p", proj)

	// OS path target with default label.
	target, err = ParseTarget("path=/etc/caddy/Caddyfile")
	require.NoError(t, err)
	require.Equal(t, "caddy", target.Label)
	p, ok := target.Flag("path")
	require.True(t, ok)
	require.Equal(t, "/etc/caddy/Caddyfile", p)

	// OS path target with explicit label.
	target, err = ParseTarget("edge,path=/etc/caddy/Caddyfile")
	require.NoError(t, err)
	require.Equal(t, "edge", target.Label)
	p, ok = target.Flag("path")
	require.True(t, ok)
	require.Equal(t, "/etc/caddy/Caddyfile", p)

	// Multiple targets rejected by single ParseTarget.
	_, err = ParseTarget("caddy1,instance=i1,project=p,caddy2,instance=i2,project=p")
	require.Error(t, err)

	// Colon syntax is not supported.
	_, err = ParseTarget("caddy1:myproj:caddy-server")
	require.Error(t, err)
	require.ErrorContains(t, err, "colon syntax is not supported")

	_, err = ParseTarget("external:caddy-proxy")
	require.Error(t, err)
	require.ErrorContains(t, err, "colon syntax is not supported")

	// Empty string.
	_, err = ParseTarget("")
	require.Error(t, err)
}

func TestParseTargets(t *testing.T) {
	// Comma-separated.
	targets, err := ParseTargets("t1,instance=i1,project=p1,t2,instance=i2,project=p2")
	require.NoError(t, err)
	require.Len(t, targets, 2)
	require.Equal(t, "t1", targets[0].Label)
	inst, _ := targets[0].Flag("instance")
	require.Equal(t, "i1", inst)
	proj, _ := targets[0].Flag("project")
	require.Equal(t, "p1", proj)
	require.Equal(t, "t2", targets[1].Label)
	inst, _ = targets[1].Flag("instance")
	require.Equal(t, "i2", inst)
	proj, _ = targets[1].Flag("project")
	require.Equal(t, "p2", proj)

	// Space-separated.
	targets, err = ParseTargets("t1,instance=i1,project=p1 t2,instance=i2,project=p2")
	require.NoError(t, err)
	require.Len(t, targets, 2)
	require.Equal(t, "t1", targets[0].Label)
	require.Equal(t, "t2", targets[1].Label)

	// Comma-separated with flags.
	targets, err = ParseTargets("t1,instance=i1,project=p1,flag1=val1,t2,instance=i2,project=p2,flag2=val2")
	require.NoError(t, err)
	require.Len(t, targets, 2)
	f1, ok := targets[0].Flag("flag1")
	require.True(t, ok)
	require.Equal(t, "val1", f1)
	f2, ok := targets[1].Flag("flag2")
	require.True(t, ok)
	require.Equal(t, "val2", f2)

	// Space-separated with flags.
	targets, err = ParseTargets("t1,instance=i1,project=p1,flag1=val1 t2,instance=i2,project=p2,flag2=val2")
	require.NoError(t, err)
	require.Len(t, targets, 2)
	f1, ok = targets[0].Flag("flag1")
	require.True(t, ok)
	require.Equal(t, "val1", f1)
	f2, ok = targets[1].Flag("flag2")
	require.True(t, ok)
	require.Equal(t, "val2", f2)

	// OS targets comma-separated.
	targets, err = ParseTargets("path=/etc/caddy/Caddyfile,edge,path=/var/caddy/Caddyfile")
	require.NoError(t, err)
	require.Len(t, targets, 2)
	require.Equal(t, "caddy", targets[0].Label)
	p, _ := targets[0].Flag("path")
	require.Equal(t, "/etc/caddy/Caddyfile", p)
	require.Equal(t, "edge", targets[1].Label)
	p, _ = targets[1].Flag("path")
	require.Equal(t, "/var/caddy/Caddyfile", p)

	// OS targets space-separated.
	targets, err = ParseTargets("path=/etc/caddy/Caddyfile edge,path=/var/caddy/Caddyfile")
	require.NoError(t, err)
	require.Len(t, targets, 2)
	require.Equal(t, "caddy", targets[0].Label)
	require.Equal(t, "edge", targets[1].Label)

	// Empty string.
	_, err = ParseTargets("")
	require.Error(t, err)

	// Whitespace only.
	_, err = ParseTargets("   ")
	require.Error(t, err)
}

func TestTargetGlobalTemplate(t *testing.T) {
	// From file path flag.
	target, err := ParseTarget("edge,instance=caddy,project=default,global_template=/etc/caddy/global.caddyfile")
	require.NoError(t, err)
	tmpl, ok := target.Flag("global_template")
	require.True(t, ok)
	require.Equal(t, "/etc/caddy/global.caddyfile", tmpl)

	// From hyphenated flag name.
	target, err = ParseTarget("edge,instance=caddy,project=default,global-template=/etc/caddy/global.caddyfile")
	require.NoError(t, err)
	tmpl, ok = target.Flag("global-template")
	require.True(t, ok)
	require.Equal(t, "/etc/caddy/global.caddyfile", tmpl)

	// No global template flag.
	target, err = ParseTarget("edge,instance=caddy,project=default")
	require.NoError(t, err)
	_, ok = target.Flag("global_template")
	require.False(t, ok)
}

func TestExtractVhosts(t *testing.T) {
	ifaces1 := []iutil.InstanceInterface{
		iutil.NewInstanceInterface("default", "incusbr0", true, []string{"10.0.1.5"}, nil),
	}
	inst1 := iutil.NewInstance(true, map[string]string{
		"user.label.caddy1": "git.example.com,upstream=3000",
	}, ifaces1, nil)
	now := time.Now()
	ev1 := iutil.NewEvent(now, "instance-started", "default", "git-1", "").WithInstance(inst1, true)

	// Replica 2 of git service (using legacy .domain to verify backward compatibility)
	ifaces2 := []iutil.InstanceInterface{
		iutil.NewInstanceInterface("default", "incusbr0", true, []string{"10.0.1.6"}, nil),
	}
	inst2 := iutil.NewInstance(true, map[string]string{
		"user.label.caddy1.domain": "git.example.com,upstream=3000",
	}, ifaces2, nil)
	ev2 := iutil.NewEvent(now, "instance-started", "default", "git-2", "").WithInstance(inst2, true)

	// Instance with different label prefix caddy2
	ifaces3 := []iutil.InstanceInterface{
		iutil.NewInstanceInterface("default", "incusbr0", true, []string{"10.0.1.7"}, nil),
	}
	inst3 := iutil.NewInstance(true, map[string]string{
		"user.label.caddy2": "other.lan,redir=https://example.com",
	}, ifaces3, nil)
	ev3 := iutil.NewEvent(now, "instance-started", "default", "other-1", "").WithInstance(inst3, true)

	events := []*iutil.Event{ev1, ev2, ev3}

	// Extract for caddy1.
	vhosts1 := extractVhosts(nil, "caddy1", events)
	require.Len(t, vhosts1, 1)
	require.Equal(t, "git.example.com", vhosts1[0].Domain)
	require.Equal(t, []string{"10.0.1.5:3000", "10.0.1.6:3000"}, vhosts1[0].Upstreams)

	// Extract for caddy2.
	vhosts2 := extractVhosts(nil, "caddy2", events)
	require.Len(t, vhosts2, 1)
	require.Equal(t, "other.lan", vhosts2[0].Domain)
	require.Equal(t, "https://example.com{uri}", vhosts2[0].Redirect)
}

func TestExtractVhostsWithNetworkFlag(t *testing.T) {
	ifaces1 := []iutil.InstanceInterface{
		iutil.NewInstanceInterface("default", "eth0", true, []string{"10.0.1.10"}, nil),
		iutil.NewInstanceInterface("default", "internal", true, []string{"192.168.10.20"}, nil),
	}
	inst1 := iutil.NewInstance(true, map[string]string{
		"user.incus-compose.service": "api",
		"user.label.edge":            "api.example.com,upstream=8080,network=internal",
	}, ifaces1, nil)
	now := time.Now()
	ev1 := iutil.NewEvent(now, "instance-started", "default", "api-1", "").WithInstance(inst1, true)

	// Replica 2 shares service and inherits network flag
	ifaces2 := []iutil.InstanceInterface{
		iutil.NewInstanceInterface("default", "eth0", true, []string{"10.0.1.11"}, nil),
		iutil.NewInstanceInterface("default", "internal", true, []string{"192.168.10.21"}, nil),
	}
	inst2 := iutil.NewInstance(true, map[string]string{
		"user.incus-compose.service": "api",
	}, ifaces2, nil)
	ev2 := iutil.NewEvent(now, "instance-started", "default", "api-2", "").WithInstance(inst2, true)

	vhosts := extractVhosts(nil, "edge", []*iutil.Event{ev1, ev2})
	require.Len(t, vhosts, 1)
	require.Equal(t, "api.example.com", vhosts[0].Domain)
	require.Equal(t, []string{"192.168.10.20:8080", "192.168.10.21:8080"}, vhosts[0].Upstreams)
	require.Equal(t, "internal", vhosts[0].Flags["network"])
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
		"user.label.caddy": "stopped.com",
	}, nil, nil)
	evStopped := iutil.NewEvent(now, "instance-stopped", "default", "stopped", "").WithInstance(instStopped, true)

	// 3. Instance with missing or whitespace-only domain.
	instNoDomain := iutil.NewInstance(true, map[string]string{
		"user.label.caddy.service": "no-domain",
	}, nil, nil)
	evNoDomain := iutil.NewEvent(now, "instance-started", "default", "no-domain", "").WithInstance(instNoDomain, true)

	instWhitespaceDomain := iutil.NewInstance(true, map[string]string{
		"user.label.caddy": "   ",
	}, nil, nil)
	evWhitespaceDomain := iutil.NewEvent(now, "instance-started", "default", "white-domain", "").WithInstance(instWhitespaceDomain, true)

	// 4. Instance with host:port in upstream label.
	ifaces := []iutil.InstanceInterface{
		iutil.NewInstanceInterface("default", "eth0", true, []string{"10.0.0.1"}, nil),
	}
	instHostPort := iutil.NewInstance(true, map[string]string{
		"user.label.caddy": "custom.lan,upstream=192.168.50.1:9090",
	}, ifaces, nil)
	evHostPort := iutil.NewEvent(now, "instance-started", "default", "custom", "").WithInstance(instHostPort, true)

	// 5. Instance with empty upstream port -> defaults to IP.
	instBareIP := iutil.NewInstance(true, map[string]string{
		"user.label.caddy": "bare.lan",
	}, ifaces, nil)
	evBareIP := iutil.NewEvent(now, "instance-started", "default", "bare", "").WithInstance(instBareIP, true)

	// 6. Instance with no IP addresses -> upstream remains empty.
	instNoIP := iutil.NewInstance(true, map[string]string{
		"user.label.caddy": "noip.lan,redir=https://fallback.lan",
	}, nil, nil)
	evNoIP := iutil.NewEvent(now, "instance-started", "default", "noip", "").WithInstance(instNoIP, true)

	events := []*iutil.Event{evNil, evStopped, evNoDomain, evWhitespaceDomain, evHostPort, evBareIP, evNoIP}
	vhosts := extractVhosts(nil, "caddy", events)

	require.Len(t, vhosts, 3)

	require.Equal(t, "custom.lan", vhosts[0].Domain)
	require.Equal(t, []string{"192.168.50.1:9090"}, vhosts[0].Upstreams)

	require.Equal(t, "bare.lan", vhosts[1].Domain)
	require.Equal(t, []string{"10.0.0.1"}, vhosts[1].Upstreams)

	require.Equal(t, "noip.lan", vhosts[2].Domain)
	require.Empty(t, vhosts[2].Upstreams)
	require.Equal(t, "https://fallback.lan{uri}", vhosts[2].Redirect)
}

func TestExtractVhostsMerging(t *testing.T) {
	now := time.Now()

	// Instance 1: sets domain and upstream, no redirect, no template.
	ifaces1 := []iutil.InstanceInterface{
		iutil.NewInstanceInterface("default", "eth0", true, []string{"10.0.0.1"}, nil),
	}
	inst1 := iutil.NewInstance(true, map[string]string{
		"user.label.caddy": "multi.lan,upstream=8080",
	}, ifaces1, nil)
	ev1 := iutil.NewEvent(now, "instance-started", "default", "inst-1", "").WithInstance(inst1, true)

	// Instance 2: same domain, different upstream, adds redirect and template.
	ifaces2 := []iutil.InstanceInterface{
		iutil.NewInstanceInterface("default", "eth0", true, []string{"10.0.0.2"}, nil),
	}
	inst2 := iutil.NewInstance(true, map[string]string{
		"user.label.caddy": "multi.lan,upstream=8080,redir=https://multi.com,template=custom_tmpl",
	}, ifaces2, nil)
	ev2 := iutil.NewEvent(now, "instance-started", "default", "inst-2", "").WithInstance(inst2, true)

	// Instance 3: same domain, duplicate upstream of inst1, attempt to overwrite redirect and template.
	inst3 := iutil.NewInstance(true, map[string]string{
		"user.label.caddy": "multi.lan,upstream=8080,redir=https://overwrite.com,template=overwrite_tmpl",
	}, ifaces1, nil)
	ev3 := iutil.NewEvent(now, "instance-started", "default", "inst-3", "").WithInstance(inst3, true)

	vhosts := extractVhosts(nil, "caddy", []*iutil.Event{ev1, ev2, ev3})
	require.Len(t, vhosts, 1)

	v := vhosts[0]
	require.Equal(t, "multi.lan", v.Domain)
	// Duplicate upstream 10.0.0.1:8080 must not be added twice, upstreams must be sorted.
	require.Equal(t, []string{"10.0.0.1:8080", "10.0.0.2:8080"}, v.Upstreams)
	// First non-empty redirect and template must be preserved.
	require.Equal(t, "https://multi.com{uri}", v.Redirect)
	require.Equal(t, "custom_tmpl", v.Template)
}

func TestParseRedirs(t *testing.T) {
	// Flags uri and no-uri with whitespace separation.
	entries := parseRedirs("www.example.com,no-uri old.example.com,uri alias.example.com")
	require.Len(t, entries, 3)
	require.Equal(t, "www.example.com", entries[0].Domain)
	require.False(t, entries[0].IncludeURI)
	require.Equal(t, "old.example.com", entries[1].Domain)
	require.True(t, entries[1].IncludeURI)
	require.Equal(t, "alias.example.com", entries[2].Domain)
	require.True(t, entries[2].IncludeURI) // default is uri

	// Comma separated without space.
	entries = parseRedirs("www.example.com,old.example.com")
	require.Len(t, entries, 2)
	require.Equal(t, "www.example.com", entries[0].Domain)
	require.True(t, entries[0].IncludeURI)
	require.Equal(t, "old.example.com", entries[1].Domain)
	require.True(t, entries[1].IncludeURI)

	// Comma and space separated.
	entries = parseRedirs("www.example.com, old.example.com")
	require.Len(t, entries, 2)
	require.Equal(t, "www.example.com", entries[0].Domain)
	require.Equal(t, "old.example.com", entries[1].Domain)

	// Multi-line YAML string.
	entries = parseRedirs("\nwww.example.com,no-uri\n  old.example.com,uri\n")
	require.Len(t, entries, 2)
	require.False(t, entries[0].IncludeURI)
	require.True(t, entries[1].IncludeURI)

	// Flags with custom template.
	entries = parseRedirs("www.example.com,template=sometemplate.caddyfile,no-uri")
	require.Len(t, entries, 1)
	require.Equal(t, "www.example.com", entries[0].Domain)
	require.False(t, entries[0].IncludeURI)
	require.Equal(t, "sometemplate.caddyfile", entries[0].Template)

	// Empty and punctuation only.
	require.Empty(t, parseRedirs(""))
	require.Empty(t, parseRedirs("  ,  , "))
}

func TestExtractVhostsWithRedirTemplate(t *testing.T) {
	ifaces := []iutil.InstanceInterface{
		iutil.NewInstanceInterface("default", "eth0", true, []string{"10.0.1.10"}, nil),
	}
	inst := iutil.NewInstance(true, map[string]string{
		"user.label.edge":        "example.com,upstream=8080",
		"user.label.edge.redirs": "old.example.com,template=custom_redir.caddyfile,no-uri default.example.com",
	}, ifaces, nil)
	now := time.Now()
	ev := iutil.NewEvent(now, "instance-started", "default", "web-1", "").WithInstance(inst, true)

	vhosts := extractVhosts(nil, "edge", []*iutil.Event{ev})
	require.Len(t, vhosts, 3)

	// Primary domain
	require.Equal(t, "example.com", vhosts[0].Domain)
	require.Empty(t, vhosts[0].Template)

	// Redir with custom template
	require.Equal(t, "old.example.com", vhosts[1].Domain)
	require.Equal(t, "https://example.com", vhosts[1].Redirect)
	require.Equal(t, "custom_redir.caddyfile", vhosts[1].Template)

	// Redir without custom template
	require.Equal(t, "default.example.com", vhosts[2].Domain)
	require.Equal(t, "https://example.com{uri}", vhosts[2].Redirect)
	require.Empty(t, vhosts[2].Template)
}

func TestExtractVhostsWithRedirs(t *testing.T) {
	ifaces := []iutil.InstanceInterface{
		iutil.NewInstanceInterface("default", "eth0", true, []string{"10.0.1.10"}, nil),
	}
	inst := iutil.NewInstance(true, map[string]string{
		"user.label.edge":        "example.com,upstream=8080",
		"user.label.edge.redirs": "www.example.com,no-uri old.example.com,uri alias.example.com",
	}, ifaces, nil)
	now := time.Now()
	ev := iutil.NewEvent(now, "instance-started", "default", "web-1", "").WithInstance(inst, true)

	vhosts := extractVhosts(nil, "edge", []*iutil.Event{ev})
	require.Len(t, vhosts, 4)

	// 1. Primary domain vhost
	require.Equal(t, "example.com", vhosts[0].Domain)
	require.Equal(t, []string{"10.0.1.10:8080"}, vhosts[0].Upstreams)
	require.Empty(t, vhosts[0].Redirect)

	// 2. Redir with no-uri
	require.Equal(t, "www.example.com", vhosts[1].Domain)
	require.Equal(t, "https://example.com", vhosts[1].Redirect)
	require.Empty(t, vhosts[1].Upstreams)

	// 3. Redir with uri
	require.Equal(t, "old.example.com", vhosts[2].Domain)
	require.Equal(t, "https://example.com{uri}", vhosts[2].Redirect)
	require.Empty(t, vhosts[2].Upstreams)

	// 4. Redir with default uri
	require.Equal(t, "alias.example.com", vhosts[3].Domain)
	require.Equal(t, "https://example.com{uri}", vhosts[3].Redirect)
	require.Empty(t, vhosts[3].Upstreams)
}

func TestExtractVhostsRedirsDeduplicationAndSelfFilter(t *testing.T) {
	now := time.Now()
	ifaces1 := []iutil.InstanceInterface{
		iutil.NewInstanceInterface("default", "eth0", true, []string{"10.0.1.10"}, nil),
	}
	inst1 := iutil.NewInstance(true, map[string]string{
		"user.label.edge": "example.com,upstream=8080",
		// example.com matches primary domain and must be ignored; www.example.com is repeated
		"user.label.edge.redirs": "example.com,uri www.example.com",
	}, ifaces1, nil)
	ev1 := iutil.NewEvent(now, "instance-started", "default", "web-1", "").WithInstance(inst1, true)

	ifaces2 := []iutil.InstanceInterface{
		iutil.NewInstanceInterface("default", "eth0", true, []string{"10.0.1.11"}, nil),
	}
	inst2 := iutil.NewInstance(true, map[string]string{
		"user.label.edge":        "example.com,upstream=8080",
		"user.label.edge.redirs": "www.example.com",
	}, ifaces2, nil)
	ev2 := iutil.NewEvent(now, "instance-started", "default", "web-2", "").WithInstance(inst2, true)

	vhosts := extractVhosts(nil, "edge", []*iutil.Event{ev1, ev2})
	require.Len(t, vhosts, 2)

	require.Equal(t, "example.com", vhosts[0].Domain)
	require.Equal(t, []string{"10.0.1.10:8080", "10.0.1.11:8080"}, vhosts[0].Upstreams)
	require.Empty(t, vhosts[0].Redirect)

	require.Equal(t, "www.example.com", vhosts[1].Domain)
	require.Equal(t, "https://example.com{uri}", vhosts[1].Redirect)
}

func TestExtractVhostsRedirsWithExplicitRedirect(t *testing.T) {
	inst := iutil.NewInstance(true, map[string]string{
		"user.label.edge":        "old.com,redir=https://new.com{uri}",
		"user.label.edge.redirs": "www.old.com,no-uri",
	}, nil, nil)
	now := time.Now()
	ev := iutil.NewEvent(now, "instance-started", "default", "redir-1", "").WithInstance(inst, true)

	vhosts := extractVhosts(nil, "edge", []*iutil.Event{ev})
	require.Len(t, vhosts, 2)

	require.Equal(t, "old.com", vhosts[0].Domain)
	require.Equal(t, "https://new.com{uri}", vhosts[0].Redirect)

	require.Equal(t, "www.old.com", vhosts[1].Domain)
	require.Equal(t, "https://new.com", vhosts[1].Redirect)
}

func TestParseEntries(t *testing.T) {
	// Flags and key-value options.
	entries := parseEntries("example.com,flag1=val1,no-uri api.example.com,flag2=val2,uri")
	require.Len(t, entries, 2)

	require.Equal(t, "example.com", entries[0].Value)
	require.Equal(t, "val1", entries[0].Flags["flag1"])
	require.Equal(t, "false", entries[0].Flags["uri"])
	require.NotContains(t, entries[0].Flags, "no-uri")

	require.Equal(t, "api.example.com", entries[1].Value)
	require.Equal(t, "val2", entries[1].Flags["flag2"])
	require.Equal(t, "true", entries[1].Flags["uri"])

	// uri=true is same as uri, uri=false is same as no-uri.
	entries = parseEntries("example.com,uri=true old.example.com,uri=false")
	require.Len(t, entries, 2)
	require.Equal(t, "true", entries[0].Flags["uri"])
	require.Equal(t, "false", entries[1].Flags["uri"])

	// Any value-less flag with no- prefix has no- stripped.
	entries = parseEntries("example.com,no-cache")
	require.Len(t, entries, 1)
	require.Equal(t, "false", entries[0].Flags["cache"])
	require.NotContains(t, entries[0].Flags, "no-cache")

	// Duplicate flags: last input wins.
	entries = parseEntries("example.com,k=v1,k=v2,no-uri,uri")
	require.Len(t, entries, 1)
	require.Equal(t, "v2", entries[0].Flags["k"])
	require.Equal(t, "true", entries[0].Flags["uri"])
	require.NotContains(t, entries[0].Flags, "no-uri")

	entries = parseEntries("example.com,uri,no-uri")
	require.Len(t, entries, 1)
	require.Equal(t, "false", entries[0].Flags["uri"])

	entries = parseEntries("example.com,no-uri,uri=true")
	require.Len(t, entries, 1)
	require.Equal(t, "true", entries[0].Flags["uri"])

	// Quoted values with spaces and commas (single and double quotes).
	entries = parseEntries("example.com,resolvers='1.1.1.1 1.0.0.1',header=\"X-Forwarded-For: 1.2.3.4, 5.6.7.8\"")
	require.Len(t, entries, 1)
	require.Equal(t, "example.com", entries[0].Value)
	require.Equal(t, "1.1.1.1 1.0.0.1", entries[0].Flags["resolvers"])
	require.Equal(t, "X-Forwarded-For: 1.2.3.4, 5.6.7.8", entries[0].Flags["header"])

	// Quoted domain with space after comma before flag.
	entries = parseEntries("'docker-registry.home.jochum.dev', resolvers=\"1.1.1.1 1.0.0.1\"")
	require.Len(t, entries, 1)
	require.Equal(t, "docker-registry.home.jochum.dev", entries[0].Value)
	require.Equal(t, "1.1.1.1 1.0.0.1", entries[0].Flags["resolvers"])

	// Multiple entries with quoted flags separated by whitespace.
	entries = parseEntries("app1.test,resolvers='1.1.1.1 1.0.0.1' app2.test,resolvers=\"2.2.2.2 2.0.0.2\"")
	require.Len(t, entries, 2)
	require.Equal(t, "app1.test", entries[0].Value)
	require.Equal(t, "1.1.1.1 1.0.0.1", entries[0].Flags["resolvers"])
	require.Equal(t, "app2.test", entries[1].Value)
	require.Equal(t, "2.2.2.2 2.0.0.2", entries[1].Flags["resolvers"])

	// Quoted entries separated by commas (with and without space).
	entries = parseEntries("\"entry1,instance=i1,project=p1\", \"entry2,instance=i2,project=p2\"")
	require.Len(t, entries, 2)
	require.Equal(t, "entry1", entries[0].Value)
	require.Equal(t, "i1", entries[0].Flags["instance"])
	require.Equal(t, "p1", entries[0].Flags["project"])
	require.Equal(t, "entry2", entries[1].Value)
	require.Equal(t, "i2", entries[1].Flags["instance"])
	require.Equal(t, "p2", entries[1].Flags["project"])

	entries = parseEntries("'entry1,instance=i1,project=p1','entry2,instance=i2,project=p2'")
	require.Len(t, entries, 2)
	require.Equal(t, "entry1", entries[0].Value)
	require.Equal(t, "entry2", entries[1].Value)

	// Empty and punctuation only.
	require.Empty(t, parseEntries(""))
	require.Empty(t, parseEntries("  ,  , "))
}

func TestExtractVhostsFlagsAndDomainModifiers(t *testing.T) {
	now := time.Now()
	ifaces := []iutil.InstanceInterface{
		iutil.NewInstanceInterface("default", "eth0", true, []string{"10.0.1.10"}, nil),
	}

	// Instance 1: domain with flags, reverse proxy.
	inst1 := iutil.NewInstance(true, map[string]string{
		"user.label.edge": "example.com,upstream=8080,flag1=val1,flag2=val2",
	}, ifaces, nil)
	ev1 := iutil.NewEvent(now, "instance-started", "default", "web-1", "").WithInstance(inst1, true)

	// Instance 2: redirect with no-uri and flags.
	inst2 := iutil.NewInstance(true, map[string]string{
		"user.label.edge": "old.example.com,redir=https://new.example.com,no-uri,flag3=val3",
	}, nil, nil)
	ev2 := iutil.NewEvent(now, "instance-started", "default", "redir-1", "").WithInstance(inst2, true)

	// Instance 3: domain with no-uri affecting redirect.
	inst3 := iutil.NewInstance(true, map[string]string{
		"user.label.edge": "legacy.example.com,redir=https://new.example.com,no-uri",
	}, nil, nil)
	ev3 := iutil.NewEvent(now, "instance-started", "default", "redir-2", "").WithInstance(inst3, true)

	vhosts := extractVhosts(nil, "edge", []*iutil.Event{ev1, ev2, ev3})
	require.Len(t, vhosts, 3)

	// 1. example.com
	require.Equal(t, "example.com", vhosts[0].Domain)
	require.Equal(t, []string{"10.0.1.10:8080"}, vhosts[0].Upstreams)
	require.Equal(t, "val1", vhosts[0].Flags["flag1"])
	require.Equal(t, "val2", vhosts[0].Flags["flag2"])

	// 2. old.example.com -> stripped {uri} due to no-uri on redirect
	require.Equal(t, "old.example.com", vhosts[1].Domain)
	require.Equal(t, "https://new.example.com", vhosts[1].Redirect)
	require.Equal(t, "val3", vhosts[1].Flags["flag3"])
	require.Equal(t, "false", vhosts[1].Flags["uri"])
	require.NotContains(t, vhosts[1].Flags, "no-uri")

	// 3. legacy.example.com -> stripped {uri} due to no-uri on domain
	require.Equal(t, "legacy.example.com", vhosts[2].Domain)
	require.Equal(t, "https://new.example.com", vhosts[2].Redirect)
	require.Equal(t, "false", vhosts[2].Flags["uri"])
	require.NotContains(t, vhosts[2].Flags, "no-uri")

	// 4. docker-registry with quoted resolvers and registry flags.
	inst4 := iutil.NewInstance(true, map[string]string{
		"user.label.edge": "docker-registry.home.jochum.dev,upstream=8080,resolvers='1.1.1.1 1.0.0.1',registry=\"docker.io\"",
	}, ifaces, nil)
	ev4 := iutil.NewEvent(now, "instance-started", "default", "registry-1", "").WithInstance(inst4, true)

	vhostsWithQuotes := extractVhosts(nil, "edge", []*iutil.Event{ev4})
	require.Len(t, vhostsWithQuotes, 1)
	require.Equal(t, "docker-registry.home.jochum.dev", vhostsWithQuotes[0].Domain)
	require.Equal(t, "1.1.1.1 1.0.0.1", vhostsWithQuotes[0].Flags["resolvers"])
	require.Equal(t, "docker.io", vhostsWithQuotes[0].Flags["registry"])
}

func TestExtractVhostsCollectReplicasByService(t *testing.T) {
	now := time.Now()

	// api-1: specifies domain, upstream, and service
	ifaces1 := []iutil.InstanceInterface{
		iutil.NewInstanceInterface("default", "eth0", true, []string{"10.0.1.10"}, nil),
	}
	inst1 := iutil.NewInstance(true, map[string]string{
		"user.incus-compose.service": "api",
		"user.label.edge":            "api.example.com,upstream=8080",
	}, ifaces1, nil)
	ev1 := iutil.NewEvent(now, "instance-started", "default", "api-1", "").WithInstance(inst1, true)

	// api-2: replica only has service tag and own IP
	ifaces2 := []iutil.InstanceInterface{
		iutil.NewInstanceInterface("default", "eth0", true, []string{"10.0.1.11"}, nil),
	}
	inst2 := iutil.NewInstance(true, map[string]string{
		"user.incus-compose.service": "api",
	}, ifaces2, nil)
	ev2 := iutil.NewEvent(now, "instance-started", "default", "api-2", "").WithInstance(inst2, true)

	// api-3: replica only has service tag and own IP
	ifaces3 := []iutil.InstanceInterface{
		iutil.NewInstanceInterface("default", "eth0", true, []string{"10.0.1.12"}, nil),
	}
	inst3 := iutil.NewInstance(true, map[string]string{
		"user.incus-compose.service": "api",
	}, ifaces3, nil)
	ev3 := iutil.NewEvent(now, "instance-started", "default", "api-3", "").WithInstance(inst3, true)

	vhosts := extractVhosts(nil, "edge", []*iutil.Event{ev1, ev2, ev3})
	require.Len(t, vhosts, 1)

	require.Equal(t, "api.example.com", vhosts[0].Domain)
	require.Equal(t, "api", vhosts[0].Service)
	require.Equal(t, []string{"10.0.1.10:8080", "10.0.1.11:8080", "10.0.1.12:8080"}, vhosts[0].Upstreams)
}

func TestExtractVhostsServicePrefixOverride(t *testing.T) {
	now := time.Now()

	// srv-1: prefix service overrides compose service
	ifaces1 := []iutil.InstanceInterface{
		iutil.NewInstanceInterface("default", "eth0", true, []string{"10.0.1.20"}, nil),
	}
	inst1 := iutil.NewInstance(true, map[string]string{
		"user.label.edge.service":    "web",
		"user.incus-compose.service": "ignored",
		"user.label.edge":            "web.lan,upstream=3000",
	}, ifaces1, nil)
	ev1 := iutil.NewEvent(now, "instance-started", "default", "srv-1", "").WithInstance(inst1, true)

	// srv-2: replica matching edge.service
	ifaces2 := []iutil.InstanceInterface{
		iutil.NewInstanceInterface("default", "eth0", true, []string{"10.0.1.21"}, nil),
	}
	inst2 := iutil.NewInstance(true, map[string]string{
		"user.label.edge.service": "web",
	}, ifaces2, nil)
	ev2 := iutil.NewEvent(now, "instance-started", "default", "srv-2", "").WithInstance(inst2, true)

	vhosts := extractVhosts(nil, "edge", []*iutil.Event{ev1, ev2})
	require.Len(t, vhosts, 1)

	require.Equal(t, "web.lan", vhosts[0].Domain)
	require.Equal(t, "web", vhosts[0].Service)
	require.Equal(t, []string{"10.0.1.20:3000", "10.0.1.21:3000"}, vhosts[0].Upstreams)
}

func TestExtractVhostsStructuredBasic(t *testing.T) {
	now := time.Now()
	ifaces := []iutil.InstanceInterface{
		iutil.NewInstanceInterface("default", "eth0", true, []string{"10.0.1.50"}, nil),
	}
	inst := iutil.NewInstance(true, map[string]string{
		"user.incus-compose.service":           "wiki",
		"user.label.caddy-external.0.domain":   "example.com",
		"user.label.caddy-external.0.upstream": "8080",
		"user.label.caddy-external.0.redirs.0": "www.example.com,uri",
		"user.label.caddy-external.0.redirs.1": "wiki.example.com,uri",
		"user.label.caddy-external.0.redirs.2": "docs.example.com,uri",
		"user.label.caddy-internal.0.domain":   "example.com",
		"user.label.caddy-internal.0.upstream": "8080",
		"user.label.caddy-internal.0.template": "internal_acme.caddyfile",
		"user.label.caddy-internal.0.redirs.0": "www.example.com,uri,template=internal_acme.caddyfile",
		"user.label.caddy-internal.0.redirs.1": "wiki.example.com,uri,template=internal_acme.caddyfile",
		"user.label.caddy-internal.0.redirs.2": "docs.example.com,uri,template=internal_acme.caddyfile",
	}, ifaces, nil)
	ev := iutil.NewEvent(now, "instance-started", "default", "wiki-1", "").WithInstance(inst, true)

	// Verify caddy-external target
	vhostsExt := extractVhosts(nil, "caddy-external", []*iutil.Event{ev})
	require.Len(t, vhostsExt, 4)

	require.Equal(t, "example.com", vhostsExt[0].Domain)
	require.Equal(t, []string{"10.0.1.50:8080"}, vhostsExt[0].Upstreams)
	require.Empty(t, vhostsExt[0].Redirect)
	require.Empty(t, vhostsExt[0].Template)
	require.Equal(t, "wiki", vhostsExt[0].Service)

	require.Equal(t, "www.example.com", vhostsExt[1].Domain)
	require.Equal(t, "https://example.com{uri}", vhostsExt[1].Redirect)
	require.Empty(t, vhostsExt[1].Template)

	require.Equal(t, "wiki.example.com", vhostsExt[2].Domain)
	require.Equal(t, "https://example.com{uri}", vhostsExt[2].Redirect)

	require.Equal(t, "docs.example.com", vhostsExt[3].Domain)
	require.Equal(t, "https://example.com{uri}", vhostsExt[3].Redirect)

	// Verify caddy-internal target with template propagation
	vhostsInt := extractVhosts(nil, "caddy-internal", []*iutil.Event{ev})
	require.Len(t, vhostsInt, 4)

	require.Equal(t, "example.com", vhostsInt[0].Domain)
	require.Equal(t, []string{"10.0.1.50:8080"}, vhostsInt[0].Upstreams)
	require.Equal(t, "internal_acme.caddyfile", vhostsInt[0].Template)

	require.Equal(t, "www.example.com", vhostsInt[1].Domain)
	require.Equal(t, "https://example.com{uri}", vhostsInt[1].Redirect)
	require.Equal(t, "internal_acme.caddyfile", vhostsInt[1].Template)

	require.Equal(t, "wiki.example.com", vhostsInt[2].Domain)
	require.Equal(t, "https://example.com{uri}", vhostsInt[2].Redirect)
	require.Equal(t, "internal_acme.caddyfile", vhostsInt[2].Template)

	require.Equal(t, "docs.example.com", vhostsInt[3].Domain)
	require.Equal(t, "https://example.com{uri}", vhostsInt[3].Redirect)
	require.Equal(t, "internal_acme.caddyfile", vhostsInt[3].Template)
}

func TestExtractVhostsStructuredMultiRouteAndGaps(t *testing.T) {
	now := time.Now()
	ifaces := []iutil.InstanceInterface{
		iutil.NewInstanceInterface("default", "eth0", true, []string{"10.0.1.60"}, nil),
	}
	// Index 0 and 2, skipping 1.
	inst := iutil.NewInstance(true, map[string]string{
		"user.incus-compose.service": "app",
		"user.label.edge.0.domain":   "app.lan",
		"user.label.edge.0.upstream": "3000",
		"user.label.edge.2.domain":   "metrics.lan",
		"user.label.edge.2.upstream": "9090",
		"user.label.edge.2.redirs.0": "stats.lan,no-uri",
	}, ifaces, nil)
	ev := iutil.NewEvent(now, "instance-started", "default", "app-1", "").WithInstance(inst, true)

	vhosts := extractVhosts(nil, "edge", []*iutil.Event{ev})
	require.Len(t, vhosts, 3)

	require.Equal(t, "app.lan", vhosts[0].Domain)
	require.Equal(t, []string{"10.0.1.60:3000"}, vhosts[0].Upstreams)

	require.Equal(t, "metrics.lan", vhosts[1].Domain)
	require.Equal(t, []string{"10.0.1.60:9090"}, vhosts[1].Upstreams)

	require.Equal(t, "stats.lan", vhosts[2].Domain)
	require.Equal(t, "https://metrics.lan", vhosts[2].Redirect)
}

func TestExtractVhostsStructuredMissingDomain(t *testing.T) {
	now := time.Now()
	ifaces := []iutil.InstanceInterface{
		iutil.NewInstanceInterface("default", "eth0", true, []string{"10.0.1.70"}, nil),
	}
	inst := iutil.NewInstance(true, map[string]string{
		"user.label.edge.0.upstream": "8080",
		"user.label.edge.1.domain":   "valid.lan",
		"user.label.edge.1.upstream": "9090",
	}, ifaces, nil)
	ev := iutil.NewEvent(now, "instance-started", "default", "invalid-1", "").WithInstance(inst, true)

	var logBuf bytes.Buffer
	logger := slog.New(slog.NewTextHandler(&logBuf, nil))

	vhosts := extractVhosts(logger, "edge", []*iutil.Event{ev})
	require.Len(t, vhosts, 1)
	require.Equal(t, "valid.lan", vhosts[0].Domain)
	require.Equal(t, []string{"10.0.1.70:9090"}, vhosts[0].Upstreams)

	logOutput := logBuf.String()
	require.Contains(t, logOutput, "structured route missing domain; skipping route")
	require.Contains(t, logOutput, "invalid-1")
}

func TestExtractVhostsStructuredMixedFormatRejected(t *testing.T) {
	now := time.Now()
	ifaces := []iutil.InstanceInterface{
		iutil.NewInstanceInterface("default", "eth0", true, []string{"10.0.1.80"}, nil),
	}
	inst := iutil.NewInstance(true, map[string]string{
		"user.label.edge":          "legacy.lan,upstream=8080",
		"user.label.edge.0.domain": "structured.lan",
	}, ifaces, nil)
	ev := iutil.NewEvent(now, "instance-started", "default", "mixed-1", "").WithInstance(inst, true)

	var logBuf bytes.Buffer
	logger := slog.New(slog.NewTextHandler(&logBuf, nil))

	vhosts := extractVhosts(logger, "edge", []*iutil.Event{ev})
	require.Empty(t, vhosts)

	logOutput := logBuf.String()
	require.Contains(t, logOutput, "instance mixes legacy and structured caddy labels; skipping")
	require.Contains(t, logOutput, "mixed-1")
}

func TestExtractVhostsStructuredServiceReplicas(t *testing.T) {
	now := time.Now()

	// Replica 1 has structured route
	ifaces1 := []iutil.InstanceInterface{
		iutil.NewInstanceInterface("default", "eth0", true, []string{"10.0.1.91"}, nil),
	}
	inst1 := iutil.NewInstance(true, map[string]string{
		"user.incus-compose.service": "web",
		"user.label.edge.0.domain":   "web.example.com",
		"user.label.edge.0.upstream": "8080",
	}, ifaces1, nil)
	ev1 := iutil.NewEvent(now, "instance-started", "default", "web-1", "").WithInstance(inst1, true)

	// Replica 2 has only service and its own IP
	ifaces2 := []iutil.InstanceInterface{
		iutil.NewInstanceInterface("default", "eth0", true, []string{"10.0.1.92"}, nil),
	}
	inst2 := iutil.NewInstance(true, map[string]string{
		"user.incus-compose.service": "web",
	}, ifaces2, nil)
	ev2 := iutil.NewEvent(now, "instance-started", "default", "web-2", "").WithInstance(inst2, true)

	vhosts := extractVhosts(nil, "edge", []*iutil.Event{ev1, ev2})
	require.Len(t, vhosts, 1)
	require.Equal(t, "web.example.com", vhosts[0].Domain)
	require.Equal(t, []string{"10.0.1.91:8080", "10.0.1.92:8080"}, vhosts[0].Upstreams)
}

func TestExtractVhostsStructuredCoexistence(t *testing.T) {
	now := time.Now()

	// Instance 1 uses legacy format
	ifaces1 := []iutil.InstanceInterface{
		iutil.NewInstanceInterface("default", "eth0", true, []string{"10.0.1.101"}, nil),
	}
	inst1 := iutil.NewInstance(true, map[string]string{
		"user.incus-compose.service": "cluster",
		"user.label.edge":            "cluster.lan,upstream=4000",
	}, ifaces1, nil)
	ev1 := iutil.NewEvent(now, "instance-started", "default", "clust-1", "").WithInstance(inst1, true)

	// Instance 2 uses structured format
	ifaces2 := []iutil.InstanceInterface{
		iutil.NewInstanceInterface("default", "eth0", true, []string{"10.0.1.102"}, nil),
	}
	inst2 := iutil.NewInstance(true, map[string]string{
		"user.incus-compose.service": "cluster",
		"user.label.edge.0.domain":   "cluster.lan",
		"user.label.edge.0.upstream": "4000",
	}, ifaces2, nil)
	ev2 := iutil.NewEvent(now, "instance-started", "default", "clust-2", "").WithInstance(inst2, true)

	vhosts := extractVhosts(nil, "edge", []*iutil.Event{ev1, ev2})
	require.Len(t, vhosts, 1)
	require.Equal(t, "cluster.lan", vhosts[0].Domain)
	require.Equal(t, []string{"10.0.1.101:4000", "10.0.1.102:4000"}, vhosts[0].Upstreams)
}

func TestExtractVhostsStructuredRedirCanonical(t *testing.T) {
	now := time.Now()
	inst := iutil.NewInstance(true, map[string]string{
		"user.label.edge.0.domain": "old.lan",
		"user.label.edge.0.redir":  "https://new.lan{uri}",
	}, nil, nil)
	ev := iutil.NewEvent(now, "instance-started", "default", "redir-canonical", "").WithInstance(inst, true)

	vhosts := extractVhosts(nil, "edge", []*iutil.Event{ev})
	require.Len(t, vhosts, 1)
	require.Equal(t, "old.lan", vhosts[0].Domain)
	require.Equal(t, "https://new.lan{uri}", vhosts[0].Redirect)
	require.Empty(t, vhosts[0].Upstreams)
}
