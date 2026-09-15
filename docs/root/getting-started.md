---
description: Quickstart guide to running Caddy and caddy-config on Incus with automated reverse proxying.
editor: markdown
published: true
tags: []
title: Getting Started
leafwiki_id: drZjvElvg
leafwiki_title: Getting Started
leafwiki_created_at: "2026-09-15T22:05:21Z"
leafwiki_updated_at: "2026-09-15T22:23:15.032427752Z"
leafwiki_creator_id: system
leafwiki_last_author_id: zyZjvP_vg
---

# Getting Started

This guide walks through deploying Caddy alongside `caddy-config` on Incus using `incus-compose`.

---

## Prerequisites

1. **Incus Server**: Running Incus 7.0+.
2. **Storage Volume for Caddy**: Caddy requires a persistent storage volume mounted to `/config` to preserve configuration across reboots and recreations.

---

## Step 1: Generate an Incus Trust Token

`caddy-config` communicates with the Incus daemon over HTTPS. You can enroll it using a one-time trust token:

```bash
incus config trust add caddy-config
```

Incus outputs a trust token (e.g. `eyJzZXJ2ZXJfbmFtZSI6...`). Save this token into a `.env` file in the same directory as your `compose.yaml`:

```bash
echo "INCUS_TOKEN=eyJzZXJ2ZXJfbmFtZSI6..." > .env
```

`incus-compose` automatically loads `.env` files and injects secrets into the stack without requiring `--os-env`.

---

## Step 2: Define `compose.yaml`

Create a `compose.yaml` file defining the Caddy proxy and `caddy-config`:

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
    command: >-
      sh -c '
      if [ ! -f /config/Caddyfile ]; then
        echo "{\n\tadmin localhost:2019\n}\n:80 {\n\trespond \"Caddy initializing...\" 503\n}\n" > /config/Caddyfile;
      fi;
      exec caddy run --config /config/Caddyfile --adapter caddyfile'

  caddy-config:
    image: ghcr.io/jochumdev/incus-caddy-config/caddy-config:latest
    restart: unless-stopped
    ports:
      - "9153:9153"
    environment:
      INCUS_CADDY_INCUS: https://10.0.1.1:8443
      INCUS_CADDY_DATA_DIR: /var/lib/caddy-config
      INCUS_CADDY_INSTANCES: "edge:default:caddy"
      INCUS_CADDY_HTTP_ADDRESS: ":9153"
      INCUS_CADDY_LOG: "INFO"
    secrets:
      - token
    volumes:
      - caddy-config:/var/lib/caddy-config

secrets:
  token:
    environment: INCUS_TOKEN

volumes:
  config:
  data:
  caddy-config:
```

Launch the stack:

```bash
incus-compose up -d
```

On first start:

1. `caddy-config` reads the secret token from `/run/secrets/token`.
2. It generates a client TLS certificate and enrolls it with the Incus API.
3. The enrolled certificate is persisted in the `config-data` volume at `/var/lib/caddy-config/client.crt` and `client.key`. Future restarts reuse the certificate without requiring a token.

---

## Step 3: Route an Application

To expose any container or virtual machine through Caddy, add `edge.domain` and `edge.upstream` labels:

```yaml
services:
  api:
    image: docker.io/library/busybox:latest
    command: httpd -f -p 8080
    labels:
      edge.domain: "api.example.test"
      edge.upstream: "8080"
```

Deploy the service:

```bash
incus-compose up -d api
```

`caddy-config` immediately catches the `instance-started` event, resolves the instance's bridge IP address, writes the updated configuration directly to the `caddy-config` storage volume, validates syntax inside Caddy, and executes a reload.

Verify that Caddy routes traffic to the new service:

```bash
curl -H "Host: api.example.test" http://127.0.0.1/
```

---

## Step 4: Verify Readiness and Health

`caddy-config` exposes observability endpoints on port `9153`:

- **Liveness** (`GET http://localhost:9153/health`): Returns HTTP 200 once the HTTP server is listening.
- **Readiness** (`GET http://localhost:9153/ready`):
  - Returns **HTTP 200 OK** once the initial Incus fleet sweep finishes (`ChainWarm`).
  - Returns **HTTP 503 Service Unavailable** while the chain is cold (`ChainCold`) or disconnected.

```bash
curl -i http://localhost:9153/ready
```

---

## Next Steps

- Explore label options in [Instance Labels & Routing](/labels).
- Customize reverse proxy blocks in [Custom Vhost Templates](/templates).
- Review all configuration options in [Configuration Reference](/configuration).
