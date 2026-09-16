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
	raw   string
	Label string
	flags map[string]string
}

// NewTarget creates a Target with the given label and flags.
func NewTarget(label string, flags map[string]string) Target {
	t := Target{
		Label: label,
		flags: flags,
	}
	t.raw = t.String()

	return t
}

// Flag returns the value of a target flag and whether it was present.
func (t Target) Flag(key string) (string, bool) {
	if t.flags == nil {
		return "", false
	}

	v, ok := t.flags[key]
	return v, ok
}

// String returns the raw input string representation of Target for logging.
func (t Target) String() string {
	if t.raw != "" {
		return t.raw
	}

	label := t.Label
	if label == "" {
		label = "caddy"
	}

	if len(t.flags) == 0 {
		return label
	}

	parts := []string{label}
	keys := make([]string, 0, len(t.flags))
	for k := range t.flags {
		keys = append(keys, k)
	}

	slices.Sort(keys)

	for _, k := range keys {
		parts = append(parts, fmt.Sprintf("%s=%s", k, t.flags[k]))
	}

	return strings.Join(parts, ",")
}

// ParseTargets parses whitespace- or comma-separated targets with optional comma-separated flags.
func ParseTargets(s string) ([]Target, error) {
	entries := parseEntries(s)
	if len(entries) == 0 {
		return nil, fmt.Errorf("empty target")
	}

	targets := make([]Target, 0, len(entries))
	for _, e := range entries {
		t, err := parseTargetEntry(e)
		if err != nil {
			return nil, err
		}

		targets = append(targets, t)
	}

	return targets, nil
}

// ParseTarget parses a single target specification with flags.
func ParseTarget(s string) (Target, error) {
	targets, err := ParseTargets(s)
	if err != nil {
		return Target{}, err
	}

	if len(targets) != 1 {
		return Target{}, fmt.Errorf("invalid target %q: expected single target", s)
	}

	return targets[0], nil
}

func parseTargetEntry(e parsedEntry) (Target, error) {
	if e.Value == "" && len(e.Flags) == 0 {
		return Target{}, fmt.Errorf("empty target")
	}

	flags := e.Flags
	if flags == nil {
		flags = make(map[string]string)
	}

	label := e.Value
	if l, ok := flags["label"]; ok && l != "" {
		label = l
	}

	if label == "" {
		label = "caddy"
	}

	if strings.Contains(e.Value, ":") {
		return Target{}, fmt.Errorf("invalid target %q: colon syntax is not supported, use flags (key=value)", e.Raw)
	}

	return Target{
		raw:   e.Raw,
		Label: label,
		flags: flags,
	}, nil
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
	Raw   string
	Value string
	Flags map[string]string
}

// parseEntries parses whitespace- or comma-separated entries with optional comma-separated flags.
func parseEntries(raw string) []parsedEntry {
	var entries []parsedEntry
	raw = strings.ReplaceAll(raw, " : ", ":")
	raw = strings.ReplaceAll(raw, ": ", ":")
	raw = strings.ReplaceAll(raw, " :", ":")
	raw = strings.ReplaceAll(raw, ";", " ")
	for strings.Contains(raw, " ,") || strings.Contains(raw, ", ") {
		raw = strings.ReplaceAll(raw, " ,", ",")
		raw = strings.ReplaceAll(raw, ", ", ",")
	}
	tokens := strings.Fields(raw)

	for _, token := range tokens {
		token = strings.Trim(token, "\"',")
		if token == "" {
			continue
		}

		parts := strings.Split(token, ",")
		var currentValue string
		var currentParts []string
		currentFlags := make(map[string]string)

		for _, part := range parts {
			part = strings.TrimSpace(part)
			part = strings.Trim(part, "\"'")
			if part == "" {
				continue
			}

			lower := strings.ToLower(part)
			if lower == "uri" {
				currentFlags["uri"] = "true"
				currentParts = append(currentParts, part)
			} else if lower == "no-uri" {
				currentFlags["uri"] = "false"
				currentParts = append(currentParts, part)
			} else if strings.HasPrefix(lower, "no-") && len(lower) > 3 && (currentValue != "" || len(currentFlags) > 0) {
				key := strings.TrimPrefix(lower, "no-")
				currentFlags[key] = "false"
				currentParts = append(currentParts, part)
			} else if strings.Contains(part, "=") {
				k, v, _ := strings.Cut(part, "=")
				key := strings.Trim(strings.TrimSpace(k), "\"'")
				val := strings.Trim(strings.TrimSpace(v), "\"'")
				isNewTarget := false
				if key == "instance" && currentFlags["instance"] != "" && currentFlags["project"] != "" {
					isNewTarget = true
				} else if key == "path" && currentFlags["path"] != "" && currentValue == "" {
					isNewTarget = true
				}

				if isNewTarget {
					entries = append(entries, parsedEntry{
						Raw:   strings.Join(currentParts, ","),
						Value: currentValue,
						Flags: currentFlags,
					})
					currentValue = ""
					currentParts = []string{part}
					currentFlags = make(map[string]string)
				}
				currentFlags[key] = val
				currentParts = append(currentParts, part)
			} else if currentValue == "" && len(currentFlags) == 0 {
				currentValue = part
				currentParts = append(currentParts, part)
			} else {
				entries = append(entries, parsedEntry{
					Raw:   strings.Join(currentParts, ","),
					Value: currentValue,
					Flags: currentFlags,
				})
				currentValue = part
				currentParts = []string{part}
				currentFlags = make(map[string]string)
			}
		}

		if currentValue != "" || len(currentFlags) > 0 {
			entries = append(entries, parsedEntry{
				Raw:   strings.Join(currentParts, ","),
				Value: currentValue,
				Flags: currentFlags,
			})
		}
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
			IncludeURI: e.Flags["uri"] != "false",
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
				if de.Flags["uri"] == "false" {
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
				includeURI := (re.Flags["uri"] != "false") && domainIncludeURI

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
					if entry.Flags["uri"] != "false" {
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
