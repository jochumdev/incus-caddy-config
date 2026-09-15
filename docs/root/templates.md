---
title: Custom Vhost Templates
description: Guide to customizing Caddyfile site blocks using inline Go templates and external template files.
published: true
editor: markdown
---

# Custom Vhost Templates

`caddy-config` uses Go's `text/template` engine to render site blocks. While standard reverse proxying and redirects work out-of-the-box, you can customize the entire site block for any instance.

---

## The Default Site Block

When an instance has no `template` label, `caddy-config` applies the default site template:

```caddyfile
{{ .Domain }} {
{{- if .Redirect }}
	redir {{ .Redirect }} permanent
{{- else if .Upstreams }}
	reverse_proxy {{ range $i, $u := .Upstreams }}{{ if $i }} {{ end }}{{ $u }}{{ end }}
{{- end }}
}
```

---

## Template Context Variables

Inside your custom template, the following fields are available:

| Variable | Type | Description |
|---|---|---|
| `.Domain` | `string` | The domain(s) defined on the instance (`user.label.<prefix>.domain`). |
| `.Upstreams` | `[]string` | Sorted list of resolved upstream addresses (e.g. `["10.0.1.5:8080", "10.0.1.6:8080"]`). |
| `.Redirect` | `string` | The redirect URL if configured (`user.label.<prefix>.redirect`). |
| `.Template` | `string` | The raw template name or inline template string. |

---

## Defining Custom Templates

You can provide custom templates in two ways:

### 1. Inline Go Template

Specify the template directly in the `template` label:

```yaml
services:
  web:
    image: docker.io/library/nginx:alpine
    labels:
      edge.domain: "spa.example.com"
      edge.upstream: "8080"
      edge.template: |
        {{ .Domain }} {
        	encode gzip zstd
        	reverse_proxy {{ index .Upstreams 0 }}
        }
```

### 2. External Template Directory (`--custom-templates-dir`)

For reusable site configurations, store templates in a directory mounted to `caddy-config` and pass `--custom-templates-dir /etc/caddy/templates`.

When an instance specifies `edge.template: "spa_site"`, `caddy-config` searches the custom templates directory in the following order:
1. `/etc/caddy/templates/spa_site`
2. `/etc/caddy/templates/spa_site.tmpl`
3. `/etc/caddy/templates/spa_site.caddyfile`

---

## Production Template Examples

### Example 1: Compression and Security Headers

```caddyfile
# /etc/caddy/templates/secure_proxy.caddyfile
{{ .Domain }} {
	encode gzip zstd

	header {
		Strict-Transport-Security "max-age=31536000; includeSubDomains; preload"
		X-Content-Type-Options "nosniff"
		X-Frame-Options "DENY"
		Referrer-Policy "strict-origin-when-cross-origin"
	}

	reverse_proxy {{ range $i, $u := .Upstreams }}{{ if $i }} {{ end }}{{ $u }}{{ end }}
}
```

**Usage in Compose**:
```yaml
services:
  app:
    image: my-app:latest
    labels:
      edge.domain: "secure.example.com"
      edge.upstream: "3000"
      edge.template: "secure_proxy"
```

---

### Example 2: Static Files with Reverse Proxy Fallback

Useful for single-page applications (React, Vue) or microservices serving static assets:

```caddyfile
# /etc/caddy/templates/spa_app.caddyfile
{{ .Domain }} {
	root * /var/www/dist
	file_server

	try_files {path} /index.html

	handle /api/* {
		reverse_proxy {{ index .Upstreams 0 }}
	}
}
```

---

### Example 3: PHP-FPM FastCGI Site

Connects directly to an upstream PHP container running `php-fpm`:

```caddyfile
# /etc/caddy/templates/php_site.caddyfile
{{ .Domain }} {
	root * /var/www/html
	php_fastcgi {{ index .Upstreams 0 }}
	file_server
}
```

**Usage in Compose**:
```yaml
services:
  wordpress:
    image: docker.io/library/wordpress:fpm-alpine
    labels:
      edge.domain: "blog.example.com"
      edge.upstream: "9000"
      edge.template: "php_site"
```

---

### Example 4: WebSockets & Long-Lived Streaming

Optimized for real-time applications requiring continuous flushes and keep-alives:

```caddyfile
# /etc/caddy/templates/websocket_proxy.caddyfile
{{ .Domain }} {
	reverse_proxy {{ range $i, $u := .Upstreams }}{{ if $i }} {{ end }}{{ $u }}{{ end }} {
		flush_interval -1
	}
}
```

---

## Template Validation Safety

Every custom template is rendered in-memory and validated using the Caddy binary inside the container (`caddy validate`) before deployment.

If a custom template contains an invalid directive, bad syntax, or broken references:
1. `caddy validate` fails inside the container.
2. `caddy-config` logs the exact error returned by Caddy.
3. The invalid temporary file is discarded.
4. **Caddy continues serving the existing, working configuration without interruption.**
