package caddy

import (
	"fmt"
	"io"
	"log/slog"
	"slices"
	"strconv"
	"strings"

	"github.com/jochumdev/incus-templated/templated"
)

// vhost holds the model data needed to render a Caddyfile site block.
type vhost struct {
	Domain    string
	Service   string
	Upstreams []string
	Redirect  string
	Template  string
	Flags     map[string]string
}

// redirEntry holds a redirection domain, whether to preserve the request URI, and an optional template.
type redirEntry struct {
	Domain     string
	IncludeURI bool
	Template   string
}

// parseRedirs parses a whitespace-separated list of redirection domains and flags.
func parseRedirs(raw string) []redirEntry {
	entries := templated.ParseEntries(raw)
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
func resolveService(inst templated.Instance, prefix string) string {
	service := inst.Config[prefix+"service"]
	service = strings.TrimSpace(service)
	if service != "" {
		return service
	}

	service = inst.Config["user.incus-compose.service"]
	service = strings.TrimSpace(service)
	if service != "" {
		return service
	}

	service = inst.Config["user.label.incus-compose.service"]
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

func parseStructuredRoutes(inst templated.Instance, targetLabel string) (map[int]*structuredRoute, bool) {
	labelKey := "user.label." + targetLabel
	prefix := labelKey + "."

	hasLegacy := false
	val, ok := inst.Config[labelKey]
	if ok && strings.TrimSpace(val) != "" {
		hasLegacy = true
	}

	val, ok = inst.Config[prefix+"domain"]
	if ok && strings.TrimSpace(val) != "" {
		hasLegacy = true
	}

	val, ok = inst.Config[prefix+"redirs"]
	if ok && strings.TrimSpace(val) != "" {
		hasLegacy = true
	}

	var routes map[int]*structuredRoute
	for k, v := range inst.Config {
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
	inst templated.Instance,
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
		network = inst.Config[prefix+"network"]
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

	ip := inst.IPv4(network)
	var upstream string
	if redirectURL == "" {
		if route.upstream != "" {
			if route.upstream != "false" && route.upstream != "none" {
				if strings.Contains(route.upstream, ":") {
					upstream = route.upstream
				} else if ip != "" {
					upstream = fmt.Sprintf("%s:%s", ip, route.upstream)
				}
			}
		} else if ip != "" {
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
		entries := templated.ParseEntries(rawRedir)
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
func extractVhosts(logger *slog.Logger, targetLabel string, instances []templated.Instance) []vhost {
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

	for _, inst := range instances {
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

		domain := inst.Config[labelKey]
		if domain == "" {
			domain = inst.Config[prefix+"domain"]
		}
		domain = strings.TrimSpace(domain)

		network := inst.Config[prefix+"network"]
		network = strings.TrimSpace(network)

		if domain != "" {
			entries := templated.ParseEntries(domain)
			for _, de := range entries {
				netVal, ok := de.Flags["network"]
				if ok && netVal != "" {
					network = netVal
					break
				}
			}
		}

		redirs := inst.Config[prefix+"redirs"]
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

	for _, inst := range instances {
		service := resolveService(inst, prefix)

		routes, hasLegacy := parseStructuredRoutes(inst, targetLabel)
		if hasLegacy && len(routes) > 0 {
			logger.Error("instance mixes legacy and structured caddy labels; skipping", "instance", inst.Name, "project", inst.Project, "target", targetLabel)

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
					logger.Error("structured route missing domain; skipping route", "instance", inst.Name, "project", inst.Project, "target", targetLabel, "index", idx)

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

		domain := inst.Config[labelKey]
		if domain == "" {
			domain = inst.Config[prefix+"domain"]
		}
		domain = strings.TrimSpace(domain)

		redirs := inst.Config[prefix+"redirs"]
		redirs = strings.TrimSpace(redirs)

		network := inst.Config[prefix+"network"]
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
			domainEntries := templated.ParseEntries(domain)
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

		ip := inst.IPv4(network)
		var upstream string
		upstreamVal, hasUpstream := domainFlags["upstream"]
		if hasUpstream {
			if upstreamVal != "" && upstreamVal != "true" {
				if strings.Contains(upstreamVal, ":") {
					upstream = upstreamVal
				} else if ip != "" {
					upstream = fmt.Sprintf("%s:%s", ip, upstreamVal)
				}
			} else if upstreamVal != "false" && upstreamVal != "none" && ip != "" {
				upstream = ip
			}
		} else if redirectURL == "" && ip != "" {
			upstream = ip
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
				entries := templated.ParseEntries(redirs)

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
