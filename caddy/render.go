// Package caddy turns instance labels into Caddyfile site blocks, as template funcs and a preset.
package caddy

import (
	"bytes"
	_ "embed"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"text/template"

	"github.com/jochumdev/incus-templated/templated"
)

//go:embed caddy.tmpl
var presetTemplate string

// defaultVhostTemplate defines the default reverse proxy or redirect site block.
const defaultVhostTemplate = `{{ .Domain }} {
{{- if .Redirect }}
	redir {{ .Redirect }} permanent
{{- else if .Upstreams }}
	reverse_proxy {{ range $i, $u := .Upstreams }}{{ if $i }} {{ end }}{{ $u }}{{ end }}
{{- end }}
}
`

// Preset is the caddy target defaults; a field the target sets itself wins.
func Preset() templated.Target {
	return templated.Target{
		Template: "caddy",
		Text:     presetTemplate,
		Path:     "/config/Caddyfile",
		Format:   []string{"caddy", "fmt", "--overwrite", "{staging}"},
		Validate: []string{"caddy", "validate", "--config", "{staging}"},
		Reload:   []string{"caddy", "reload", "--config", "{path}", "--adapter", "caddyfile"},
	}
}

// Funcs are the template funcs vhosts, which groups a target's instances into site blocks, and vhost, which renders one.
func Funcs(logger *slog.Logger, templatesDir string) template.FuncMap {
	defaultTmpl := template.Must(template.New("default_vhost").Parse(defaultVhostTemplate))

	return template.FuncMap{
		"vhosts": func(d templated.Data) []vhost {
			return extractVhosts(logger, d.Label, d.Instances)
		},
		"vhost": func(v vhost) (string, error) {
			tmpl, err := resolveTemplate(v, templatesDir, defaultTmpl)
			if err != nil {
				return "", fmt.Errorf("resolving template for domain %q: %w", v.Domain, err)
			}

			var buf bytes.Buffer

			err = tmpl.Execute(&buf, v)
			if err != nil {
				return "", fmt.Errorf("rendering template for domain %q: %w", v.Domain, err)
			}

			return strings.TrimSpace(buf.String()), nil
		},
	}
}

// resolveTemplate returns the template to use for a vhost.
func resolveTemplate(v vhost, templatesDir string, defaultTmpl *template.Template) (*template.Template, error) {
	if v.Template == "" {
		return defaultTmpl, nil
	}

	// If the input is a valid path (dir+file+ext) use it or 404, else its an inline template.
	if !strings.ContainsAny(v.Template, "{\n") && filepath.Ext(v.Template) != "" {
		var candidates []string
		if filepath.IsAbs(v.Template) {
			candidates = []string{v.Template}
		} else {
			if templatesDir != "" {
				candidates = append(candidates, filepath.Join(templatesDir, v.Template))
			}
			candidates = append(candidates, v.Template)
		}

		var readErr error
		for _, path := range candidates {
			content, err := os.ReadFile(path)
			if err != nil {
				if readErr == nil {
					readErr = err
				}

				continue
			}

			tmpl, err := template.New(filepath.Base(path)).Option("missingkey=zero").Parse(string(content))
			if err != nil {
				return nil, fmt.Errorf("parsing template file %q: %w", path, err)
			}

			return tmpl, nil
		}

		if readErr != nil {
			return nil, fmt.Errorf("reading template file %q: %w", v.Template, readErr)
		}

		return nil, fmt.Errorf("reading template file %q: %w", v.Template, os.ErrNotExist)
	}

	tmpl, err := template.New("inline_" + v.Domain).Option("missingkey=zero").Parse(v.Template)
	if err != nil {
		return nil, fmt.Errorf("parsing inline template: %w", err)
	}

	return tmpl, nil
}
