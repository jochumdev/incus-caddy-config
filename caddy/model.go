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
	Path     string
	Flags    map[string]string
}

// IsOS reports whether the target points to a local filesystem Caddyfile rather than an Incus container.
func (t Target) IsOS() bool {
	return t.Path != ""
}

// GlobalTemplate returns the custom global template file path from target flags, if configured.
func (t Target) GlobalTemplate() string {
	if tmpl, ok := t.Flags["global_template"]; ok {
		return tmpl
	}

	return t.Flags["global-template"]
}

// String returns the string representation of Target,
// including comma-separated flags if present.
func (t Target) String() string {
	var base string
	if t.IsOS() {
		if t.Label == "caddy" || t.Label == "" {
			base = t.Path
		} else {
			base = fmt.Sprintf("%s:%s", t.Label, t.Path)
		}
	} else {
		base = fmt.Sprintf("%s:%s:%s", t.Label, t.Project, t.Instance)
	}

	if len(t.Flags) == 0 {
		return base
	}

	keys := make([]string, 0, len(t.Flags))
	for k := range t.Flags {
		keys = append(keys, k)
	}

	slices.Sort(keys)

	parts := []string{base}
	for _, k := range keys {
		v := t.Flags[k]
		if (k == "uri" || k == "no-uri") && v == "true" {
			parts = append(parts, k)
		} else {
			parts = append(parts, fmt.Sprintf("%s=%s", k, v))
		}
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

// ParseTarget parses a single "label:project:instance" specification with optional flags.
func ParseTarget(s string) (Target, error) {
	targets, err := ParseTargets(s)
	if err != nil {
		return Target{}, err
	}

	if len(targets) != 1 {
		return Target{}, fmt.Errorf("invalid target %q: expected single target", s)
	}

	if targets[0].IsOS() {
		return Target{}, fmt.Errorf("invalid target %q: expected 'label:project:instance'", s)
	}

	return targets[0], nil
}

// ParseOSTargets parses whitespace- or comma-separated OS targets with optional comma-separated flags.
func ParseOSTargets(s string) ([]Target, error) {
	targets, err := ParseTargets(s)
	if err != nil {
		return nil, err
	}

	for _, t := range targets {
		if !t.IsOS() {
			return nil, fmt.Errorf("invalid OS target: expected '[label:]path'")
		}
	}

	return targets, nil
}

// ParseOSTarget parses a single "[label:]path" specification, defaulting to label "caddy" if omitted.
func ParseOSTarget(s string) (Target, error) {
	targets, err := ParseTargets(s)
	if err != nil {
		return Target{}, err
	}

	if len(targets) != 1 {
		return Target{}, fmt.Errorf("invalid OS target %q: expected single target", s)
	}

	if !targets[0].IsOS() {
		return Target{}, fmt.Errorf("invalid OS target %q: expected '[label:]path'", s)
	}

	return targets[0], nil
}

func parseTargetEntry(e parsedEntry) (Target, error) {
	val := e.Value
	if val == "" {
		return Target{}, fmt.Errorf("empty target")
	}

	var flags map[string]string
	if len(e.Flags) > 0 {
		flags = e.Flags
	}

	// 1. Bare OS path (starts with / or .)
	idx := strings.Index(val, ":")
	if idx == -1 {
		if strings.HasPrefix(val, "/") || strings.HasPrefix(val, ".") {
			return Target{Label: "caddy", Path: val, Flags: flags}, nil
		}

		return Target{}, fmt.Errorf("invalid target %q: expected 'label:project:instance' or '[label:]path'", val)
	}

	// 2. Bare Windows drive letter (e.g. C:\... or C:/...)
	if isWindowsDrive(val) {
		return Target{Label: "caddy", Path: val, Flags: flags}, nil
	}

	// 3. Windows drive letter with explicit label prefix (e.g. edge:C:\...)
	rem := val[idx+1:]
	if isWindowsDrive(rem) {
		label := strings.TrimSpace(val[:idx])
		if label == "" {
			label = "caddy"
		}

		return Target{Label: label, Path: rem, Flags: flags}, nil
	}

	// 4. Split by colons: 2 parts = OS target [label:]path, 3 parts = Incus target label:project:instance
	parts := strings.Split(val, ":")
	if len(parts) == 2 {
		label := strings.TrimSpace(parts[0])
		path := strings.TrimSpace(parts[1])
		if label == "" {
			label = "caddy"
		}

		if path == "" {
			return Target{}, fmt.Errorf("invalid OS target %q: empty path", val)
		}

		if !strings.ContainsAny(path, "/\\") && !strings.HasPrefix(path, ".") {
			return Target{}, fmt.Errorf("invalid target %q: expected 'label:project:instance' or '[label:]path'", val)
		}

		return Target{Label: label, Path: path, Flags: flags}, nil
	}

	if len(parts) == 3 {
		if parts[0] == "" || parts[1] == "" || parts[2] == "" {
			return Target{}, fmt.Errorf("invalid target %q: expected 'label:project:instance'", val)
		}

		return Target{
			Label:    parts[0],
			Project:  parts[1],
			Instance: parts[2],
			Flags:    flags,
		}, nil
	}

	return Target{}, fmt.Errorf("invalid target %q: expected 'label:project:instance' or '[label:]path'", val)
}

func isWindowsDrive(s string) bool {
	return len(s) > 2 && s[1] == ':' && (s[2] == '\\' || s[2] == '/') &&
		((s[0] >= 'a' && s[0] <= 'z') || (s[0] >= 'A' && s[0] <= 'Z'))
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
	raw = strings.ReplaceAll(raw, " : ", ":")
	raw = strings.ReplaceAll(raw, ": ", ":")
	raw = strings.ReplaceAll(raw, " :", ":")
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
