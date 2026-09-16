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
func render(vhosts []vhost, templatesDir, globalTemplate string) ([]byte, error) {
	var buf bytes.Buffer

	defaultGlobalTmpl, err := template.New("default_global").Parse(defaultBaseTemplate)
	if err != nil {
		return nil, fmt.Errorf("parsing default global template: %w", err)
	}

	globalTmpl := defaultGlobalTmpl
	if globalTemplate != "" {
		tmpl, err := loadGlobalTemplate(globalTemplate)
		if err != nil {
			return nil, fmt.Errorf("loading global template: %w", err)
		}

		globalTmpl = tmpl
	}

	var globalBuf bytes.Buffer
	err = globalTmpl.Execute(&globalBuf, map[string]any{
		"Vhosts": vhosts,
	})
	if err != nil {
		return nil, fmt.Errorf("rendering global template: %w", err)
	}

	renderedGlobal := strings.TrimSpace(globalBuf.String())
	if renderedGlobal != "" {
		buf.WriteString(renderedGlobal)
		buf.WriteString("\n")
	}

	defaultTmpl, err := template.New("default_vhost").Parse(defaultVhostTemplate)
	if err != nil {
		return nil, fmt.Errorf("parsing default vhost template: %w", err)
	}

	for _, v := range vhosts {
		tmpl, err := resolveTemplate(v, templatesDir, defaultTmpl)
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
			if buf.Len() > 0 {
				buf.WriteString("\n")
			}

			buf.WriteString(rendered)
			buf.WriteString("\n")
		}
	}

	return buf.Bytes(), nil
}

// loadGlobalTemplate loads a custom global template from a file.
func loadGlobalTemplate(globalTemplateFile string) (*template.Template, error) {
	content, err := os.ReadFile(globalTemplateFile)
	if err != nil {
		return nil, fmt.Errorf("reading global template file %q: %w", globalTemplateFile, err)
	}

	tmpl, err := template.New(filepath.Base(globalTemplateFile)).Parse(string(content))
	if err != nil {
		return nil, fmt.Errorf("parsing global template file %q: %w", globalTemplateFile, err)
	}

	return tmpl, nil
}

// resolveTemplate returns the template to use for a vhost.
func resolveTemplate(v vhost, templatesDir string, defaultTmpl *template.Template) (*template.Template, error) {
	if v.Template == "" {
		return defaultTmpl, nil
	}

	// 1. Check if it matches a template file in templatesDir.
	if templatesDir != "" {
		candidates := []string{
			filepath.Join(templatesDir, v.Template),
			filepath.Join(templatesDir, v.Template+".tmpl"),
			filepath.Join(templatesDir, v.Template+".caddyfile"),
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
