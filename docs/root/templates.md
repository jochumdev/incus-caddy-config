---
description: Guide to customizing Caddyfile site blocks using inline Go templates and external template files.
editor: markdown
published: true
title: Custom Vhost Templates
leafwiki_id: ICZjDP_vg
leafwiki_title: Custom Vhost Templates
leafwiki_created_at: "2026-09-15T22:05:21Z"
leafwiki_updated_at: "2026-09-15T22:05:21Z"
leafwiki_creator_id: system
leafwiki_last_author_id: system
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

## Customizing the Global Options Block

By default, `caddy-config` prepends a minimal global options block to every rendered Caddyfile:

```caddyfile
{
	admin localhost:2019
}
```

You can overwrite this block to configure global settings such as TLS certificates, global logging, email, acme CA endpoints, or trusted proxies.

### Specifying a Global Template (`--global-template`)

You specify a custom global template per target route label using `--global-template` (or `INCUS_CADDY_GLOBAL_TEMPLATE`). The flag takes the format `<label>:<path-or-template>` and can be repeated to configure each target label independently:

**As a file path:**
```bash
caddy-config run \
  --caddy-instance edge:default:caddy \
  --global-template edge:/etc/caddy/global.caddyfile
```

**Targeted multi-instance configuration:**
```bash
caddy-config run \
  --caddy-instance external:default:caddy-external \
  --caddy-instance internal:default:caddy-internal \
  --global-template external:/etc/caddy/external.global.caddyfile \
  --global-template internal:/etc/caddy/internal.global.caddyfile
```

**As an inline template string:**
```bash
caddy-config run \
  --caddy-instance edge:default:caddy \
  --global-template 'edge:{
	admin localhost:2019
	email admin@example.com
}'
```

Any deployment target whose label does not have an explicit `--global-template` specified uses the default minimal options block (`{ admin localhost:2019 }`).

### Global Template Context Variables

The global template is evaluated with Go's `text/template` engine and has access to:

| Variable | Type | Description |
|---|---|---|
| `.Vhosts` | `[]vhost` | List of all virtual hosts configured for this deployment target. |

Example:
```caddyfile
{
	admin localhost:2019
	{{- if .Vhosts }}
	# Configured for {{ len .Vhosts }} active routes
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

### 2. External Template Directory (`--templates-dir`)

For reusable site configurations, store templates in a directory mounted to `caddy-config` and pass `--templates-dir /etc/caddy/templates`.

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
