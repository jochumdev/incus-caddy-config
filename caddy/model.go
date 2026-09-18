package caddy

import (
	"fmt"
	"io"
	"log/slog"
	"net/netip"
	"slices"
	"strconv"
	"strings"
	"unicode"

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

func trimQuotes(s string) string {
	s = strings.TrimSpace(s)
	if len(s) < 2 {
		return s
	}

	quote := s[0]
	isQuote := quote == '"' || quote == '\''
	if !isQuote || s[len(s)-1] != quote {
		return s
	}

	for i := 1; i < len(s)-1; i++ {
		if s[i] == quote && s[i-1] != '\\' {
			return s
		}
	}

	return s[1 : len(s)-1]
}

func splitTokensQuoteAware(raw string) []string {
	var tokens []string
	var cur strings.Builder
	var inQuote rune

	runes := []rune(raw)
	n := len(runes)

	isAdjacentToPunct := func(idx int, forward bool) bool {
		step := 1
		if !forward {
			step = -1
		}
		for i := idx + step; i >= 0 && i < n; i += step {
			r := runes[i]
			if unicode.IsSpace(r) {
				continue
			}

			return r == ',' || r == ':'
		}

		return false
	}

	for i := 0; i < n; i++ {
		r := runes[i]

		if inQuote == 0 {
			quoteChar := r == '\'' || r == '"'
			if quoteChar {
				inQuote = r
				cur.WriteRune(r)

				continue
			}

			if r == ';' {
				if cur.Len() > 0 {
					tokens = append(tokens, cur.String())
					cur.Reset()
				}

				continue
			}

			if unicode.IsSpace(r) {
				adjacent := isAdjacentToPunct(i, false) || isAdjacentToPunct(i, true)
				if adjacent {
					continue
				}

				if cur.Len() > 0 {
					tokens = append(tokens, cur.String())
					cur.Reset()
				}

				continue
			}

			cur.WriteRune(r)
		} else {
			if r == inQuote {
				inQuote = 0
				cur.WriteRune(r)

				curStr := cur.String()
				isFullyQuoted := len(curStr) > 0 && rune(curStr[0]) == r
				if isFullyQuoted {
					k := i + 1
					for k < n && unicode.IsSpace(runes[k]) {
						k++
					}

					hasSemi := k < n && runes[k] == ';'
					if hasSemi {
						tokens = append(tokens, curStr)
						cur.Reset()
						k++
						for k < n && unicode.IsSpace(runes[k]) {
							k++
						}
						i = k - 1
					}

					hasComma := k < n && runes[k] == ','
					if hasComma {
						next := k + 1
						for next < n && unicode.IsSpace(runes[next]) {
							next++
						}

						nextIsQuote := next < n && (runes[next] == '"' || runes[next] == '\'')
						if nextIsQuote {
							tokens = append(tokens, curStr)
							cur.Reset()
							i = next - 1
						}
					}
				}

				continue
			}
			cur.WriteRune(r)
		}
	}

	if cur.Len() > 0 {
		tokens = append(tokens, cur.String())
	}

	return tokens
}

func splitPartsQuoteAware(token string) []string {
	var parts []string
	var cur strings.Builder
	var inQuote rune

	for _, r := range token {
		if inQuote == 0 {
			quoteChar := r == '\'' || r == '"'
			if quoteChar {
				inQuote = r
				cur.WriteRune(r)
			} else if r == ',' {
				p := strings.TrimSpace(cur.String())
				if p != "" {
					parts = append(parts, p)
				}
				cur.Reset()
			} else {
				cur.WriteRune(r)
			}
		} else {
			if r == inQuote {
				inQuote = 0
			}
			cur.WriteRune(r)
		}
	}

	p := strings.TrimSpace(cur.String())
	if p != "" {
		parts = append(parts, p)
	}

	return parts
}

// parseEntries parses whitespace- or comma-separated entries with optional comma-separated flags.
func parseEntries(raw string) []parsedEntry {
	var entries []parsedEntry
	tokens := splitTokensQuoteAware(raw)

	for _, token := range tokens {
		token = strings.Trim(token, ",")
		token = trimQuotes(token)
		if token == "" {
			continue
		}

		parts := splitPartsQuoteAware(token)
		var currentValue string
		var currentParts []string
		currentFlags := make(map[string]string)

		for _, part := range parts {
			part = strings.TrimSpace(part)
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
				key := trimQuotes(k)
				val := trimQuotes(v)
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
					currentParts = nil
					currentFlags = make(map[string]string)
				}
				currentFlags[key] = val
				currentParts = append(currentParts, part)
			} else if currentValue == "" && len(currentFlags) == 0 {
				currentValue = trimQuotes(part)
				currentParts = append(currentParts, part)
			} else {
				entries = append(entries, parsedEntry{
					Raw:   strings.Join(currentParts, ","),
					Value: currentValue,
					Flags: currentFlags,
				})
				currentValue = trimQuotes(part)
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

// redirEntry holds a redirection domain, whether to preserve the request URI, and an optional template.
type redirEntry struct {
	Domain     string
	IncludeURI bool
	Template   string
}

// parseRedirs parses a whitespace-separated list of redirection domains and flags.
func parseRedirs(raw string) []redirEntry {
	entries := parseEntries(raw)
	redirs := make([]redirEntry, 0, len(entries))
	for _, e := range entries {
		tmpl := e.Flags["template"]

		redirs = append(redirs, redirEntry{
			Domain:     e.Value,
			IncludeURI: e.Flags["uri"] != "false",
			Template:   tmpl,
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

type structuredRoute struct {
	index    int
	domain   string
	upstream string
	template string
	network  string
	redir    string
	flags    map[string]string
	redirs   map[int]string
	rawRedir []string
}

func parseStructuredRoutes(inst *iutil.Instance, targetLabel string) (map[int]*structuredRoute, bool) {
	labelKey := "user.label." + targetLabel
	prefix := labelKey + "."

	hasLegacy := false
	val, ok := inst.ConfigValue(labelKey)
	if ok && strings.TrimSpace(val) != "" {
		hasLegacy = true
	}

	val, ok = inst.ConfigValue(prefix + "domain")
	if ok && strings.TrimSpace(val) != "" {
		hasLegacy = true
	}

	val, ok = inst.ConfigValue(prefix + "redirs")
	if ok && strings.TrimSpace(val) != "" {
		hasLegacy = true
	}

	var routes map[int]*structuredRoute
	for k, v := range inst.Config() {
		if !strings.HasPrefix(k, prefix) {
			continue
		}

		rem := strings.TrimPrefix(k, prefix)
		idxStr, field, hasDot := strings.Cut(rem, ".")
		if !hasDot {
			continue
		}

		idx, err := strconv.Atoi(idxStr)
		if err != nil || idx < 0 {
			continue
		}

		if routes == nil {
			routes = make(map[int]*structuredRoute)
		}

		route, exists := routes[idx]
		if !exists {
			route = &structuredRoute{
				index:  idx,
				flags:  make(map[string]string),
				redirs: make(map[int]string),
			}
			routes[idx] = route
		}

		val := strings.TrimSpace(v)
		switch field {
		case "domain":
			route.domain = val
			route.flags["domain"] = val
		case "upstream":
			route.upstream = val
			route.flags["upstream"] = val
		case "template":
			route.template = val
			route.flags["template"] = val
		case "network":
			route.network = val
			route.flags["network"] = val
		case "redir":
			route.redir = val
			route.flags["redir"] = val
		case "redirs":
			route.rawRedir = append(route.rawRedir, val)
		default:
			if strings.HasPrefix(field, "redirs.") {
				mStr := strings.TrimPrefix(field, "redirs.")
				m, err := strconv.Atoi(mStr)
				if err == nil && m >= 0 {
					route.redirs[m] = val
				} else {
					route.rawRedir = append(route.rawRedir, val)
				}
			} else {
				route.flags[field] = val
			}
		}
	}

	return routes, hasLegacy
}

func addVhost(
	vhostMap map[string]*vhost,
	order *[]string,
	domain, service, redirectURL, tmpl, upstream string,
	flags map[string]string,
) {
	existing, found := vhostMap[domain]
	if !found {
		v := &vhost{
			Domain:   domain,
			Service:  service,
			Redirect: redirectURL,
			Template: tmpl,
			Flags:    flags,
		}
		if upstream != "" {
			v.Upstreams = []string{upstream}
		}
		vhostMap[domain] = v
		*order = append(*order, domain)

		return
	}

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
	for k, v := range flags {
		_, ok := existing.Flags[k]
		if !ok {
			existing.Flags[k] = v
		}
	}
}

func addRedirVhost(
	vhostMap map[string]*vhost,
	order *[]string,
	domain, service, redirectURL, tmpl string,
	flags map[string]string,
) {
	existing, found := vhostMap[domain]
	if !found {
		v := &vhost{
			Domain:   domain,
			Service:  service,
			Redirect: redirectURL,
			Template: tmpl,
			Flags:    flags,
		}
		vhostMap[domain] = v
		*order = append(*order, domain)

		return
	}

	if len(existing.Upstreams) == 0 && existing.Redirect == "" {
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
	for k, v := range flags {
		_, ok := existing.Flags[k]
		if !ok {
			existing.Flags[k] = v
		}
	}
}

func processStructuredRoute(
	inst *iutil.Instance,
	route *structuredRoute,
	service, prefix string,
	vhostMap map[string]*vhost,
	order *[]string,
) {
	domainFields := strings.Fields(route.domain)
	if len(domainFields) == 0 {
		return
	}
	cleanDomain := strings.Join(domainFields, " ")

	network := route.network
	if network == "" {
		network, _ = inst.ConfigValue(prefix + "network")
		network = strings.TrimSpace(network)
	}

	var redirectURL string
	if route.redir != "" {
		redirectURL = route.redir
		includeURI := route.flags["uri"] != "false"
		if includeURI {
			hasSuffix := strings.HasSuffix(redirectURL, "{uri}")
			if !hasSuffix {
				redirectURL += "{uri}"
			}
		} else {
			redirectURL = strings.TrimSuffix(redirectURL, "{uri}")
		}
	}

	ip := resolveIPv4(inst, network)
	var upstream string
	if ip != "" && redirectURL == "" {
		if route.upstream != "" {
			if route.upstream != "false" && route.upstream != "none" {
				if strings.Contains(route.upstream, ":") {
					upstream = route.upstream
				} else {
					upstream = fmt.Sprintf("%s:%s", ip, route.upstream)
				}
			}
		} else {
			upstream = ip
		}
	}

	addVhost(vhostMap, order, cleanDomain, service, redirectURL, route.template, upstream, route.flags)

	var redirList []string
	mIndices := make([]int, 0, len(route.redirs))
	for m := range route.redirs {
		mIndices = append(mIndices, m)
	}
	slices.Sort(mIndices)
	for _, m := range mIndices {
		redirList = append(redirList, route.redirs[m])
	}
	redirList = append(redirList, route.rawRedir...)

	if len(redirList) == 0 {
		return
	}

	var baseTarget string
	if redirectURL != "" {
		baseTarget = redirectURL
	} else {
		primaryDomain := domainFields[0]
		hasPrefix := strings.HasPrefix(primaryDomain, "http://") || strings.HasPrefix(primaryDomain, "https://")
		if hasPrefix {
			baseTarget = primaryDomain
		} else {
			baseTarget = "https://" + primaryDomain
		}
	}

	for _, rawRedir := range redirList {
		entries := parseEntries(rawRedir)
		for _, entry := range entries {
			if entry.Value == "" || slices.Contains(domainFields, entry.Value) {
				continue
			}

			targetURL := baseTarget
			if entry.Flags["uri"] != "false" {
				hasSuffix := strings.HasSuffix(targetURL, "{uri}")
				if !hasSuffix {
					targetURL += "{uri}"
				}
			} else {
				targetURL = strings.TrimSuffix(targetURL, "{uri}")
			}

			redirTmpl := entry.Flags["template"]
			addRedirVhost(vhostMap, order, entry.Value, service, targetURL, redirTmpl, entry.Flags)
		}
	}
}

// extractVhosts extracts and groups vhost routes for a target label from instance events.
func extractVhosts(logger *slog.Logger, targetLabel string, instances []*iutil.Event) []vhost {
	if logger == nil {
		logger = slog.New(slog.NewTextHandler(io.Discard, nil))
	}

	labelKey := "user.label." + targetLabel
	prefix := labelKey + "."
	vhostMap := make(map[string]*vhost)
	order := make([]string, 0)

	type serviceConfig struct {
		domain  string
		network string
		redirs  string
		routes  []*structuredRoute
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

		routes, hasLegacy := parseStructuredRoutes(inst, targetLabel)
		if hasLegacy && len(routes) > 0 {
			continue
		}

		cfg := serviceConfigs[service]
		if len(routes) > 0 {
			if len(cfg.routes) == 0 {
				indices := make([]int, 0, len(routes))
				for idx := range routes {
					indices = append(indices, idx)
				}
				slices.Sort(indices)

				var valid []*structuredRoute
				for _, idx := range indices {
					r := routes[idx]
					if r.domain != "" {
						valid = append(valid, r)
					}
				}
				if len(valid) > 0 {
					cfg.routes = valid
				}
			}
			serviceConfigs[service] = cfg

			continue
		}

		domain, _ := inst.ConfigValue(labelKey)
		if domain == "" {
			domain, _ = inst.ConfigValue(prefix + "domain")
		}
		domain = strings.TrimSpace(domain)

		network, _ := inst.ConfigValue(prefix + "network")
		network = strings.TrimSpace(network)

		if domain != "" {
			entries := parseEntries(domain)
			for _, de := range entries {
				netVal, ok := de.Flags["network"]
				if ok && netVal != "" {
					network = netVal
					break
				}
			}
		}

		redirs, _ := inst.ConfigValue(prefix + "redirs")
		redirs = strings.TrimSpace(redirs)

		if cfg.domain == "" && domain != "" {
			cfg.domain = domain
		}
		if cfg.network == "" && network != "" {
			cfg.network = network
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

		routes, hasLegacy := parseStructuredRoutes(inst, targetLabel)
		if hasLegacy && len(routes) > 0 {
			logger.Error("instance mixes legacy and structured caddy labels; skipping", "instance", ev.Name(), "project", ev.ProjectName(), "target", targetLabel)

			continue
		}

		if len(routes) > 0 {
			indices := make([]int, 0, len(routes))
			for idx := range routes {
				indices = append(indices, idx)
			}
			slices.Sort(indices)

			for _, idx := range indices {
				r := routes[idx]
				if r.domain == "" {
					logger.Error("structured route missing domain; skipping route", "instance", ev.Name(), "project", ev.ProjectName(), "target", targetLabel, "index", idx)

					continue
				}
				processStructuredRoute(inst, r, service, prefix, vhostMap, &order)
			}

			continue
		}

		cfg := serviceConfigs[service]
		if !hasLegacy && len(cfg.routes) > 0 {
			for _, r := range cfg.routes {
				processStructuredRoute(inst, r, service, prefix, vhostMap, &order)
			}

			continue
		}

		domain, _ := inst.ConfigValue(labelKey)
		if domain == "" {
			domain, _ = inst.ConfigValue(prefix + "domain")
		}
		domain = strings.TrimSpace(domain)

		redirs, _ := inst.ConfigValue(prefix + "redirs")
		redirs = strings.TrimSpace(redirs)

		network, _ := inst.ConfigValue(prefix + "network")
		network = strings.TrimSpace(network)

		if service != "" {
			if domain == "" {
				domain = cfg.domain
			}
			if redirs == "" {
				redirs = cfg.redirs
			}
			if network == "" {
				network = cfg.network
			}
		}

		if domain == "" && redirs == "" {
			continue
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

		netFlag, ok := domainFlags["network"]
		if ok && netFlag != "" {
			network = netFlag
		}

		if redirs == "" {
			redirs = domainFlags["redirs"]
		}

		var redirectURL string
		redirVal, hasRedir := domainFlags["redir"]
		if hasRedir && redirVal != "" {
			redirectURL = redirVal
			includeURI := (domainFlags["uri"] != "false") && domainIncludeURI
			if includeURI {
				hasSuffix := strings.HasSuffix(redirectURL, "{uri}")
				if !hasSuffix {
					redirectURL += "{uri}"
				}
			} else {
				redirectURL = strings.TrimSuffix(redirectURL, "{uri}")
			}
		}

		ip := resolveIPv4(inst, network)
		var upstream string
		upstreamVal, hasUpstream := domainFlags["upstream"]
		if ip != "" {
			if hasUpstream {
				if upstreamVal != "" && upstreamVal != "true" {
					if strings.Contains(upstreamVal, ":") {
						upstream = upstreamVal
					} else {
						upstream = fmt.Sprintf("%s:%s", ip, upstreamVal)
					}
				} else if upstreamVal != "false" && upstreamVal != "none" {
					upstream = ip
				}
			} else if redirectURL == "" {
				upstream = ip
			}
		}

		tmpl := domainFlags["template"]

		if cleanDomain != "" {
			addVhost(vhostMap, &order, cleanDomain, service, redirectURL, tmpl, upstream, domainFlags)
		}

		if redirs != "" {
			var baseTarget string
			if redirectURL != "" {
				baseTarget = redirectURL
			} else if cleanDomain != "" {
				primaryDomain := strings.Fields(cleanDomain)[0]
				hasPrefix := strings.HasPrefix(primaryDomain, "http://") || strings.HasPrefix(primaryDomain, "https://")
				if hasPrefix {
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
						hasSuffix := strings.HasSuffix(targetURL, "{uri}")
						if !hasSuffix {
							targetURL += "{uri}"
						}
					} else {
						targetURL = strings.TrimSuffix(targetURL, "{uri}")
					}

					redirTmpl := entry.Flags["template"]
					addRedirVhost(vhostMap, &order, entry.Value, service, targetURL, redirTmpl, entry.Flags)
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
