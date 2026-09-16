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
		tmpl, err := loadGlobalTemplate(templatesDir, globalTemplate)
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

// loadGlobalTemplate loads a custom global template from a file or templatesDir.
func loadGlobalTemplate(templatesDir, globalTemplateFile string) (*template.Template, error) {
	var candidates []string
	if filepath.IsAbs(globalTemplateFile) {
		candidates = []string{globalTemplateFile}
		if filepath.Ext(globalTemplateFile) == "" {
			candidates = append(candidates,
				globalTemplateFile+".tmpl",
				globalTemplateFile+".caddyfile",
			)
		}
	} else {
		if templatesDir != "" {
			candidates = append(candidates, filepath.Join(templatesDir, globalTemplateFile))
			if filepath.Ext(globalTemplateFile) == "" {
				candidates = append(candidates,
					filepath.Join(templatesDir, globalTemplateFile+".tmpl"),
					filepath.Join(templatesDir, globalTemplateFile+".caddyfile"),
				)
			}
		}
		candidates = append(candidates, globalTemplateFile)
		if filepath.Ext(globalTemplateFile) == "" {
			candidates = append(candidates,
				globalTemplateFile+".tmpl",
				globalTemplateFile+".caddyfile",
			)
		}
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

		tmpl, err := template.New(filepath.Base(path)).Parse(string(content))
		if err != nil {
			return nil, fmt.Errorf("parsing global template file %q: %w", path, err)
		}

		return tmpl, nil
	}

	if readErr != nil {
		return nil, fmt.Errorf("reading global template file %q: %w", globalTemplateFile, readErr)
	}

	return nil, fmt.Errorf("reading global template file %q: %w", globalTemplateFile, os.ErrNotExist)
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

			tmpl, err := template.New(filepath.Base(path)).Parse(string(content))
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

	// Else its an inline template.
	tmpl, err := template.New("inline_" + v.Domain).Parse(v.Template)
	if err != nil {
		return nil, fmt.Errorf("parsing inline template: %w", err)
	}

	return tmpl, nil
}
