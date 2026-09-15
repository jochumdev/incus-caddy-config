package caddy

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"text/template"
)

// defaultBaseTemplate defines the global Caddyfile header.
const defaultBaseTemplate = `{
	admin localhost:2019
}
`

// defaultVhostTemplate defines the default reverse proxy or redirect site block.
const defaultVhostTemplate = `{{ .Domain }} {
{{- if .Redirect }}
	redir {{ .Redirect }} permanent
{{- else if .Upstreams }}
	reverse_proxy {{ range $i, $u := .Upstreams }}{{ if $i }} {{ end }}{{ $u }}{{ end }}
{{- end }}
}
`

// render renders the complete Caddyfile for a list of vhosts.
func render(vhosts []vhost, customTemplatesDir string) ([]byte, error) {
	var buf bytes.Buffer
	buf.WriteString(defaultBaseTemplate)

	defaultTmpl, err := template.New("default_vhost").Parse(defaultVhostTemplate)
	if err != nil {
		return nil, fmt.Errorf("parsing default vhost template: %w", err)
	}

	for _, v := range vhosts {
		buf.WriteString("\n")
		tmpl, err := resolveTemplate(v, customTemplatesDir, defaultTmpl)
		if err != nil {
			return nil, fmt.Errorf("resolving template for domain %q: %w", v.Domain, err)
		}

		var vhostBuf bytes.Buffer
		err = tmpl.Execute(&vhostBuf, v)
		if err != nil {
			return nil, fmt.Errorf("rendering template for domain %q: %w", v.Domain, err)
		}

		rendered := strings.TrimSpace(vhostBuf.String())
		if rendered != "" {
			buf.WriteString(rendered)
			buf.WriteString("\n")
		}
	}

	return buf.Bytes(), nil
}

// resolveTemplate returns the template to use for a vhost.
func resolveTemplate(v vhost, customTemplatesDir string, defaultTmpl *template.Template) (*template.Template, error) {
	if v.Template == "" {
		return defaultTmpl, nil
	}

	// 1. Check if it matches a template file in customTemplatesDir.
	if customTemplatesDir != "" {
		candidates := []string{
			filepath.Join(customTemplatesDir, v.Template),
			filepath.Join(customTemplatesDir, v.Template+".tmpl"),
			filepath.Join(customTemplatesDir, v.Template+".caddyfile"),
		}

		for _, path := range candidates {
			content, err := os.ReadFile(path)
			if err == nil {
				return template.New(filepath.Base(path)).Parse(string(content))
			}
		}
	}

	// 2. Treat as an inline Go template string.
	tmpl, err := template.New("inline_" + v.Domain).Parse(v.Template)
	if err != nil {
		return nil, fmt.Errorf("parsing inline template: %w", err)
	}

	return tmpl, nil
}
