---
description: Reference guide for configuring dynamic Caddy reverse proxy routes using Incus instance labels.
editor: markdown
published: true
title: Instance Labels & Routing
leafwiki_id: erZCvElDR
leafwiki_title: Instance Labels & Routing
leafwiki_created_at: "2026-09-15T22:05:21Z"
leafwiki_updated_at: "2026-09-15T22:05:21Z"
leafwiki_creator_id: system
leafwiki_last_author_id: system
---

# Instance Labels & Routing

`caddy-config` derives routing rules from labels assigned to Incus containers and virtual machines.

---

## Target Binding Syntax

`caddy-config` connects a label prefix to a target Caddy server using `--caddy-instance` (for Incus containers/VMs) or `--os-path` (for local OS deployment):

```text
--caddy-instance <label>:<project>:<instance>
--os-path [<label>:]<path>
```

For example:
```bash
# Remote Incus instance
--caddy-instance edge:default:caddy-prod

# Local OS Caddyfile (defaults to label "caddy")
--os-path /etc/caddy/Caddyfile

# Local OS Caddyfile with custom label prefix
--os-path edge:/etc/caddy/Caddyfile
```

You can specify `--caddy-instance` and `--os-path` multiple times to route different subsets of services to distinct Caddy targets.

---

## Label Prefixing & Compose Syntax

In Incus, user-defined labels carry the `user.label.` prefix.

When using **`incus-compose`**, the `user.label.` prefix is automatically added to all entries under the `labels:` section.

| In `compose.yaml` | In Incus (`incus config show <instance>`) |
|---|---|
| `labels: edge.domain: "example.com"` | `user.label.edge.domain: "example.com"` |
| `labels: edge.upstream: "8080"` | `user.label.edge.upstream: "8080"` |

---

## Label Reference

For a target bound to prefix `edge`:

| Label | Description | Example |
|---|---|---|
| `user.label.edge.domain` | **(Required)** Domain name(s) to match, with optional flags (e.g. `example.com,flag1=value1`). Multiple domains are separated by spaces. | `api.example.com` or `app.lan web.lan` |
| `user.label.edge.upstream` | Target port or `host:port` override. If omitted, routes to container IP on default HTTP port. | `8080`, `3000`, or `10.0.1.50:9090` |
| `user.label.edge.network` | Incus network interface name to resolve IPv4 from. Defaults to the first valid non-loopback IPv4 address. | `incusbr0`, `eth0`, or `internal` |
| `user.label.edge.redirect` | Target URL for permanent redirects (renders `redir <url> permanent`). Defaults to appending `{uri}` unless `,no-uri` is specified. | `https://example.com` or `https://example.com,no-uri` |
| `user.label.edge.redirs` | Plain redirection domain(s) mapping to primary domain. Options: `,uri` (default) or `,no-uri`. | `www.example.com,no-uri old.example.com,uri` |
| `user.label.edge.template` | Custom vhost template name in `--templates-dir` or an inline Go template. | `php_site` or inline site block |
| `user.label.edge.service` | Custom service name override (defaults to `user.incus-compose.service`). Groups instance replicas together. | `payments-api` |

---

## IPv4 Address Resolution

`caddy-config` resolves upstream container IP addresses dynamically:

1. **Target Network Matching**:
   If `user.label.<prefix>.network` is set (e.g. `internal`), `caddy-config` searches the instance's interfaces for one attached to `internal` and selects its first non-loopback IPv4 address.
2. **First Available Non-Loopback IPv4**:
   If no network is specified (or the specified network is not attached), `caddy-config` selects the first non-loopback IPv4 address across all attached interfaces.
3. **Loopback & Invalid Address Exclusion**:
   Addresses matching `127.0.0.0/8` (loopback) or unparseable IP addresses are automatically skipped.

---

## Routing Patterns & Examples

### 1. Simple Reverse Proxy

Routes `http://web.example.test` to port `8080` of the `web` container:

```yaml
services:
  web:
    image: docker.io/library/nginx:alpine
    labels:
      edge.domain: "web.example.test"
      edge.upstream: "8080"
```

Rendered Caddyfile:
```caddyfile
web.example.test {
	reverse_proxy 10.0.1.15:8080
}
```

### 2. Multi-Domain Routing

To match multiple domains for the same service, separate them with spaces:

```yaml
services:
  portal:
    image: docker.io/library/nginx:alpine
    labels:
      edge.domain: "portal.example.com app.example.com"
      edge.upstream: "80"
```

Rendered Caddyfile:
```caddyfile
portal.example.com app.example.com {
	reverse_proxy 10.0.1.18:80
}
```

### 3. Automatic Load Balancing (Multiple Replicas)

When multiple instances define the same `edge.domain`, `caddy-config` merges their upstreams into a single sorted load-balanced `reverse_proxy` directive:

```yaml
services:
  api1:
    image: docker.io/library/busybox:latest
    command: httpd -f -p 8080
    labels:
      edge.domain: "api.example.com"
      edge.upstream: "8080"

  api2:
    image: docker.io/library/busybox:latest
    command: httpd -f -p 8080
    labels:
      edge.domain: "api.example.com"
      edge.upstream: "8080"
```

Rendered Caddyfile:
```caddyfile
api.example.com {
	reverse_proxy 10.0.1.20:8080 10.0.1.21:8080
}
```

If `api1` stops, `caddy-config` detects the stop event and updates Caddy to route solely to `api2`. When `api1` restarts, it is automatically restored to the pool.

### 4. Canonical Domain Redirect

To redirect one domain to another:

```yaml
services:
  old-site:
    image: docker.io/library/busybox:latest
    command: sh -c "sleep infinity"
    labels:
      edge.domain: "old.example.com"
      edge.redirect: "https://new.example.com{uri}"
```

Rendered Caddyfile:
```caddyfile
old.example.com {
	redir https://new.example.com{uri} permanent
}
```

### 5. Plain Redirections (`<label>.redirs`)

To redirect alternate or legacy domains (such as `www.` or alias domains) to the primary domain without running separate containers:

```yaml
services:
  web:
    image: docker.io/library/nginx:alpine
    labels:
      edge.domain: "example.com"
      edge.upstream: "80"
      edge.redirs: "www.example.com,uri old.example.com,no-uri"
```

Rendered Caddyfile:
```caddyfile
example.com {
	reverse_proxy 10.0.1.15:80
}

old.example.com {
	redir https://example.com permanent
}

www.example.com {
	redir https://example.com{uri} permanent
}
```

Flag options per domain:
- `uri` (default if omitted): preserves the request path and query (`redir https://example.com{uri} permanent`).
- `no-uri`: drops the request path and redirects to root (`redir https://example.com permanent`).

### 6. Multi-Network Instance (Specific Network)

If an instance is connected to both a private management network (`mgmt`) and an internal service bridge (`appbr0`), explicitly pick the interface for reverse proxying:

```yaml
services:
  backend:
    image: docker.io/library/nginx:alpine
    labels:
      edge.domain: "backend.internal"
      edge.upstream: "8000"
      edge.network: "appbr0"
```

