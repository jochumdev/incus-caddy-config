package templated

import (
	"fmt"
	"strings"
)

// Target is one rendered file: which instances feed it, where it lands, and how it is checked and applied.
type Target struct {
	Label    string            `yaml:"label"`
	Preset   string            `yaml:"preset"`
	Instance string            `yaml:"instance"`
	Project  string            `yaml:"project"`
	Path     string            `yaml:"path"`
	Template string            `yaml:"template"`
	Text     string            `yaml:"-"`
	Format   []string          `yaml:"format"`
	Validate []string          `yaml:"validate"`
	Reload   []string          `yaml:"reload"`
	Flags    map[string]string `yaml:"flags"`
}

// String names the target for logs.
func (t Target) String() string {
	if t.Instance != "" {
		return fmt.Sprintf("%s -> %s/%s:%s", t.Label, t.Project, t.Instance, t.Path)
	}

	return fmt.Sprintf("%s -> %s", t.Label, t.Path)
}

// Key identifies the file a target writes; two targets on one file would overwrite each other.
func (t Target) Key() string {
	if t.Instance != "" {
		return t.Project + "/" + t.Instance + ":" + t.Path
	}

	return "os:" + t.Path
}

// ParseTargets parses whitespace- or comma-separated targets with optional comma-separated flags.
func ParseTargets(s string) ([]Target, error) {
	entries := ParseEntries(s)
	if len(entries) == 0 {
		return nil, fmt.Errorf("empty target")
	}

	targets := make([]Target, 0, len(entries))
	for _, e := range entries {
		if strings.Contains(e.Value, ":") {
			return nil, fmt.Errorf("invalid target %q: colon syntax is not supported, use flags (key=value)", e.Raw)
		}

		t := Target{Label: e.Value, Flags: map[string]string{}}
		for k, v := range e.Flags {
			switch k {
			case "label":
				t.Label = v
			case "preset":
				t.Preset = v
			case "instance":
				t.Instance = v
			case "project":
				t.Project = v
			case "path":
				t.Path = v
			case "template":
				t.Template = v
			case "format":
				t.Format = strings.Fields(v)
			case "validate":
				t.Validate = strings.Fields(v)
			case "reload":
				t.Reload = strings.Fields(v)
			default:
				t.Flags[k] = v
			}
		}

		targets = append(targets, t)
	}

	return targets, nil
}
