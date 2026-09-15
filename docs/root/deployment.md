---
description: Production deployment guide, volume setup, container reboot persistence, and troubleshooting.
editor: markdown
published: true
title: Deployment & Operations
leafwiki_id: eXZjvElvg
leafwiki_title: Deployment & Operations
leafwiki_created_at: "2026-09-15T22:05:21Z"
leafwiki_updated_at: "2026-09-15T22:05:21Z"
leafwiki_creator_id: system
leafwiki_last_author_id: system
---

# Deployment & Operations

This guide covers operational practices for running `caddy-config` in production environments.

---

## Production `compose.yaml` Stack

Here is the recommended production stack deploying Caddy alongside `caddy-config`:

```yaml
services:
  caddy:
    image: docker.io/library/caddy:2.11.4-alpine
    restart: unless-stopped
    ports:
      - "80:80"
      - "443:443"
    volumes:
      - caddy-config:/config
      - caddy-data:/data
    command: >-
      sh -c '
      if [ ! -f /config/Caddyfile ]; then
        echo "{\n\tadmin localhost:2019\n}\n:80 {\n\trespond \"Initializing edge proxy...\" 503\n}\n" > /config/Caddyfile;
      fi;
      exec caddy run --config /config/Caddyfile --adapter caddyfile'

  caddy-config:
    image: ghcr.io/lxc/incus-caddy-config:latest
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
      - config-data:/var/lib/caddy-config
    depends_on:
      caddy:
        condition: service_started

secrets:
  token:
    environment: INCUS_TOKEN

volumes:
  caddy-config:
  caddy-data:
  config-data:
```

### Purpose of Each Volume

1. **`caddy-config:/config`**:
   Persistent Incus custom storage volume holding `/config/Caddyfile`. `caddy-config` connects directly to this volume over SFTP to write and update the configuration.
2. **`caddy-data:/data`**:
   Persistent volume where Caddy stores automatic TLS certificates (from Let's Encrypt / ZeroSSL) and OCSP stapling cache.
3. **`config-data:/var/lib/caddy-config`**:
   Stores `caddy-config`'s enrolled client TLS certificate (`client.crt`, `client.key`) so trust tokens are only needed once.

---

## Reboot & Lifecycle Behavior

### Container Restarts & Host Reboots

Because `/config` is backed by a custom storage volume:
- When the Caddy container restarts (`incus-compose restart caddy`) or the Incus host reboots, Caddy immediately starts up using the persisted `Caddyfile` on the volume.
- Traffic continues to be served immediately on boot without waiting for `caddy-config` to initialize.

### Offline / Cold Reconciliations

If Caddy is temporarily stopped (e.g. during maintenance or image upgrades):
1. `caddy-config` detects the container is stopped.
2. It resolves the storage volume mounted at `/config` and writes the updated `Caddyfile` directly to the volume via SFTP.
3. Because Caddy is not running, in-container validation and reload are skipped.
4. When Caddy starts up, it reads the updated configuration from the storage volume on its first boot cycle.

---

## Multi-Project Routing

By default, `caddy-config` monitors all visible Incus projects. You can restrict monitoring to specific projects:

```bash
caddy-config run \
  --project prod \
  --project staging \
  --caddy-instance edge:default:caddy
```

### Multiple Caddy Servers

You can route different services to different Caddy instances using separate label prefixes:

```bash
caddy-config run \
  --caddy-instance public:default:caddy-external \
  --caddy-instance internal:default:caddy-internal
```

- Instances tagged with `user.label.public.domain` route to `caddy-external`.
- Instances tagged with `user.label.internal.domain` route to `caddy-internal`.

---

## Operational Troubleshooting

### 1. View Current Deployed Caddyfile

Inspect the active configuration directly inside the Caddy container:

```bash
incus-compose exec caddy cat /config/Caddyfile
```

### 2. Check Controller Logs

Increase log verbosity using `INCUS_CADDY_LOG=DEBUG` or `TRACE`:

```bash
incus-compose logs -f caddy-config
```

`DEBUG` logs display:
- Incoming Incus events and actions (`instance-started`, `instance-stopped`, `instance-renamed`).
- Discovered network interfaces and resolved IPv4 addresses.
- In-container `caddy validate` exit codes and output.
- Direct storage volume SFTP staging paths.

### 3. Test In-Container Validation Manually

If you suspect a configuration syntax issue:

```bash
incus-compose exec caddy caddy validate --config /config/Caddyfile --adapter caddyfile
```

### 4. Common Error Scenarios

- **`opening SFTP session for volume ... not found`**:
  Verify that Caddy's `/config` directory is backed by a named storage volume in `compose.yaml`.
- **`executing caddy reload: connection refused`**:
  Verify that Caddy's global block includes `{ admin localhost:2019 }`. If `admin off` was specified, Caddy cannot process reload signals over its internal Admin API.
- **`readiness 503 Service Unavailable`**:
  Indicates the chain is still performing the initial fleet sweep or is disconnected from the Incus API. Check network connectivity to Incus.
