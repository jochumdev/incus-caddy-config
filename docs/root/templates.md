---
description: The data every target template sees, the template funcs, and the Caddy preset with its per-vhost templates.
editor: markdown
published: true
title: Templates
leafwiki_id: ICZjDP_vg
leafwiki_title: Templates
leafwiki_created_at: "2026-09-15T22:05:21Z"
leafwiki_updated_at: "2026-09-15T22:05:21Z"
leafwiki_creator_id: system
leafwiki_last_author_id: system
---

# Templates

Every target renders one Go [`text/template`](https://pkg.go.dev/text/template) into one file. A target names its template with `template=<file>` (resolved against `--templates-dir` unless absolute) or takes one from a preset (`preset=caddy`). Templates are read once at startup.

---

## Template Data

The template is executed with:

| Field | Type | Description |
|---|---|---|
| `.Label` | `string` | The target's label. |
| `.Flags` | `map[string]string` | The target's own extra `key=value` options (e.g. `job=incus`). |
| `.Instances` | `[]Instance` | Every **running** instance carrying the label, sorted by project, then name. |

Each `Instance`:

| Field | Type | Description |
|---|---|---|
| `.Name` | `string` | Instance name. |
| `.Project` | `string` | Incus project. |
| `.Location` | `string` | Cluster member the instance runs on; empty off a cluster. |
| `.Label` | `string` | Value of `user.label.<label>`. |
| `.Labels` | `map[string]string` | Keys below the label with the prefix cut: `user.label.prom.port` is `.Labels.port`. |
| `.Config` | `map[string]string` | Full expanded configuration, without `volatile.*` keys. |
| `.Devices` | `map[string]map[string]string` | Expanded devices by name. |
| `.Interfaces` | `[]Interface` | One per NIC: `.Network`, `.Project`, `.IPv4`, `.IPv6`. |
| `.IPv4 "<network>"` | method | First non-loopback IPv4 on that network, or on any network when it has none; `.IPv4 ""` for any. |

A label change, an address change, or a move to another cluster member re-renders the target.

A label or flag an instance or target does not carry renders as an empty string, so `{{ .Labels.port }}` on an instance without `port` gives nothing rather than `<no value>`. Test for it with `{{ if .Labels.port }}`.

## Template Funcs

Besides the Go built-ins (`index`, `printf`, `len`, `range`, ...):

| Func | Description |
|---|---|
| `json <value>` | JSON-encode a value: `{{ json .Name }}` gives `"web-1"`. |
| `join <list> <sep>` | `strings.Join`. |
| `split <string> <sep>` | `strings.Split`. |
| `vhosts .` | Caddy: group the instances into site blocks (see [Caddy Preset](#caddy-preset)). |
| `vhost <v>` | Caddy: render one site block. |

---

## Examples

### Prometheus `file_sd`

Prometheus watches the file itself, so the target needs no commands:

```bash
incus-templated run --templates-dir /etc/incus-templated/templates \
  --target prom,path=/etc/prometheus/sd/incus.json,template=file_sd.json.tmpl,job=incus
```

```text
{{/* /etc/incus-templated/templates/file_sd.json.tmpl */}}
[
{{- range $i, $inst := .Instances }}{{ if $i }},{{ end }}
  {
    "targets": [{{ json (printf "%s:%s" ($inst.IPv4 "") $inst.Labels.port) }}],
    "labels": {
      "job": {{ json $.Flags.job }},
      "instance": {{ json $inst.Name }},
      "project": {{ json $inst.Project }},
      "location": {{ json $inst.Location }}
    }
  }
{{- end }}
]
```

```yaml
services:
  node-exporter:
    image: quay.io/prometheus/node-exporter:latest
    labels:
      prom: ""
      prom.port: "9100"
```

### Upstreams on the cluster member's address

When instances sit on a per-member NAT bridge and are reached through a proxy device on the member's address, the upstream depends on where the instance runs. `.Location` follows the instance across members, so the mapping lives in the template. Keep the member addresses on the target as flags (`member.node1=10.0.0.11`) and look them up by location:

```text
{{- range .Instances }}
{{ .Labels.domain }} {
	reverse_proxy {{ index $.Flags (printf "member.%s" .Location) }}:{{ .Labels.upstream }}
}
{{- end }}
```

```bash
--target edge,path=/etc/caddy/Caddyfile,template=members.caddyfile,format='caddy fmt --overwrite {staging}',reload='caddy reload --config {path}',member.node1=10.0.0.11,member.node2=10.0.0.12
```

---

## Caddy Preset

`preset=caddy` renders this template (see [Configuration](configuration#presets) for the commands it sets):

```caddyfile
{
	admin localhost:2019
}
{{- range vhosts . }}

{{ vhost . }}
{{- end }}
```

`vhosts .` reads the target's instances with the [label rules](labels): domains, `upstream=`, `network=`, `redir=`, redirection domains, structured `.N.` routes, and load balancing across instances sharing a domain. `vhost` renders one site block with the block's own `template=` or the default below.

To change the global options block, copy the template above into your own file and point the target at it: `--target edge,preset=caddy,instance=caddy,project=default,template=edge.caddyfile`. The preset still provides the commands.

### Default Vhost Template

```caddyfile
{{ .Domain }} {
{{- if .Redirect }}
	redir {{ .Redirect }} permanent
{{- else if .Upstreams }}
	reverse_proxy {{ range $i, $u := .Upstreams }}{{ if $i }} {{ end }}{{ $u }}{{ end }}
{{- end }}
}
```

### Vhost Variables

A per-vhost template is executed with one site block:

| Variable | Type | Description |
|---|---|---|
| `.Domain` | `string` | The domain(s) defined on the instance (`user.label.<prefix>`). |
| `.Service` | `string` | The service name if defined (`user.label.<prefix>.service` or `user.incus-compose.service`). |
| `.Upstreams` | `[]string` | Sorted list of resolved upstream addresses (e.g. `["10.0.1.5:8080", "10.0.1.6:8080"]`). |
| `.Redirect` | `string` | The redirect URL if configured via `redir=<url>` flag on domain. |
| `.Template` | `string` | The raw template name or inline template string (`template=<path>` flag). |
| `.Flags` | `map[string]string` | Custom flags parsed from label definitions (e.g. `example.com,flag1=value1,resolvers='1.1.1.1 1.0.0.1'`). Single or double quotes preserve spaces and commas. |

---

### Per-Vhost Templates

Custom templates are specified directly on the target label using the `template=<value>` flag:

#### From the templates directory

For reusable site configurations, store templates in a directory mounted to `incus-templated` and pass `--templates-dir /etc/caddy/templates`.

When an instance specifies a valid path with an extension (dir+file+ext, e.g. `template=spa_site.caddyfile` or `template=spa_site.tmpl`), `incus-templated` resolves it against `--templates-dir` (or uses an absolute path if provided). If the file does not exist, rendering fails and the live Caddyfile stays as it is.

```yaml
services:
  web:
    image: docker.io/library/nginx:alpine
    labels:
      edge: "spa.example.com,upstream=8080,template=spa_site.caddyfile"
```

#### Inline

Any template input without a file extension is evaluated as an inline Go template:

```yaml
services:
  web:
    image: docker.io/library/nginx:alpine
    labels:
      edge: |
        spa.example.com,upstream=8080,template={{ .Domain }} {
        	encode gzip zstd
        	reverse_proxy {{ index .Upstreams 0 }}
        }
```

---

### Per-Vhost Examples

#### Example 1: Compression and Security Headers

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
      edge: "secure.example.com,upstream=3000,template=secure_proxy.caddyfile"
```

---

#### Example 2: Static Files with Reverse Proxy Fallback

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

#### Example 3: PHP-FPM FastCGI Site

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
      edge: "blog.example.com,upstream=9000,template=php_site.caddyfile"
```

---

#### Example 4: WebSockets & Long-Lived Streaming

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

## Validation Safety

Every rendered file is written to `<path>.tmp` and checked by the target's `format` and `validate` commands before it replaces the live file. With the Caddy preset that is `caddy fmt` and `caddy validate`, run inside the Caddy container.

If either fails:
1. `incus-templated` logs the command's output.
2. The live file is not touched; the `.tmp` file is left for inspection.
3. **The consumer keeps serving the existing, working configuration.**
