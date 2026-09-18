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
--caddy-instance [<label>,]instance=<instance>,project=<project>[,<key>=<value>...]
--os-path [<label>,]path=<path>[,<key>=<value>...]
```

For example:
```bash
# Remote Incus instance
--caddy-instance edge,instance=caddy-prod,project=default

# Remote Incus instance with custom flags (e.g. global template)
--caddy-instance edge,instance=caddy-prod,project=default,global_template=/etc/caddy/global.caddyfile

# Local OS Caddyfile (defaults to label "caddy")
--os-path path=/etc/caddy/Caddyfile

# Local OS Caddyfile with custom label prefix and reload flag
--os-path edge,path=/etc/caddy/Caddyfile,reload=custom
```

Targets can be specified multiple times, or comma- or whitespace-separated within a single flag or environment variable, to route different subsets of services to distinct Caddy targets.

---

## Label Prefixing & Compose Syntax

In Incus, user-defined labels carry the `user.label.` prefix.

When using **`incus-compose`**, the `user.label.` prefix is automatically added to all entries under the `labels:` section.

| In `compose.yaml` | In Incus (`incus config show <instance>`) |
|---|---|
| `labels: edge: "example.com,upstream=8080"` | `user.label.edge: "example.com,upstream=8080"` |

---

## Label Reference

For a target bound to prefix `edge`:

| Label | Description | Example |
|---|---|---|
| `user.label.edge` | **(Required)** Domain name(s) to match, with optional flags (e.g. `upstream=8080`, `network=<iface>`, `redir=<url>`, `template=<file>`, `resolvers='1.1.1.1 1.0.0.1'`). Multiple domains are separated by spaces. | `api.example.com,upstream=8080` or `app.lan,network=eth0,template=spa.caddyfile` |
| `user.label.edge.redirs` | Plain redirection domain(s) mapping to primary domain. Options: `,uri` (default), `,no-uri`, or `,template=<file>`. | `www.example.com,no-uri old.example.com,template=redir.caddyfile` |
| `user.label.edge.service` | Custom service name override (defaults to `user.incus-compose.service`). Groups instance replicas together. | `payments-api` |

---

## Structured Labels Format

As an alternative to compressed single-string label values, `caddy-config` supports a structured label format using individual key-value pairs:

```yaml
services:
  wiki:
    labels:
      caddy-external.0.domain: example.com
      caddy-external.0.upstream: 8080
      caddy-external.0.redirs.0: www.example.com,uri
      caddy-external.0.redirs.1: wiki.example.com,uri
      caddy-external.0.redirs.2: docs.example.com,uri
      caddy-internal.0.domain: example.com
      caddy-internal.0.upstream: 8080
      caddy-internal.0.redirs.0: www.example.com,uri,template=internal_acme.caddyfile
      caddy-internal.0.redirs.1: wiki.example.com,uri,template=internal_acme.caddyfile
      caddy-internal.0.redirs.2: docs.example.com,uri,template=internal_acme.caddyfile
```

### Schema & Keys

For target label `<L>` and route index `<n>` (non-negative integer):

| Label Key | Description | Example |
|---|---|---|
| `<L>.<n>.domain` | **(Required)** Domain name(s) to match for route `<n>`. | `example.com` |
| `<L>.<n>.upstream` | Upstream port (e.g. `8080`) or `host:port`. | `8080` |
| `<L>.<n>.template` | Custom Caddyfile site template for route `<n>`. | `internal_acme.caddyfile` |
| `<L>.<n>.network` | Network interface name for IP resolution. | `eth0` |
| `<L>.<n>.redir` | Canonical redirect target (instead of upstream). | `https://new.example.com{uri}` |
| `<L>.<n>.redirs.<m>` | Alias redirect domain with optional flags (e.g. `,uri`, `,no-uri`, `,template=<file>`). | `www.example.com,uri` |

### Rules

- **Service is once per instance**: Service name is set per instance (`user.label.<L>.service` or compose service), not per route index.
- **No mixing formats on a single instance**: An instance must use either legacy compressed labels or structured labels for a given target label. Mixing both formats on the same instance logs an error and skips the instance. Different instances in the same project can use different formats.
- **Required domain**: Each route index `<n>` must define `.domain`. If omitted, an error is logged and route `<n>` is skipped.
- **Sparse indices**: Index gaps are valid (e.g. indices `0` and `2` without `1`). Routes are sorted and evaluated numerically.
- **Redirection indices**: Multiple redirect domains use `.redirs.<m>` with ascending numeric indices `<m>`.


---

## IPv4 Address Resolution

`caddy-config` resolves upstream container IP addresses dynamically:

1. **Target Network Matching**:
   If the `network` flag is set on the target label (e.g. `,network=internal` or `user.label.<prefix>.network`), `caddy-config` searches the instance's interfaces for one attached to `internal` and selects its first non-loopback IPv4 address.
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
      edge: "web.example.test,upstream=8080"
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
      edge: "portal.example.com app.example.com,upstream=80"
```

Rendered Caddyfile:
```caddyfile
portal.example.com app.example.com {
	reverse_proxy 10.0.1.18:80
}
```

### 3. Automatic Load Balancing (Multiple Replicas)

When multiple instances define the same `edge` target label, `caddy-config` merges their upstreams into a single sorted load-balanced `reverse_proxy` directive:

```yaml
services:
  api1:
    image: docker.io/library/busybox:latest
    command: httpd -f -p 8080
    labels:
      edge: "api.example.com,upstream=8080"

  api2:
    image: docker.io/library/busybox:latest
    command: httpd -f -p 8080
    labels:
      edge: "api.example.com,upstream=8080"
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
      edge: "old.example.com,redir=https://new.example.com{uri}"
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
      edge: "example.com,upstream=80"
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
- `template=<file>`: custom template file in `--templates-dir` or absolute path (e.g. `template=redir.caddyfile`) to render the redirect block instead of the default `redir` block.

### 6. Multi-Network Instance (Specific Network)

If an instance is connected to both a private management network (`mgmt`) and an internal service bridge (`appbr0`), explicitly pick the interface for reverse proxying via the `network` flag:

```yaml
services:
  backend:
    image: docker.io/library/nginx:alpine
    labels:
      edge: "backend.internal,upstream=8000,network=appbr0"
```

