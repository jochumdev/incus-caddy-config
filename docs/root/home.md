---
description: Dynamic Caddy reverse proxy configuration driven by Incus instance events via ievent.
editor: markdown
published: true
tags: []
title: Incus Caddy Config
leafwiki_id: frWCDE_vR
leafwiki_title: Incus Caddy Config
leafwiki_created_at: "2026-09-15T22:05:21Z"
leafwiki_updated_at: "2026-09-15T22:23:42.502291694Z"
leafwiki_creator_id: system
leafwiki_last_author_id: zyZjvP_vg
---
# Incus Caddy Config

Dynamic, zero-touch reverse proxy configuration for [Caddy](https://caddyserver.com/) on [Incus](https://linuxcontainers.org/incus/), driven by instance lifecycle events.

`caddy-config` watches your Incus fleet via the `ievent` stream, renders Caddyfiles from instance labels in real time, validates them in-container, and deploys them directly into persistent storage volumes via Incus SFTP.

```mermaid
flowchart TD
    Incus["Incus API (Events & Control)"] -->|instance events| Chain["ievent Chain<br/>(source &rarr; debounce &rarr; enricher)"]
    Chain --> Plugin["caddy-config Plugin"]

    subgraph Storage["Incus Storage Pool"]
        Vol[("custom/caddy-config volume<br/>mounted at /config")]
    end

    subgraph Caddy["Serving Caddy Container"]
        CaddyProc["caddy:2.11.4-alpine<br/>(admin localhost:2019)"]
    end

    Plugin -->|"1. SFTP write (.Caddyfile.tmp)"| Vol
    Plugin -->|"2. Incus exec ('caddy validate')"| CaddyProc
    Plugin -->|"3. SFTP atomic rename (.Caddyfile.tmp &rarr; Caddyfile)"| Vol
    Plugin -->|"4. Incus exec ('caddy reload')"| CaddyProc
```

## Quick Example

Define Caddy and your backend application in `compose.yaml`:

```yaml
services:
  caddy:
    image: docker.io/library/caddy:2.11.4-alpine
    restart: unless-stopped
    ports:
      - "80:80"
      - "443:443"
    volumes:
      - config:/config
      - data:/data

  api:
    image: docker.io/library/busybox:latest
    command: httpd -f -p 8080
    labels:
      edge.domain: "api.example.com"
      edge.upstream: "8080"

volumes:
  config:
  data:
```

When `caddy-config` runs with `--caddy-instance edge:default:caddy`, it automatically discovers `api`, resolves its Incus bridge IPv4 address, and renders the reverse proxy block into Caddy:

```caddyfile
{
	admin localhost:2019
}

api.example.com {
	reverse_proxy 10.0.1.42:8080
}
```

When you scale, stop, start, or rename instances, Caddy reloads within milliseconds of the Incus event.

---

## Key Highlights

- **Zero Admin Port Exposure**: Caddy's Admin API is bound strictly to `localhost:2019` inside the container. No admin port is mapped to the host or public network.
- **Direct Storage Volume SFTP**: Configurations are written directly to the underlying Incus storage volume (`conn.GetStoragePoolVolumeFileSFTP`). If Caddy reboots or starts cold, it immediately boots with the current configuration.
- **Safe Atomic Swaps**: Renders in-memory, writes to `/.Caddyfile.tmp`, verifies syntax with `caddy validate` inside the container, and atomically swaps via `sftp.PosixRename`. Broken configurations are rejected before touching the active site.
- **Single Goroutine Concurrency**: State reconciliation is strictly confined to a single goroutine (Rule A4). No mutexes, lock contention, or race hazards.
- **Warm Gating**: Changes are suppressed while the event chain is cold (`ChainCold`). Deployments only run once the fleet sweep completes (`ChainWarm`), eliminating route churn on startup.
- **Flexible Custom Templating**: Full support for custom site blocks via inline Go templates or external template directories (`--templates-dir`).

---

## Documentation Navigation

- [Getting Started](/getting-started) - Prerequisites, installation, and first deployment
- [Architecture & Security](/architecture) - Process isolation, volume resolution, and concurrency model
- [Instance Labels & Routing](/labels) - Label specification, network resolution, and load balancing
- [Custom Vhost Templates](/templates) - Advanced site blocks, template variables, and examples
- [Configuration Reference](/configuration) - CLI flags, environment variables, and HTTP endpoints
- [Deployment & Operations](/deployment) - Production compose stacks, reboot survival, and troubleshooting
