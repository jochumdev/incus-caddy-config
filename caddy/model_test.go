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

func TestParseOSTarget(t *testing.T) {
	// Bare path defaults to label "caddy".
	target, err := ParseOSTarget("/etc/caddy/Caddyfile")
	require.NoError(t, err)
	require.Equal(t, OSTarget{Label: "caddy", Path: "/etc/caddy/Caddyfile"}, target)

	// Explicit label prefix.
	target, err = ParseOSTarget("edge:/etc/caddy/Caddyfile")
	require.NoError(t, err)
	require.Equal(t, OSTarget{Label: "edge", Path: "/etc/caddy/Caddyfile"}, target)

	// Whitespace trimming.
	target, err = ParseOSTarget("  myedge : /var/caddy/Caddyfile  ")
	require.NoError(t, err)
	require.Equal(t, OSTarget{Label: "myedge", Path: "/var/caddy/Caddyfile"}, target)

	// Windows drive letter without label prefix.
	target, err = ParseOSTarget(`C:\caddy\Caddyfile`)
	require.NoError(t, err)
	require.Equal(t, OSTarget{Label: "caddy", Path: `C:\caddy\Caddyfile`}, target)

	// Windows drive letter with forward slash.
	target, err = ParseOSTarget("D:/caddy/Caddyfile")
	require.NoError(t, err)
	require.Equal(t, OSTarget{Label: "caddy", Path: "D:/caddy/Caddyfile"}, target)

	// Windows drive letter with explicit label prefix.
	target, err = ParseOSTarget(`edge:C:\caddy\Caddyfile`)
	require.NoError(t, err)
	require.Equal(t, OSTarget{Label: "edge", Path: `C:\caddy\Caddyfile`}, target)

	// Invalid empty target.
	_, err = ParseOSTarget("")
	require.Error(t, err)

	_, err = ParseOSTarget("   ")
	require.Error(t, err)

	// Invalid empty path.
	_, err = ParseOSTarget("edge:")
	require.Error(t, err)

	_, err = ParseOSTarget(":")
	require.Error(t, err)
}

func TestParseGlobalTemplate(t *testing.T) {
	// Explicit prefix with file path.
	gt, err := ParseGlobalTemplate("caddy-external:/etc/caddy/external.caddyfile")
	require.NoError(t, err)
	require.Equal(t, GlobalTemplate{Label: "caddy-external", Template: "/etc/caddy/external.caddyfile"}, gt)

	// Explicit prefix with inline template.
	gt, err = ParseGlobalTemplate("internal:{\n\tadmin localhost:2019\n}")
	require.NoError(t, err)
	require.Equal(t, GlobalTemplate{Label: "internal", Template: "{\n\tadmin localhost:2019\n}"}, gt)

	// Explicit prefix with Windows drive letter path.
	gt, err = ParseGlobalTemplate(`edge:C:\caddy\global.caddyfile`)
	require.NoError(t, err)
	require.Equal(t, GlobalTemplate{Label: "edge", Template: `C:\caddy\global.caddyfile`}, gt)

	// Bare path without prefix must fail.
	_, err = ParseGlobalTemplate("/etc/caddy/global.caddyfile")
	require.Error(t, err)

	// Bare inline template containing ':' without prefix must fail.
	_, err = ParseGlobalTemplate("{\n\tadmin localhost:2019\n}")
	require.Error(t, err)

	// Windows drive letter without label prefix must fail.
	_, err = ParseGlobalTemplate(`C:\caddy\global.caddyfile`)
	require.Error(t, err)

	// Empty string.
	_, err = ParseGlobalTemplate("")
	require.Error(t, err)

	_, err = ParseGlobalTemplate("   ")
	require.Error(t, err)

	// Empty template with prefix.
	_, err = ParseGlobalTemplate("edge:")
	require.Error(t, err)

	_, err = ParseGlobalTemplate(":")
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
	require.Equal(t, "https://example.com{uri}", vhosts2[0].Redirect)
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
	require.Equal(t, "https://fallback.lan{uri}", vhosts[2].Redirect)
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

	// Empty and punctuation only.
	require.Empty(t, parseRedirs(""))
	require.Empty(t, parseRedirs("  ,  , "))
}

func TestExtractVhostsWithRedirs(t *testing.T) {
	ifaces := []iutil.InstanceInterface{
		iutil.NewInstanceInterface("default", "eth0", true, []string{"10.0.1.10"}, nil),
	}
	inst := iutil.NewInstance(true, map[string]string{
		"user.label.edge.domain":   "example.com",
		"user.label.edge.upstream": "8080",
		"user.label.edge.redirs":   "www.example.com,no-uri old.example.com,uri alias.example.com",
	}, ifaces, nil)
	now := time.Now()
	ev := iutil.NewEvent(now, "instance-started", "default", "web-1", "").WithInstance(inst, true)

	vhosts := extractVhosts("edge", []*iutil.Event{ev})
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
		"user.label.edge.domain":   "example.com",
		"user.label.edge.upstream": "8080",
		// example.com matches primary domain and must be ignored; www.example.com is repeated
		"user.label.edge.redirs": "example.com,uri www.example.com",
	}, ifaces1, nil)
	ev1 := iutil.NewEvent(now, "instance-started", "default", "web-1", "").WithInstance(inst1, true)

	ifaces2 := []iutil.InstanceInterface{
		iutil.NewInstanceInterface("default", "eth0", true, []string{"10.0.1.11"}, nil),
	}
	inst2 := iutil.NewInstance(true, map[string]string{
		"user.label.edge.domain":   "example.com",
		"user.label.edge.upstream": "8080",
		"user.label.edge.redirs":   "www.example.com",
	}, ifaces2, nil)
	ev2 := iutil.NewEvent(now, "instance-started", "default", "web-2", "").WithInstance(inst2, true)

	vhosts := extractVhosts("edge", []*iutil.Event{ev1, ev2})
	require.Len(t, vhosts, 2)

	require.Equal(t, "example.com", vhosts[0].Domain)
	require.Equal(t, []string{"10.0.1.10:8080", "10.0.1.11:8080"}, vhosts[0].Upstreams)
	require.Empty(t, vhosts[0].Redirect)

	require.Equal(t, "www.example.com", vhosts[1].Domain)
	require.Equal(t, "https://example.com{uri}", vhosts[1].Redirect)
}

func TestExtractVhostsRedirsWithExplicitRedirect(t *testing.T) {
	inst := iutil.NewInstance(true, map[string]string{
		"user.label.edge.domain":   "old.com",
		"user.label.edge.redirect": "https://new.com{uri}",
		"user.label.edge.redirs":   "www.old.com,no-uri",
	}, nil, nil)
	now := time.Now()
	ev := iutil.NewEvent(now, "instance-started", "default", "redir-1", "").WithInstance(inst, true)

	vhosts := extractVhosts("edge", []*iutil.Event{ev})
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
	require.False(t, entries[0].IncludeURI)
	require.Equal(t, "val1", entries[0].Flags["flag1"])
	require.Equal(t, "true", entries[0].Flags["no-uri"])

	require.Equal(t, "api.example.com", entries[1].Value)
	require.True(t, entries[1].IncludeURI)
	require.Equal(t, "val2", entries[1].Flags["flag2"])
	require.Equal(t, "true", entries[1].Flags["uri"])

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
		"user.label.edge.domain":   "example.com,flag1=val1,flag2=val2",
		"user.label.edge.upstream": "8080",
	}, ifaces, nil)
	ev1 := iutil.NewEvent(now, "instance-started", "default", "web-1", "").WithInstance(inst1, true)

	// Instance 2: redirect with no-uri and flags.
	inst2 := iutil.NewInstance(true, map[string]string{
		"user.label.edge.domain":   "old.example.com",
		"user.label.edge.redirect": "https://new.example.com,no-uri,flag3=val3",
	}, nil, nil)
	ev2 := iutil.NewEvent(now, "instance-started", "default", "redir-1", "").WithInstance(inst2, true)

	// Instance 3: domain with no-uri affecting redirect.
	inst3 := iutil.NewInstance(true, map[string]string{
		"user.label.edge.domain":   "legacy.example.com,no-uri",
		"user.label.edge.redirect": "https://new.example.com",
	}, nil, nil)
	ev3 := iutil.NewEvent(now, "instance-started", "default", "redir-2", "").WithInstance(inst3, true)

	vhosts := extractVhosts("edge", []*iutil.Event{ev1, ev2, ev3})
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
	require.Equal(t, "true", vhosts[1].Flags["no-uri"])

	// 3. legacy.example.com -> stripped {uri} due to no-uri on domain
	require.Equal(t, "legacy.example.com", vhosts[2].Domain)
	require.Equal(t, "https://new.example.com", vhosts[2].Redirect)
	require.Equal(t, "true", vhosts[2].Flags["no-uri"])
}

func TestExtractVhostsCollectReplicasByService(t *testing.T) {
	now := time.Now()

	// api-1: specifies domain, upstream, and service
	ifaces1 := []iutil.InstanceInterface{
		iutil.NewInstanceInterface("default", "eth0", true, []string{"10.0.1.10"}, nil),
	}
	inst1 := iutil.NewInstance(true, map[string]string{
		"user.incus-compose.service": "api",
		"user.label.edge.domain":     "api.example.com",
		"user.label.edge.upstream":   "8080",
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

	vhosts := extractVhosts("edge", []*iutil.Event{ev1, ev2, ev3})
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
		"user.label.edge.domain":     "web.lan",
		"user.label.edge.upstream":   "3000",
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

	vhosts := extractVhosts("edge", []*iutil.Event{ev1, ev2})
	require.Len(t, vhosts, 1)

	require.Equal(t, "web.lan", vhosts[0].Domain)
	require.Equal(t, "web", vhosts[0].Service)
	require.Equal(t, []string{"10.0.1.20:3000", "10.0.1.21:3000"}, vhosts[0].Upstreams)
}
