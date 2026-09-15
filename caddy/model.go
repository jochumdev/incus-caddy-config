package caddy

import (
	"fmt"
	"net/netip"
	"slices"
	"strings"

	"github.com/lxc/incus-compose/ievent/iutil"
)

// Target represents a Caddy server destination bound to a specific route label.
type Target struct {
	Label    string
	Project  string
	Instance string
}

// ParseTarget parses a "label:project:instance" specification.
func ParseTarget(s string) (Target, error) {
	parts := strings.Split(s, ":")
	if len(parts) != 3 || parts[0] == "" || parts[1] == "" || parts[2] == "" {
		return Target{}, fmt.Errorf("invalid target %q: expected 'label:project:instance'", s)
	}

	return Target{
		Label:    parts[0],
		Project:  parts[1],
		Instance: parts[2],
	}, nil
}

// vhost holds the model data needed to render a Caddyfile site block.
type vhost struct {
	Domain    string
	Upstreams []string
	Redirect  string
	Template  string
}

// extractVhosts extracts and groups vhost routes for a target label from instance events.
func extractVhosts(targetLabel string, instances []*iutil.Event) []vhost {
	prefix := "user.label." + targetLabel + "."
	vhostMap := make(map[string]*vhost)
	order := make([]string, 0)

	for _, ev := range instances {
		inst := ev.Instance()
		if inst == nil || !inst.Running() {
			continue
		}

		domain, ok := inst.ConfigValue(prefix + "domain")
		if !ok || strings.TrimSpace(domain) == "" {
			continue
		}

		domain = strings.TrimSpace(domain)
		upstreamPort, _ := inst.ConfigValue(prefix + "upstream")
		upstreamPort = strings.TrimSpace(upstreamPort)

		network, _ := inst.ConfigValue(prefix + "network")
		network = strings.TrimSpace(network)

		redirect, _ := inst.ConfigValue(prefix + "redirect")
		redirect = strings.TrimSpace(redirect)

		tmpl, _ := inst.ConfigValue(prefix + "template")
		tmpl = strings.TrimSpace(tmpl)

		ip := resolveIPv4(inst, network)
		var upstream string
		if ip != "" {
			if upstreamPort != "" {
				if strings.Contains(upstreamPort, ":") {
					upstream = upstreamPort
				} else {
					upstream = fmt.Sprintf("%s:%s", ip, upstreamPort)
				}
			} else {
				upstream = ip
			}
		}

		existing, found := vhostMap[domain]
		if !found {
			v := &vhost{
				Domain:   domain,
				Redirect: redirect,
				Template: tmpl,
			}
			if upstream != "" {
				v.Upstreams = []string{upstream}
			}
			vhostMap[domain] = v
			order = append(order, domain)
		} else {
			if upstream != "" && !slices.Contains(existing.Upstreams, upstream) {
				existing.Upstreams = append(existing.Upstreams, upstream)
				slices.Sort(existing.Upstreams)
			}
			if existing.Redirect == "" && redirect != "" {
				existing.Redirect = redirect
			}
			if existing.Template == "" && tmpl != "" {
				existing.Template = tmpl
			}
		}
	}

	result := make([]vhost, 0, len(order))
	for _, key := range order {
		result = append(result, *vhostMap[key])
	}

	return result
}

// resolveIPv4 finds the IPv4 address for the specified network, or the first available IPv4.
func resolveIPv4(inst *iutil.Instance, targetNetwork string) string {
	if inst == nil {
		return ""
	}

	// 1. If targetNetwork is specified, match that network specifically.
	if targetNetwork != "" {
		for iface := range inst.Interfaces() {
			if iface.Network() == targetNetwork {
				for _, ip := range iface.IPv4() {
					parsed, err := netip.ParseAddr(ip)
					if err == nil && parsed.Is4() && !parsed.IsLoopback() {
						return parsed.String()
					}
				}
			}
		}
	}

	// 2. Otherwise, pick the first IPv4 of the first interface with a valid address.
	for iface := range inst.Interfaces() {
		for _, ip := range iface.IPv4() {
			parsed, err := netip.ParseAddr(ip)
			if err == nil && parsed.Is4() && !parsed.IsLoopback() {
				return parsed.String()
			}
		}
	}

	return ""
}
