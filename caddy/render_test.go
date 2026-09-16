package caddy

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
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

	content, err := render(vhosts, "", "")
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

	content, err := render(vhosts, "", "")
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

	content, err := render(vhosts, tmpDir, "")
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

	_, err := render(vhosts, "", "")
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

	_, err = render(vhosts, tmpDir, "")
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

	_, err := render(vhosts, "", "")
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

	content, err := render(vhosts, "", "")
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

	content, err := render(vhosts, tmpDir, "")
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
	_, err = render(vhostsMissing, tmpDir, "")
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
	content, err = render(vhostsAbs, "", "")
	require.NoError(t, err)
	require.Contains(t, string(content), "abs.example.com { abs_tmpl }")
}

func TestRendererGlobalTemplateWithVhostsContext(t *testing.T) {
	tmpDir := t.TempDir()
	globalFile := filepath.Join(tmpDir, "vhosts_global.caddyfile")
	err := os.WriteFile(globalFile, []byte(`{
	admin localhost:2019
	# Total vhosts: {{ len .Vhosts }}
}`), 0600)
	require.NoError(t, err)

	vhosts := []vhost{
		{Domain: "a.example.com", Upstreams: []string{"10.0.1.1:80"}},
		{Domain: "b.example.com", Upstreams: []string{"10.0.1.2:80"}},
	}

	content, err := render(vhosts, "", globalFile)
	require.NoError(t, err)

	out := string(content)
	require.Contains(t, out, "# Total vhosts: 2")
}

func TestRendererInvalidGlobalTemplate(t *testing.T) {
	tmpDir := t.TempDir()
	globalFile := filepath.Join(tmpDir, "invalid_global.caddyfile")
	err := os.WriteFile(globalFile, []byte("{{ unclosed"), 0600)
	require.NoError(t, err)

	vhosts := []vhost{{Domain: "app.example.com"}}

	_, err = render(vhosts, "", globalFile)
	require.Error(t, err)
	require.Contains(t, err.Error(), "parsing global template file")
}

func TestRendererMissingGlobalTemplateFile(t *testing.T) {
	vhosts := []vhost{{Domain: "app.example.com"}}

	_, err := render(vhosts, "", "/nonexistent/global.caddyfile")
	require.Error(t, err)
	require.Contains(t, err.Error(), "reading global template file")
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

	content, err := render(vhosts, "", "")
	require.NoError(t, err)

	out := string(content)
	require.Contains(t, out, "secure.example.com {")
	require.Contains(t, out, "tls internal")
	require.Contains(t, out, `header Strict-Transport-Security "max-age=31536000"`)
	require.Contains(t, out, "reverse_proxy 10.0.1.10:8443")
}

func TestRendererGlobalTemplateInTemplatesDir(t *testing.T) {
	tmpDir := t.TempDir()

	err := os.WriteFile(filepath.Join(tmpDir, "external.caddyfile"), []byte("{\n\tadmin localhost:2019\n\t# external-global\n}"), 0600)
	require.NoError(t, err)

	err = os.WriteFile(filepath.Join(tmpDir, "internal.tmpl"), []byte("{\n\tadmin localhost:2019\n\t# internal-global\n}"), 0600)
	require.NoError(t, err)

	vhosts := []vhost{{Domain: "app.example.com", Upstreams: []string{"10.0.1.1:80"}}}

	// Exact file name
	content, err := render(vhosts, tmpDir, "external.caddyfile")
	require.NoError(t, err)
	require.Contains(t, string(content), "# external-global")

	// Resolving .caddyfile extension
	content, err = render(vhosts, tmpDir, "external")
	require.NoError(t, err)
	require.Contains(t, string(content), "# external-global")

	// Resolving .tmpl extension
	content, err = render(vhosts, tmpDir, "internal")
	require.NoError(t, err)
	require.Contains(t, string(content), "# internal-global")
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

	content, err := render(vhosts, tmpDir, "")
	require.NoError(t, err)

	out := string(content)
	require.Contains(t, out, "old.example.com {")
	require.Contains(t, out, "redir https://example.com{uri} 301")
}
