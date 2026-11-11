package caddy

import (
	"bytes"
	"log/slog"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/jochumdev/incus-templated/templated"
)

func TestExtractVhosts(t *testing.T) {
	ifaces1 := []templated.Interface{
		{Project: "default", Network: "incusbr0", IPv4: []string{"10.0.1.5"}, IPv6: nil},
	}
	inst1 := templated.Instance{Config: map[string]string{
		"user.label.caddy1": "git.example.com,upstream=3000",
	}, Interfaces: ifaces1}
	ev1 := named(inst1, "default", "git-1")

	// Replica 2 of git service (using legacy .domain to verify backward compatibility)
	ifaces2 := []templated.Interface{
		{Project: "default", Network: "incusbr0", IPv4: []string{"10.0.1.6"}, IPv6: nil},
	}
	inst2 := templated.Instance{Config: map[string]string{
		"user.label.caddy1.domain": "git.example.com,upstream=3000",
	}, Interfaces: ifaces2}
	ev2 := named(inst2, "default", "git-2")

	// Instance with different label prefix caddy2
	ifaces3 := []templated.Interface{
		{Project: "default", Network: "incusbr0", IPv4: []string{"10.0.1.7"}, IPv6: nil},
	}
	inst3 := templated.Instance{Config: map[string]string{
		"user.label.caddy2": "other.lan,redir=https://example.com",
	}, Interfaces: ifaces3}
	ev3 := named(inst3, "default", "other-1")

	events := []templated.Instance{ev1, ev2, ev3}

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
	ifaces1 := []templated.Interface{
		{Project: "default", Network: "eth0", IPv4: []string{"10.0.1.10"}, IPv6: nil},
		{Project: "default", Network: "internal", IPv4: []string{"192.168.10.20"}, IPv6: nil},
	}
	inst1 := templated.Instance{Config: map[string]string{
		"user.incus-compose.service": "api",
		"user.label.edge":            "api.example.com,upstream=8080,network=internal",
	}, Interfaces: ifaces1}
	ev1 := named(inst1, "default", "api-1")

	// Replica 2 shares service and inherits network flag
	ifaces2 := []templated.Interface{
		{Project: "default", Network: "eth0", IPv4: []string{"10.0.1.11"}, IPv6: nil},
		{Project: "default", Network: "internal", IPv4: []string{"192.168.10.21"}, IPv6: nil},
	}
	inst2 := templated.Instance{Config: map[string]string{
		"user.incus-compose.service": "api",
	}, Interfaces: ifaces2}
	ev2 := named(inst2, "default", "api-2")

	vhosts := extractVhosts(nil, "edge", []templated.Instance{ev1, ev2})
	require.Len(t, vhosts, 1)
	require.Equal(t, "api.example.com", vhosts[0].Domain)
	require.Equal(t, []string{"192.168.10.20:8080", "192.168.10.21:8080"}, vhosts[0].Upstreams)
	require.Equal(t, "internal", vhosts[0].Flags["network"])
}

func TestExtractVhostsEdgeCases(t *testing.T) {
	// Nil and stopped instances never reach the funcs; templated drops them.

	// 3. Instance with missing or whitespace-only domain.
	instNoDomain := templated.Instance{Config: map[string]string{
		"user.label.caddy.service": "no-domain",
	}}
	evNoDomain := named(instNoDomain, "default", "no-domain")

	instWhitespaceDomain := templated.Instance{Config: map[string]string{
		"user.label.caddy": "   ",
	}}
	evWhitespaceDomain := named(instWhitespaceDomain, "default", "white-domain")

	// 4. Instance with host:port in upstream label.
	ifaces := []templated.Interface{
		{Project: "default", Network: "eth0", IPv4: []string{"10.0.0.1"}, IPv6: nil},
	}
	instHostPort := templated.Instance{Config: map[string]string{
		"user.label.caddy": "custom.lan,upstream=192.168.50.1:9090",
	}, Interfaces: ifaces}
	evHostPort := named(instHostPort, "default", "custom")

	// 5. Instance with empty upstream port -> defaults to IP.
	instBareIP := templated.Instance{Config: map[string]string{
		"user.label.caddy": "bare.lan",
	}, Interfaces: ifaces}
	evBareIP := named(instBareIP, "default", "bare")

	// 6. Instance with no IP addresses -> upstream remains empty.
	instNoIP := templated.Instance{Config: map[string]string{
		"user.label.caddy": "noip.lan,redir=https://fallback.lan",
	}}
	evNoIP := named(instNoIP, "default", "noip")

	events := []templated.Instance{evNoDomain, evWhitespaceDomain, evHostPort, evBareIP, evNoIP}
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
	// Instance 1: sets domain and upstream, no redirect, no template.
	ifaces1 := []templated.Interface{
		{Project: "default", Network: "eth0", IPv4: []string{"10.0.0.1"}, IPv6: nil},
	}
	inst1 := templated.Instance{Config: map[string]string{
		"user.label.caddy": "multi.lan,upstream=8080",
	}, Interfaces: ifaces1}
	ev1 := named(inst1, "default", "inst-1")

	// Instance 2: same domain, different upstream, adds redirect and template.
	ifaces2 := []templated.Interface{
		{Project: "default", Network: "eth0", IPv4: []string{"10.0.0.2"}, IPv6: nil},
	}
	inst2 := templated.Instance{Config: map[string]string{
		"user.label.caddy": "multi.lan,upstream=8080,redir=https://multi.com,template=custom_tmpl",
	}, Interfaces: ifaces2}
	ev2 := named(inst2, "default", "inst-2")

	// Instance 3: same domain, duplicate upstream of inst1, attempt to overwrite redirect and template.
	inst3 := templated.Instance{Config: map[string]string{
		"user.label.caddy": "multi.lan,upstream=8080,redir=https://overwrite.com,template=overwrite_tmpl",
	}, Interfaces: ifaces1}
	ev3 := named(inst3, "default", "inst-3")

	vhosts := extractVhosts(nil, "caddy", []templated.Instance{ev1, ev2, ev3})
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
	ifaces := []templated.Interface{
		{Project: "default", Network: "eth0", IPv4: []string{"10.0.1.10"}, IPv6: nil},
	}
	inst := templated.Instance{Config: map[string]string{
		"user.label.edge":        "example.com,upstream=8080",
		"user.label.edge.redirs": "old.example.com,template=custom_redir.caddyfile,no-uri default.example.com",
	}, Interfaces: ifaces}
	ev := named(inst, "default", "web-1")

	vhosts := extractVhosts(nil, "edge", []templated.Instance{ev})
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
	ifaces := []templated.Interface{
		{Project: "default", Network: "eth0", IPv4: []string{"10.0.1.10"}, IPv6: nil},
	}
	inst := templated.Instance{Config: map[string]string{
		"user.label.edge":        "example.com,upstream=8080",
		"user.label.edge.redirs": "www.example.com,no-uri old.example.com,uri alias.example.com",
	}, Interfaces: ifaces}
	ev := named(inst, "default", "web-1")

	vhosts := extractVhosts(nil, "edge", []templated.Instance{ev})
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
	ifaces1 := []templated.Interface{
		{Project: "default", Network: "eth0", IPv4: []string{"10.0.1.10"}, IPv6: nil},
	}
	inst1 := templated.Instance{Config: map[string]string{
		"user.label.edge": "example.com,upstream=8080",
		// example.com matches primary domain and must be ignored; www.example.com is repeated
		"user.label.edge.redirs": "example.com,uri www.example.com",
	}, Interfaces: ifaces1}
	ev1 := named(inst1, "default", "web-1")

	ifaces2 := []templated.Interface{
		{Project: "default", Network: "eth0", IPv4: []string{"10.0.1.11"}, IPv6: nil},
	}
	inst2 := templated.Instance{Config: map[string]string{
		"user.label.edge":        "example.com,upstream=8080",
		"user.label.edge.redirs": "www.example.com",
	}, Interfaces: ifaces2}
	ev2 := named(inst2, "default", "web-2")

	vhosts := extractVhosts(nil, "edge", []templated.Instance{ev1, ev2})
	require.Len(t, vhosts, 2)

	require.Equal(t, "example.com", vhosts[0].Domain)
	require.Equal(t, []string{"10.0.1.10:8080", "10.0.1.11:8080"}, vhosts[0].Upstreams)
	require.Empty(t, vhosts[0].Redirect)

	require.Equal(t, "www.example.com", vhosts[1].Domain)
	require.Equal(t, "https://example.com{uri}", vhosts[1].Redirect)
}

func TestExtractVhostsRedirsWithExplicitRedirect(t *testing.T) {
	inst := templated.Instance{Config: map[string]string{
		"user.label.edge":        "old.com,redir=https://new.com{uri}",
		"user.label.edge.redirs": "www.old.com,no-uri",
	}}
	ev := named(inst, "default", "redir-1")

	vhosts := extractVhosts(nil, "edge", []templated.Instance{ev})
	require.Len(t, vhosts, 2)

	require.Equal(t, "old.com", vhosts[0].Domain)
	require.Equal(t, "https://new.com{uri}", vhosts[0].Redirect)

	require.Equal(t, "www.old.com", vhosts[1].Domain)
	require.Equal(t, "https://new.com", vhosts[1].Redirect)
}

func TestExtractVhostsFlagsAndDomainModifiers(t *testing.T) {
	ifaces := []templated.Interface{
		{Project: "default", Network: "eth0", IPv4: []string{"10.0.1.10"}, IPv6: nil},
	}

	// Instance 1: domain with flags, reverse proxy.
	inst1 := templated.Instance{Config: map[string]string{
		"user.label.edge": "example.com,upstream=8080,flag1=val1,flag2=val2",
	}, Interfaces: ifaces}
	ev1 := named(inst1, "default", "web-1")

	// Instance 2: redirect with no-uri and flags.
	inst2 := templated.Instance{Config: map[string]string{
		"user.label.edge": "old.example.com,redir=https://new.example.com,no-uri,flag3=val3",
	}}
	ev2 := named(inst2, "default", "redir-1")

	// Instance 3: domain with no-uri affecting redirect.
	inst3 := templated.Instance{Config: map[string]string{
		"user.label.edge": "legacy.example.com,redir=https://new.example.com,no-uri",
	}}
	ev3 := named(inst3, "default", "redir-2")

	vhosts := extractVhosts(nil, "edge", []templated.Instance{ev1, ev2, ev3})
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
	inst4 := templated.Instance{Config: map[string]string{
		"user.label.edge": "docker-registry.home.jochum.dev,upstream=8080,resolvers='1.1.1.1 1.0.0.1',registry=\"docker.io\"",
	}, Interfaces: ifaces}
	ev4 := named(inst4, "default", "registry-1")

	vhostsWithQuotes := extractVhosts(nil, "edge", []templated.Instance{ev4})
	require.Len(t, vhostsWithQuotes, 1)
	require.Equal(t, "docker-registry.home.jochum.dev", vhostsWithQuotes[0].Domain)
	require.Equal(t, "1.1.1.1 1.0.0.1", vhostsWithQuotes[0].Flags["resolvers"])
	require.Equal(t, "docker.io", vhostsWithQuotes[0].Flags["registry"])
}

func TestExtractVhostsCollectReplicasByService(t *testing.T) {
	// api-1: specifies domain, upstream, and service
	ifaces1 := []templated.Interface{
		{Project: "default", Network: "eth0", IPv4: []string{"10.0.1.10"}, IPv6: nil},
	}
	inst1 := templated.Instance{Config: map[string]string{
		"user.incus-compose.service": "api",
		"user.label.edge":            "api.example.com,upstream=8080",
	}, Interfaces: ifaces1}
	ev1 := named(inst1, "default", "api-1")

	// api-2: replica only has service tag and own IP
	ifaces2 := []templated.Interface{
		{Project: "default", Network: "eth0", IPv4: []string{"10.0.1.11"}, IPv6: nil},
	}
	inst2 := templated.Instance{Config: map[string]string{
		"user.incus-compose.service": "api",
	}, Interfaces: ifaces2}
	ev2 := named(inst2, "default", "api-2")

	// api-3: replica only has service tag and own IP
	ifaces3 := []templated.Interface{
		{Project: "default", Network: "eth0", IPv4: []string{"10.0.1.12"}, IPv6: nil},
	}
	inst3 := templated.Instance{Config: map[string]string{
		"user.incus-compose.service": "api",
	}, Interfaces: ifaces3}
	ev3 := named(inst3, "default", "api-3")

	vhosts := extractVhosts(nil, "edge", []templated.Instance{ev1, ev2, ev3})
	require.Len(t, vhosts, 1)

	require.Equal(t, "api.example.com", vhosts[0].Domain)
	require.Equal(t, "api", vhosts[0].Service)
	require.Equal(t, []string{"10.0.1.10:8080", "10.0.1.11:8080", "10.0.1.12:8080"}, vhosts[0].Upstreams)
}

func TestExtractVhostsServicePrefixOverride(t *testing.T) {
	// srv-1: prefix service overrides compose service
	ifaces1 := []templated.Interface{
		{Project: "default", Network: "eth0", IPv4: []string{"10.0.1.20"}, IPv6: nil},
	}
	inst1 := templated.Instance{Config: map[string]string{
		"user.label.edge.service":    "web",
		"user.incus-compose.service": "ignored",
		"user.label.edge":            "web.lan,upstream=3000",
	}, Interfaces: ifaces1}
	ev1 := named(inst1, "default", "srv-1")

	// srv-2: replica matching edge.service
	ifaces2 := []templated.Interface{
		{Project: "default", Network: "eth0", IPv4: []string{"10.0.1.21"}, IPv6: nil},
	}
	inst2 := templated.Instance{Config: map[string]string{
		"user.label.edge.service": "web",
	}, Interfaces: ifaces2}
	ev2 := named(inst2, "default", "srv-2")

	vhosts := extractVhosts(nil, "edge", []templated.Instance{ev1, ev2})
	require.Len(t, vhosts, 1)

	require.Equal(t, "web.lan", vhosts[0].Domain)
	require.Equal(t, "web", vhosts[0].Service)
	require.Equal(t, []string{"10.0.1.20:3000", "10.0.1.21:3000"}, vhosts[0].Upstreams)
}

func TestExtractVhostsStructuredBasic(t *testing.T) {
	ifaces := []templated.Interface{
		{Project: "default", Network: "eth0", IPv4: []string{"10.0.1.50"}, IPv6: nil},
	}
	inst := templated.Instance{Config: map[string]string{
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
	}, Interfaces: ifaces}
	ev := named(inst, "default", "wiki-1")

	// Verify caddy-external target
	vhostsExt := extractVhosts(nil, "caddy-external", []templated.Instance{ev})
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
	vhostsInt := extractVhosts(nil, "caddy-internal", []templated.Instance{ev})
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
	ifaces := []templated.Interface{
		{Project: "default", Network: "eth0", IPv4: []string{"10.0.1.60"}, IPv6: nil},
	}
	// Index 0 and 2, skipping 1.
	inst := templated.Instance{Config: map[string]string{
		"user.incus-compose.service": "app",
		"user.label.edge.0.domain":   "app.lan",
		"user.label.edge.0.upstream": "3000",
		"user.label.edge.2.domain":   "metrics.lan",
		"user.label.edge.2.upstream": "9090",
		"user.label.edge.2.redirs.0": "stats.lan,no-uri",
	}, Interfaces: ifaces}
	ev := named(inst, "default", "app-1")

	vhosts := extractVhosts(nil, "edge", []templated.Instance{ev})
	require.Len(t, vhosts, 3)

	require.Equal(t, "app.lan", vhosts[0].Domain)
	require.Equal(t, []string{"10.0.1.60:3000"}, vhosts[0].Upstreams)

	require.Equal(t, "metrics.lan", vhosts[1].Domain)
	require.Equal(t, []string{"10.0.1.60:9090"}, vhosts[1].Upstreams)

	require.Equal(t, "stats.lan", vhosts[2].Domain)
	require.Equal(t, "https://metrics.lan", vhosts[2].Redirect)
}

func TestExtractVhostsStructuredMissingDomain(t *testing.T) {
	ifaces := []templated.Interface{
		{Project: "default", Network: "eth0", IPv4: []string{"10.0.1.70"}, IPv6: nil},
	}
	inst := templated.Instance{Config: map[string]string{
		"user.label.edge.0.upstream": "8080",
		"user.label.edge.1.domain":   "valid.lan",
		"user.label.edge.1.upstream": "9090",
	}, Interfaces: ifaces}
	ev := named(inst, "default", "invalid-1")

	var logBuf bytes.Buffer
	logger := slog.New(slog.NewTextHandler(&logBuf, nil))

	vhosts := extractVhosts(logger, "edge", []templated.Instance{ev})
	require.Len(t, vhosts, 1)
	require.Equal(t, "valid.lan", vhosts[0].Domain)
	require.Equal(t, []string{"10.0.1.70:9090"}, vhosts[0].Upstreams)

	logOutput := logBuf.String()
	require.Contains(t, logOutput, "structured route missing domain; skipping route")
	require.Contains(t, logOutput, "invalid-1")
}

func TestExtractVhostsStructuredMixedFormatRejected(t *testing.T) {
	ifaces := []templated.Interface{
		{Project: "default", Network: "eth0", IPv4: []string{"10.0.1.80"}, IPv6: nil},
	}
	inst := templated.Instance{Config: map[string]string{
		"user.label.edge":          "legacy.lan,upstream=8080",
		"user.label.edge.0.domain": "structured.lan",
	}, Interfaces: ifaces}
	ev := named(inst, "default", "mixed-1")

	var logBuf bytes.Buffer
	logger := slog.New(slog.NewTextHandler(&logBuf, nil))

	vhosts := extractVhosts(logger, "edge", []templated.Instance{ev})
	require.Empty(t, vhosts)

	logOutput := logBuf.String()
	require.Contains(t, logOutput, "instance mixes legacy and structured caddy labels; skipping")
	require.Contains(t, logOutput, "mixed-1")
}

func TestExtractVhostsStructuredServiceReplicas(t *testing.T) {
	// Replica 1 has structured route
	ifaces1 := []templated.Interface{
		{Project: "default", Network: "eth0", IPv4: []string{"10.0.1.91"}, IPv6: nil},
	}
	inst1 := templated.Instance{Config: map[string]string{
		"user.incus-compose.service": "web",
		"user.label.edge.0.domain":   "web.example.com",
		"user.label.edge.0.upstream": "8080",
	}, Interfaces: ifaces1}
	ev1 := named(inst1, "default", "web-1")

	// Replica 2 has only service and its own IP
	ifaces2 := []templated.Interface{
		{Project: "default", Network: "eth0", IPv4: []string{"10.0.1.92"}, IPv6: nil},
	}
	inst2 := templated.Instance{Config: map[string]string{
		"user.incus-compose.service": "web",
	}, Interfaces: ifaces2}
	ev2 := named(inst2, "default", "web-2")

	vhosts := extractVhosts(nil, "edge", []templated.Instance{ev1, ev2})
	require.Len(t, vhosts, 1)
	require.Equal(t, "web.example.com", vhosts[0].Domain)
	require.Equal(t, []string{"10.0.1.91:8080", "10.0.1.92:8080"}, vhosts[0].Upstreams)
}

func TestExtractVhostsStructuredCoexistence(t *testing.T) {
	// Instance 1 uses legacy format
	ifaces1 := []templated.Interface{
		{Project: "default", Network: "eth0", IPv4: []string{"10.0.1.101"}, IPv6: nil},
	}
	inst1 := templated.Instance{Config: map[string]string{
		"user.incus-compose.service": "cluster",
		"user.label.edge":            "cluster.lan,upstream=4000",
	}, Interfaces: ifaces1}
	ev1 := named(inst1, "default", "clust-1")

	// Instance 2 uses structured format
	ifaces2 := []templated.Interface{
		{Project: "default", Network: "eth0", IPv4: []string{"10.0.1.102"}, IPv6: nil},
	}
	inst2 := templated.Instance{Config: map[string]string{
		"user.incus-compose.service": "cluster",
		"user.label.edge.0.domain":   "cluster.lan",
		"user.label.edge.0.upstream": "4000",
	}, Interfaces: ifaces2}
	ev2 := named(inst2, "default", "clust-2")

	vhosts := extractVhosts(nil, "edge", []templated.Instance{ev1, ev2})
	require.Len(t, vhosts, 1)
	require.Equal(t, "cluster.lan", vhosts[0].Domain)
	require.Equal(t, []string{"10.0.1.101:4000", "10.0.1.102:4000"}, vhosts[0].Upstreams)
}

func TestExtractVhostsStructuredRedirCanonical(t *testing.T) {
	inst := templated.Instance{Config: map[string]string{
		"user.label.edge.0.domain": "old.lan",
		"user.label.edge.0.redir":  "https://new.lan{uri}",
	}}
	ev := named(inst, "default", "redir-canonical")

	vhosts := extractVhosts(nil, "edge", []templated.Instance{ev})
	require.Len(t, vhosts, 1)
	require.Equal(t, "old.lan", vhosts[0].Domain)
	require.Equal(t, "https://new.lan{uri}", vhosts[0].Redirect)
	require.Empty(t, vhosts[0].Upstreams)
}

func TestExtractVhostsHostPortWithoutIP(t *testing.T) {
	// 1. Legacy label format with host:port upstream and no network interfaces / IP.
	instLegacy := templated.Instance{Config: map[string]string{
		"user.label.caddy": "app.example.com,upstream=192.0.2.10:8080",
	}}
	evLegacy := named(instLegacy, "default", "app-legacy")

	// 2. Structured label format with host:port upstream and no network interfaces / IP.
	instStructured := templated.Instance{Config: map[string]string{
		"user.label.caddy.0.domain":   "structured.example.com",
		"user.label.caddy.0.upstream": "192.0.2.11:8080",
	}}
	evStructured := named(instStructured, "default", "app-struct")

	vhosts := extractVhosts(nil, "caddy", []templated.Instance{evLegacy, evStructured})
	require.Len(t, vhosts, 2)

	require.Equal(t, "app.example.com", vhosts[0].Domain)
	require.Equal(t, []string{"192.0.2.10:8080"}, vhosts[0].Upstreams)

	require.Equal(t, "structured.example.com", vhosts[1].Domain)
	require.Equal(t, []string{"192.0.2.11:8080"}, vhosts[1].Upstreams)
}

func named(inst templated.Instance, project, name string) templated.Instance {
	inst.Project = project
	inst.Name = name

	return inst
}
