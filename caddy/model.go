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

// OSTarget represents a local filesystem Caddyfile target on the same host or container.
type OSTarget struct {
	Label string
	Path  string
}

// ParseOSTarget parses a "[label:]path" specification, defaulting to label "caddy" if omitted.
func ParseOSTarget(s string) (OSTarget, error) {
	s = strings.TrimSpace(s)
	if s == "" {
		return OSTarget{}, fmt.Errorf("empty OS target")
	}

	idx := strings.Index(s, ":")
	if idx == -1 {
		return OSTarget{Label: "caddy", Path: s}, nil
	}

	// Windows drive letter check (e.g. C:\... or C:/...)
	if idx == 1 && len(s) > 2 && (s[2] == '\\' || s[2] == '/') && ((s[0] >= 'a' && s[0] <= 'z') || (s[0] >= 'A' && s[0] <= 'Z')) {
		return OSTarget{Label: "caddy", Path: s}, nil
	}

	label := strings.TrimSpace(s[:idx])
	path := strings.TrimSpace(s[idx+1:])
	if label == "" {
		label = "caddy"
	}

	if path == "" {
		return OSTarget{}, fmt.Errorf("invalid OS target %q: empty path", s)
	}

	return OSTarget{Label: label, Path: path}, nil
}

// GlobalTemplate represents a global options block template bound to a target label.
type GlobalTemplate struct {
	Label    string
	Template string
}

// ParseGlobalTemplate parses a "label:template" specification.
func ParseGlobalTemplate(s string) (GlobalTemplate, error) {
	s = strings.TrimSpace(s)
	if s == "" {
		return GlobalTemplate{}, fmt.Errorf("empty global template")
	}

	idx := strings.Index(s, ":")
	if idx == -1 {
		return GlobalTemplate{}, fmt.Errorf("invalid global template %q: expected 'label:template'", s)
	}

	// Reject bare Windows drive letter without label (e.g. C:\... or C:/...)
	if idx == 1 && len(s) > 2 && (s[2] == '\\' || s[2] == '/') && ((s[0] >= 'a' && s[0] <= 'z') || (s[0] >= 'A' && s[0] <= 'Z')) {
		return GlobalTemplate{}, fmt.Errorf("invalid global template %q: expected 'label:template'", s)
	}

	// Reject inline template block without label prefix
	braceIdx := strings.Index(s, "{")
	if braceIdx != -1 && braceIdx < idx {
		return GlobalTemplate{}, fmt.Errorf("invalid global template %q: expected 'label:template'", s)
	}

	label := strings.TrimSpace(s[:idx])
	tmpl := strings.TrimSpace(s[idx+1:])
	if label == "" || tmpl == "" {
		return GlobalTemplate{}, fmt.Errorf("invalid global template %q: expected 'label:template'", s)
	}

	return GlobalTemplate{Label: label, Template: tmpl}, nil
}

// vhost holds the model data needed to render a Caddyfile site block.
type vhost struct {
	Domain    string
	Service   string
	Upstreams []string
	Redirect  string
	Template  string
	Flags     map[string]string
}

// parsedEntry represents a parsed domain or target with associated flags.
type parsedEntry struct {
	Value      string
	Flags      map[string]string
	IncludeURI bool
}

// parseEntries parses whitespace- or comma-separated entries with optional comma-separated flags.
func parseEntries(raw string) []parsedEntry {
	var entries []parsedEntry
	tokens := strings.Fields(raw)

	for _, token := range tokens {
		token = strings.Trim(token, ",")
		if token == "" {
			continue
		}

		parts := strings.Split(token, ",")
		currentValue := strings.TrimSpace(parts[0])
		if currentValue == "" {
			continue
		}

		currentFlags := make(map[string]string)
		currentIncludeURI := true

		for _, part := range parts[1:] {
			part = strings.TrimSpace(part)
			if part == "" {
				continue
			}

			lower := strings.ToLower(part)
			switch lower {
			case "no-uri":
				currentIncludeURI = false
				currentFlags["no-uri"] = "true"
			case "uri":
				currentIncludeURI = true
				currentFlags["uri"] = "true"
			default:
				if strings.Contains(part, "=") {
					k, v, _ := strings.Cut(part, "=")
					currentFlags[strings.TrimSpace(k)] = strings.TrimSpace(v)
				} else {
					entries = append(entries, parsedEntry{
						Value:      currentValue,
						Flags:      currentFlags,
						IncludeURI: currentIncludeURI,
					})
					currentValue = part
					currentFlags = make(map[string]string)
					currentIncludeURI = true
				}
			}
		}

		entries = append(entries, parsedEntry{
			Value:      currentValue,
			Flags:      currentFlags,
			IncludeURI: currentIncludeURI,
		})
	}

	return entries
}

// redirEntry holds a redirection domain and whether to preserve the request URI.
type redirEntry struct {
	Domain     string
	IncludeURI bool
}

// parseRedirs parses a whitespace-separated list of redirection domains and flags.
func parseRedirs(raw string) []redirEntry {
	entries := parseEntries(raw)
	redirs := make([]redirEntry, 0, len(entries))
	for _, e := range entries {
		redirs = append(redirs, redirEntry{
			Domain:     e.Value,
			IncludeURI: e.IncludeURI,
		})
	}
	return redirs
}

// resolveService resolves the service name for an instance, checking target prefix service,
// then falling back to user.incus-compose.service and user.label.incus-compose.service.
func resolveService(inst *iutil.Instance, prefix string) string {
	service, _ := inst.ConfigValue(prefix + "service")
	service = strings.TrimSpace(service)
	if service != "" {
		return service
	}

	service, _ = inst.ConfigValue("user.incus-compose.service")
	service = strings.TrimSpace(service)
	if service != "" {
		return service
	}

	service, _ = inst.ConfigValue("user.label.incus-compose.service")
	return strings.TrimSpace(service)
}

// extractVhosts extracts and groups vhost routes for a target label from instance events.
func extractVhosts(targetLabel string, instances []*iutil.Event) []vhost {
	prefix := "user.label." + targetLabel + "."
	vhostMap := make(map[string]*vhost)
	order := make([]string, 0)

	type serviceConfig struct {
		domain       string
		upstreamPort string
		network      string
		redirect     string
		tmpl         string
		redirs       string
	}
	serviceConfigs := make(map[string]serviceConfig)

	for _, ev := range instances {
		inst := ev.Instance()
		if inst == nil || !inst.Running() {
			continue
		}

		service := resolveService(inst, prefix)
		if service == "" {
			continue
		}

		domain, _ := inst.ConfigValue(prefix + "domain")
		domain = strings.TrimSpace(domain)

		upstreamPort, _ := inst.ConfigValue(prefix + "upstream")
		upstreamPort = strings.TrimSpace(upstreamPort)

		network, _ := inst.ConfigValue(prefix + "network")
		network = strings.TrimSpace(network)

		redirect, _ := inst.ConfigValue(prefix + "redirect")
		redirect = strings.TrimSpace(redirect)

		tmpl, _ := inst.ConfigValue(prefix + "template")
		tmpl = strings.TrimSpace(tmpl)

		redirs, _ := inst.ConfigValue(prefix + "redirs")
		redirs = strings.TrimSpace(redirs)

		cfg := serviceConfigs[service]
		if cfg.domain == "" && domain != "" {
			cfg.domain = domain
		}
		if cfg.upstreamPort == "" && upstreamPort != "" {
			cfg.upstreamPort = upstreamPort
		}
		if cfg.network == "" && network != "" {
			cfg.network = network
		}
		if cfg.redirect == "" && redirect != "" {
			cfg.redirect = redirect
		}
		if cfg.tmpl == "" && tmpl != "" {
			cfg.tmpl = tmpl
		}
		if cfg.redirs == "" && redirs != "" {
			cfg.redirs = redirs
		}
		serviceConfigs[service] = cfg
	}

	for _, ev := range instances {
		inst := ev.Instance()
		if inst == nil || !inst.Running() {
			continue
		}

		service := resolveService(inst, prefix)

		domain, _ := inst.ConfigValue(prefix + "domain")
		domain = strings.TrimSpace(domain)

		redirs, _ := inst.ConfigValue(prefix + "redirs")
		redirs = strings.TrimSpace(redirs)

		upstreamPort, _ := inst.ConfigValue(prefix + "upstream")
		upstreamPort = strings.TrimSpace(upstreamPort)

		network, _ := inst.ConfigValue(prefix + "network")
		network = strings.TrimSpace(network)

		redirect, _ := inst.ConfigValue(prefix + "redirect")
		redirect = strings.TrimSpace(redirect)

		tmpl, _ := inst.ConfigValue(prefix + "template")
		tmpl = strings.TrimSpace(tmpl)

		if service != "" {
			cfg := serviceConfigs[service]
			if domain == "" {
				domain = cfg.domain
			}
			if redirs == "" {
				redirs = cfg.redirs
			}
			if upstreamPort == "" {
				upstreamPort = cfg.upstreamPort
			}
			if network == "" {
				network = cfg.network
			}
			if redirect == "" {
				redirect = cfg.redirect
			}
			if tmpl == "" {
				tmpl = cfg.tmpl
			}
		}

		if domain == "" && redirs == "" {
			continue
		}

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

		var cleanDomain string
		domainFlags := make(map[string]string)
		domainIncludeURI := true

		if domain != "" {
			domainEntries := parseEntries(domain)
			var domainNames []string
			for _, de := range domainEntries {
				if de.Value != "" {
					domainNames = append(domainNames, de.Value)
				}
				if !de.IncludeURI {
					domainIncludeURI = false
				}
				for k, v := range de.Flags {
					domainFlags[k] = v
				}
			}
			cleanDomain = strings.Join(domainNames, " ")
		}

		var redirectURL string
		if redirect != "" {
			redirEntries := parseEntries(redirect)
			if len(redirEntries) > 0 {
				re := redirEntries[0]
				redirectURL = re.Value
				includeURI := re.IncludeURI && domainIncludeURI

				if includeURI {
					if !strings.HasSuffix(redirectURL, "{uri}") {
						redirectURL += "{uri}"
					}
				} else {
					redirectURL = strings.TrimSuffix(redirectURL, "{uri}")
				}

				for k, v := range re.Flags {
					domainFlags[k] = v
				}
			}
		}

		if cleanDomain != "" {
			existing, found := vhostMap[cleanDomain]
			if !found {
				v := &vhost{
					Domain:   cleanDomain,
					Service:  service,
					Redirect: redirectURL,
					Template: tmpl,
					Flags:    domainFlags,
				}
				if upstream != "" {
					v.Upstreams = []string{upstream}
				}
				vhostMap[cleanDomain] = v
				order = append(order, cleanDomain)
			} else {
				if upstream != "" {
					if !slices.Contains(existing.Upstreams, upstream) {
						existing.Upstreams = append(existing.Upstreams, upstream)
						slices.Sort(existing.Upstreams)
					}
					if redirectURL == "" {
						existing.Redirect = ""
					}
				}
				if existing.Redirect == "" && redirectURL != "" {
					existing.Redirect = redirectURL
				}
				if existing.Template == "" && tmpl != "" {
					existing.Template = tmpl
				}
				if existing.Service == "" && service != "" {
					existing.Service = service
				}
				if existing.Flags == nil {
					existing.Flags = make(map[string]string)
				}
				for k, v := range domainFlags {
					if _, ok := existing.Flags[k]; !ok {
						existing.Flags[k] = v
					}
				}
			}
		}

		if redirs != "" {
			var baseTarget string
			if redirectURL != "" {
				baseTarget = redirectURL
			} else if cleanDomain != "" {
				primaryDomain := strings.Fields(cleanDomain)[0]
				if strings.HasPrefix(primaryDomain, "http://") || strings.HasPrefix(primaryDomain, "https://") {
					baseTarget = primaryDomain
				} else {
					baseTarget = "https://" + primaryDomain
				}
			}

			if baseTarget != "" {
				domainFields := strings.Fields(cleanDomain)
				entries := parseEntries(redirs)

				for _, entry := range entries {
					if entry.Value == "" || slices.Contains(domainFields, entry.Value) {
						continue
					}

					targetURL := baseTarget
					if entry.IncludeURI {
						if !strings.HasSuffix(targetURL, "{uri}") {
							targetURL += "{uri}"
						}
					} else {
						targetURL = strings.TrimSuffix(targetURL, "{uri}")
					}

					existingRedir, found := vhostMap[entry.Value]
					if !found {
						vhostMap[entry.Value] = &vhost{
							Domain:   entry.Value,
							Service:  service,
							Redirect: targetURL,
							Flags:    entry.Flags,
						}
						order = append(order, entry.Value)
					} else {
						if len(existingRedir.Upstreams) == 0 && existingRedir.Redirect == "" {
							existingRedir.Redirect = targetURL
						}
						if existingRedir.Service == "" && service != "" {
							existingRedir.Service = service
						}
						if existingRedir.Flags == nil {
							existingRedir.Flags = make(map[string]string)
						}
						for k, v := range entry.Flags {
							if _, ok := existingRedir.Flags[k]; !ok {
								existingRedir.Flags[k] = v
							}
						}
					}
				}
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
