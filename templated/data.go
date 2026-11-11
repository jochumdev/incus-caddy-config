package templated

import (
	"maps"
	"net/netip"
	"strings"

	"github.com/lxc/incus-compose/ievent/iutil"
)

// Data is what a target's template is executed with.
type Data struct {
	Label     string
	Flags     map[string]string
	Instances []Instance
}

// Instance is one running instance carrying the target's label.
type Instance struct {
	Name     string
	Project  string
	Location string

	// Label is the value of user.label.<label>, and Labels the keys below it with that prefix cut.
	Label  string
	Labels map[string]string

	Config     map[string]string
	Devices    map[string]map[string]string
	Interfaces []Interface
}

// Interface is one NIC of an instance.
type Interface struct {
	Network string
	Project string
	IPv4    []string
	IPv6    []string
}

// IPv4 is the first non-loopback IPv4 on network, or on any network when that has none.
func (i Instance) IPv4(network string) string {
	if network != "" {
		for _, iface := range i.Interfaces {
			if iface.Network != network {
				continue
			}

			ip := firstIPv4(iface.IPv4)
			if ip != "" {
				return ip
			}
		}
	}

	for _, iface := range i.Interfaces {
		ip := firstIPv4(iface.IPv4)
		if ip != "" {
			return ip
		}
	}

	return ""
}

func firstIPv4(addrs []string) string {
	for _, ip := range addrs {
		parsed, err := netip.ParseAddr(ip)
		if err == nil && parsed.Is4() && !parsed.IsLoopback() {
			return parsed.String()
		}
	}

	return ""
}

// HasLabel reports whether config carries user.label.<label> or a key below it.
func HasLabel(config map[string]string, label string) bool {
	exact := "user.label." + label
	for k := range config {
		if k == exact || strings.HasPrefix(k, exact+".") {
			return true
		}
	}

	return false
}

func newInstance(ev *iutil.Event, label string) Instance {
	inst := ev.Instance()
	exact := "user.label." + label

	out := Instance{
		Name:     ev.Name(),
		Project:  ev.ProjectName(),
		Location: inst.Location(),
		Labels:   map[string]string{},
		Config:   maps.Collect(inst.Config()),
		Devices:  maps.Collect(inst.Devices()),
	}

	for k, v := range out.Config {
		if k == exact {
			out.Label = v

			continue
		}

		rest, ok := strings.CutPrefix(k, exact+".")
		if ok {
			out.Labels[rest] = v
		}
	}

	for iface := range inst.Interfaces() {
		out.Interfaces = append(out.Interfaces, Interface{
			Network: iface.Network(),
			Project: iface.Project(),
			IPv4:    iface.IPv4(),
			IPv6:    iface.IPv6(),
		})
	}

	return out
}
