---
description: Quickstart guide to running Caddy and incus-templated on Incus with automated reverse proxying.
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

This guide walks through deploying Caddy alongside `incus-templated` on Incus using `incus-compose`.

---

## Prerequisites

1. **Incus Server**: Running Incus 7.0+.
2. **Storage Volume for Caddy**: Caddy requires a persistent storage volume mounted to `/config` to preserve configuration across reboots and recreations.

---

## Step 1: Generate an Incus Trust Token

`incus-templated` communicates with the Incus daemon over HTTPS. You can enroll it using a one-time trust token:

```bash
incus config trust add incus-templated
```

Incus outputs a trust token (e.g. `eyJzZXJ2ZXJfbmFtZSI6...`). Save this token into a `.env` file in the same directory as your `compose.yaml`:

```bash
echo "INCUS_TOKEN=eyJzZXJ2ZXJfbmFtZSI6..." > .env
```

`incus-compose` automatically loads `.env` files and injects secrets into the stack without requiring `--os-env`.

---

## Step 2: Define `compose.yaml`

Create a `compose.yaml` file defining the Caddy proxy and `incus-templated`:

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
    command: |
      sh -c 'if [ ! -f /config/Caddyfile ]; then echo -e "{\n\tadmin localhost:2019\n}\n:80 {\n\trespond \"Caddy initializing...\" 503\n}\n" > /config/Caddyfile; fi; exec caddy run --config /config/Caddyfile --adapter caddyfile'

  incus-templated:
    image: ghcr.io/jochumdev/incus-templated/incus-templated:1.0.0-beta.1
    restart: unless-stopped
    depends_on:
      caddy:
        condition: service_healthy
    # ports:
    #   - "9153:9153"
    environment:
      INCUS_TEMPLATED_INCUS: "${INCUS_TEMPLATED_INCUS:-https://10.0.1.1:8443}"
      INCUS_TEMPLATED_DATA_DIR: /var/lib/incus-templated
      INCUS_TEMPLATED_TARGETS: "edge,preset=caddy,instance=caddy,project=default"
      INCUS_TEMPLATED_HTTP_ADDRESS: ":9153"
      INCUS_TEMPLATED_LOG: "INFO"
    secrets:
      - token
    volumes:
      - incus-templated:/var/lib/incus-templated

secrets:
  token:
    environment: INCUS_TOKEN

volumes:
  config:
  data:
  incus-templated:
```

Launch the stack:

```bash
incus-compose up -d
```

On first start:

1. `incus-templated` reads the secret token from `/run/secrets/token`.
2. It generates a client TLS certificate and enrolls it with the Incus API.
3. The enrolled certificate is persisted in the `config-data` volume at `/var/lib/incus-templated/client.crt` and `client.key`. Future restarts reuse the certificate without requiring a token.

---

## Step 3: Route an Application

To expose any container or virtual machine through Caddy, add an `edge` label with an `upstream` flag:

```yaml
services:
  api:
    image: docker.io/library/busybox:latest
    command: httpd -f -p 8080
    labels:
      edge: "api.example.test,upstream=8080"
```

Deploy the service:

```bash
incus-compose up -d api
```

`incus-templated` immediately catches the `instance-started` event, resolves the instance's bridge IP address, writes the updated configuration directly to the `incus-templated` storage volume, validates syntax inside Caddy, and executes a reload.

Verify that Caddy routes traffic to the new service:

```bash
curl -H "Host: api.example.test" http://127.0.0.1/
```

---

## Step 4: Verify Readiness and Health

`incus-templated` exposes observability endpoints on port `9153`:

- **Liveness** (`GET http://localhost:9153/health`): Returns HTTP 200 once the HTTP server is listening.
- **Readiness** (`GET http://localhost:9153/ready`):
  - Returns **HTTP 200 OK** once the initial Incus fleet sweep finishes (`ChainWarm`).
  - Returns **HTTP 503 Service Unavailable** while the chain is cold (`ChainCold`) or disconnected.

```bash
curl -i http://localhost:9153/ready
```

---

## Alternative: Co-located Deployment on Host or VM

If you run Caddy directly on the host OS or inside a VM alongside `incus-templated`, give the target a `path=` instead of `instance=` and `project=`:

```bash
# Listen to Incus and update local Caddyfile directly
incus-templated run \
  --target edge,preset=caddy,path=/etc/caddy/Caddyfile \
  --remote default
```

`incus-templated` validates the syntax and reloads the local Caddy daemon directly on the host filesystem.

---

## Next Steps

- Explore label options in [Caddy Labels & Routing](/labels).
- Render anything else, or customize reverse proxy blocks, in [Templates](/templates).
- Review all configuration options in [Configuration Reference](/configuration).
