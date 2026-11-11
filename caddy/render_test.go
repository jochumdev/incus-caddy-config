package caddy

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"
	"text/template"

	"github.com/stretchr/testify/require"

	"github.com/jochumdev/incus-templated/templated"
)

func TestRendererDefaultVhost(t *testing.T) {
	vhosts := []vhost{
		{
			Domain:    "git.example.com",
			Upstreams: []string{"10.0.1.5:3000", "10.0.1.6:3000"},
		},
		{
			Domain:   "old.example.com",
			Redirect: "https://new.example.com{uri}",
		},
	}

	content, err := render(vhosts, "")
	require.NoError(t, err)

	out := string(content)
	require.Contains(t, out, "admin localhost:2019")
	require.Contains(t, out, "git.example.com {")
	require.Contains(t, out, "reverse_proxy 10.0.1.5:3000 10.0.1.6:3000")
	require.Contains(t, out, "old.example.com {")
	require.Contains(t, out, "redir https://new.example.com{uri} permanent")
}

func TestRendererCustomInlineTemplate(t *testing.T) {
	customTmpl := `{{ .Domain }} {
	encode gzip zstd
	reverse_proxy {{ index .Upstreams 0 }}
}`

	vhosts := []vhost{
		{
			Domain:    "spa.example.com",
			Upstreams: []string{"10.0.1.10:8080"},
			Template:  customTmpl,
		},
	}

	content, err := render(vhosts, "")
	require.NoError(t, err)

	out := string(content)
	require.Contains(t, out, "spa.example.com {")
	require.Contains(t, out, "encode gzip zstd")
	require.Contains(t, out, "reverse_proxy 10.0.1.10:8080")
}

func TestRendererCustomTemplateFile(t *testing.T) {
	tmpDir := t.TempDir()
	templateFile := filepath.Join(tmpDir, "static_proxy.tmpl")
	err := os.WriteFile(templateFile, []byte(`{{ .Domain }} {
	root * /var/www/html
	file_server
}`), 0600)
	require.NoError(t, err)

	vhosts := []vhost{
		{
			Domain:   "static.example.com",
			Template: "static_proxy.tmpl",
		},
	}

	content, err := render(vhosts, tmpDir)
	require.NoError(t, err)

	out := string(content)
	require.Contains(t, out, "static.example.com {")
	require.Contains(t, out, "file_server")
}

func TestRendererInvalidInlineTemplate(t *testing.T) {
	vhosts := []vhost{
		{
			Domain:   "bad.example.com",
			Template: "{{ .Domain { unclosed",
		},
	}

	_, err := render(vhosts, "")
	require.Error(t, err)
	require.Contains(t, err.Error(), "bad.example.com")
}

func TestRendererInvalidTemplateFile(t *testing.T) {
	tmpDir := t.TempDir()
	templateFile := filepath.Join(tmpDir, "broken.tmpl")
	err := os.WriteFile(templateFile, []byte(`{{ .Domain { unclosed`), 0600)
	require.NoError(t, err)

	vhosts := []vhost{
		{
			Domain:   "broken.example.com",
			Template: "broken.tmpl",
		},
	}

	_, err = render(vhosts, tmpDir)
	require.Error(t, err)
	require.Contains(t, err.Error(), "resolving template for domain \"broken.example.com\"")
}

func TestRendererTemplateExecutionFailure(t *testing.T) {
	// Calling .Domain as a function causes a runtime template execution error.
	vhosts := []vhost{
		{
			Domain:   "fail.example.com",
			Template: `{{ .Domain }} { {{ call .Domain }} }`,
		},
	}

	_, err := render(vhosts, "")
	require.Error(t, err)
	require.Contains(t, err.Error(), "rendering template for domain \"fail.example.com\"")
}

func TestRendererWhitespaceOnlyTemplate(t *testing.T) {
	vhosts := []vhost{
		{
			Domain:   "empty.example.com",
			Template: "   \n\t  \n  ",
		},
	}

	content, err := render(vhosts, "")
	require.NoError(t, err)

	// Whitespace-only block is not appended, only defaultBaseTemplate remains.
	out := string(content)
	require.Contains(t, out, "admin localhost:2019")
	require.NotContains(t, out, "empty.example.com")
}

func TestRendererTemplateFilePathOr404(t *testing.T) {
	tmpDir := t.TempDir()

	// 1. Valid path with extension found in templatesDir
	err := os.WriteFile(filepath.Join(tmpDir, "proxy.caddyfile"), []byte(`{{ .Domain }} { caddyfile_ext }`), 0600)
	require.NoError(t, err)

	vhosts := []vhost{
		{
			Domain:   "ext.example.com",
			Template: "proxy.caddyfile",
		},
	}

	content, err := render(vhosts, tmpDir)
	require.NoError(t, err)

	out := string(content)
	require.Contains(t, out, "ext.example.com { caddyfile_ext }")

	// 2. Valid path with extension not found -> 404 (error)
	vhostsMissing := []vhost{
		{
			Domain:   "missing.example.com",
			Template: "missing.caddyfile",
		},
	}
	_, err = render(vhostsMissing, tmpDir)
	require.Error(t, err)
	require.Contains(t, err.Error(), "reading template file")

	// 3. Absolute path with extension
	absFile := filepath.Join(tmpDir, "abs.tmpl")
	err = os.WriteFile(absFile, []byte(`{{ .Domain }} { abs_tmpl }`), 0600)
	require.NoError(t, err)

	vhostsAbs := []vhost{
		{
			Domain:   "abs.example.com",
			Template: absFile,
		},
	}
	content, err = render(vhostsAbs, "")
	require.NoError(t, err)
	require.Contains(t, string(content), "abs.example.com { abs_tmpl }")
}

func TestRendererCustomTemplateWithFlags(t *testing.T) {
	customTmpl := `{{ .Domain }} {
{{- if eq .Flags.tls "internal" }}
	tls internal
{{- end }}
{{- if .Flags.hsts }}
	header Strict-Transport-Security "max-age=31536000"
{{- end }}
	reverse_proxy {{ index .Upstreams 0 }}
}`

	vhosts := []vhost{
		{
			Domain:    "secure.example.com",
			Upstreams: []string{"10.0.1.10:8443"},
			Template:  customTmpl,
			Flags: map[string]string{
				"tls":  "internal",
				"hsts": "true",
			},
		},
	}

	content, err := render(vhosts, "")
	require.NoError(t, err)

	out := string(content)
	require.Contains(t, out, "secure.example.com {")
	require.Contains(t, out, "tls internal")
	require.Contains(t, out, `header Strict-Transport-Security "max-age=31536000"`)
	require.Contains(t, out, "reverse_proxy 10.0.1.10:8443")
}

func TestRendererRedirWithCustomTemplate(t *testing.T) {
	tmpDir := t.TempDir()
	redirTmplFile := filepath.Join(tmpDir, "custom_redir.caddyfile")
	err := os.WriteFile(redirTmplFile, []byte(`{{ .Domain }} {
	redir {{ .Redirect }} 301
}`), 0600)
	require.NoError(t, err)

	vhosts := []vhost{
		{
			Domain:   "old.example.com",
			Redirect: "https://example.com{uri}",
			Template: "custom_redir.caddyfile",
		},
	}

	content, err := render(vhosts, tmpDir)
	require.NoError(t, err)

	out := string(content)
	require.Contains(t, out, "old.example.com {")
	require.Contains(t, out, "redir https://example.com{uri} 301")
}

// render runs the shipped preset over vhosts as given, skipping extraction.
func render(vhosts []vhost, templatesDir string) ([]byte, error) {
	funcs := Funcs(nil, templatesDir)
	funcs["vhosts"] = func(templated.Data) []vhost { return vhosts }

	tmpl, err := template.New("caddy").Funcs(funcs).Parse(presetTemplate)
	if err != nil {
		return nil, err
	}

	var buf bytes.Buffer

	err = tmpl.Execute(&buf, templated.Data{})

	return buf.Bytes(), err
}
