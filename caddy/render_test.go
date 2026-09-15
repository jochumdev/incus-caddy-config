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
			Template: "static_proxy",
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
			Template: "broken",
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

func TestRendererTemplateExtensions(t *testing.T) {
	tmpDir := t.TempDir()

	// 1. .caddyfile extension
	err := os.WriteFile(filepath.Join(tmpDir, "proxy.caddyfile"), []byte(`{{ .Domain }} { caddyfile_ext }`), 0600)
	require.NoError(t, err)

	// 2. Exact match without extension
	err = os.WriteFile(filepath.Join(tmpDir, "exact_match"), []byte(`{{ .Domain }} { exact_match }`), 0600)
	require.NoError(t, err)

	vhosts := []vhost{
		{
			Domain:   "ext.example.com",
			Template: "proxy",
		},
		{
			Domain:   "exact.example.com",
			Template: "exact_match",
		},
	}

	content, err := render(vhosts, tmpDir, "")
	require.NoError(t, err)

	out := string(content)
	require.Contains(t, out, "ext.example.com { caddyfile_ext }")
	require.Contains(t, out, "exact.example.com { exact_match }")
}

func TestRendererCustomGlobalInlineTemplate(t *testing.T) {
	globalTmpl := `{
	admin localhost:2019
	email admin@example.com
}`
	vhosts := []vhost{
		{
			Domain:    "app.example.com",
			Upstreams: []string{"10.0.1.5:8080"},
		},
	}

	content, err := render(vhosts, "", globalTmpl)
	require.NoError(t, err)

	out := string(content)
	require.Contains(t, out, "email admin@example.com")
	require.Contains(t, out, "admin localhost:2019")
	require.Contains(t, out, "app.example.com {")
}

func TestRendererCustomGlobalTemplateFile(t *testing.T) {
	tmpDir := t.TempDir()
	globalFile := filepath.Join(tmpDir, "my_global.caddyfile")
	err := os.WriteFile(globalFile, []byte(`{
	admin localhost:2019
	auto_https off
}`), 0600)
	require.NoError(t, err)

	vhosts := []vhost{
		{
			Domain:    "app.example.com",
			Upstreams: []string{"10.0.1.5:8080"},
		},
	}

	content, err := render(vhosts, "", globalFile)
	require.NoError(t, err)

	out := string(content)
	require.Contains(t, out, "auto_https off")
	require.Contains(t, out, "app.example.com {")
}

func TestRendererCustomGlobalTemplateInCustomTemplatesDir(t *testing.T) {
	tmpDir := t.TempDir()
	globalFile := filepath.Join(tmpDir, "global.caddyfile")
	err := os.WriteFile(globalFile, []byte(`{
	admin localhost:2019
	servers {
		trusted_proxies static 10.0.0.0/8
	}
}`), 0600)
	require.NoError(t, err)

	vhosts := []vhost{
		{
			Domain:    "app.example.com",
			Upstreams: []string{"10.0.1.5:8080"},
		},
	}

	// globalTemplate is empty, should auto-discover global.caddyfile in customTemplatesDir
	content, err := render(vhosts, tmpDir, "")
	require.NoError(t, err)

	out := string(content)
	require.Contains(t, out, "trusted_proxies static 10.0.0.0/8")
	require.Contains(t, out, "app.example.com {")
}

func TestRendererCustomGlobalTemplateInCustomTemplatesDirTmpl(t *testing.T) {
	tmpDir := t.TempDir()
	globalFile := filepath.Join(tmpDir, "global.tmpl")
	err := os.WriteFile(globalFile, []byte(`{
	admin localhost:2019
	log {
		level DEBUG
	}
}`), 0600)
	require.NoError(t, err)

	vhosts := []vhost{
		{
			Domain:    "app.example.com",
			Upstreams: []string{"10.0.1.5:8080"},
		},
	}

	content, err := render(vhosts, tmpDir, "")
	require.NoError(t, err)

	out := string(content)
	require.Contains(t, out, "level DEBUG")
	require.Contains(t, out, "app.example.com {")
}

func TestRendererGlobalTemplateWithVhostsContext(t *testing.T) {
	globalTmpl := `{
	admin localhost:2019
	# Total vhosts: {{ len .Vhosts }}
}`
	vhosts := []vhost{
		{Domain: "a.example.com", Upstreams: []string{"10.0.1.1:80"}},
		{Domain: "b.example.com", Upstreams: []string{"10.0.1.2:80"}},
	}

	content, err := render(vhosts, "", globalTmpl)
	require.NoError(t, err)

	out := string(content)
	require.Contains(t, out, "# Total vhosts: 2")
}

func TestRendererInvalidGlobalTemplate(t *testing.T) {
	vhosts := []vhost{{Domain: "app.example.com"}}

	_, err := render(vhosts, "", "{{ unclosed")
	require.Error(t, err)
	require.Contains(t, err.Error(), "resolving global template")
}

func TestRendererMissingGlobalTemplateFile(t *testing.T) {
	vhosts := []vhost{{Domain: "app.example.com"}}

	_, err := render(vhosts, "", "/nonexistent/global.caddyfile")
	require.Error(t, err)
	require.Contains(t, err.Error(), "reading global template file")
}

func TestRendererInvalidGlobalTemplateInCustomTemplatesDir(t *testing.T) {
	tmpDir := t.TempDir()
	globalFile := filepath.Join(tmpDir, "global.tmpl")
	err := os.WriteFile(globalFile, []byte(`{{ unclosed`), 0600)
	require.NoError(t, err)

	vhosts := []vhost{{Domain: "app.example.com"}}

	_, err = render(vhosts, tmpDir, "")
	require.Error(t, err)
	require.Contains(t, err.Error(), "parsing global template file")
}
