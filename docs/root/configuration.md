---
description: Complete specification of CLI flags, canonical environment variables, and authentication options.
editor: markdown
published: true
title: Configuration Reference
leafwiki_id: fXWCvE_vR
leafwiki_title: Configuration Reference
leafwiki_created_at: "2026-09-15T22:05:21Z"
leafwiki_updated_at: "2026-09-15T22:05:21Z"
leafwiki_creator_id: system
leafwiki_last_author_id: system
---

# Configuration Reference

`caddy-config` is configured via command-line flags or environment variables.

Every flag maps to exactly **one canonical environment variable** prefixed with `INCUS_CADDY_*` (plus the standard `INCUS_REMOTE`).

---

## Configuration Flags & Environment Variables

| Flag | Canonical Env Var | Default | Description |
|---|---|---|---|
| `--incus` | `INCUS_CADDY_INCUS` | | URL of the Incus API (e.g. `https://127.0.0.1:8443`). |
| `--token` | `INCUS_CADDY_TOKEN` | | One-time trust token. If omitted, reads from `--secrets-dir/token`. |
| `--data-dir` | `INCUS_CADDY_DATA_DIR` | `/var/lib/caddy-config` | Persistent directory storing the enrolled client TLS certificate. |
| `--secrets-dir` | `INCUS_CADDY_SECRETS_DIR` | `/run/secrets` | Directory holding secret files (e.g. `/run/secrets/token`). |
| `--client-cert` | `INCUS_CADDY_CLIENT_CERT` | | Path to an existing client TLS certificate (requires `--client-key`). |
| `--client-key` | `INCUS_CADDY_CLIENT_KEY` | | Path to the private key for `--client-cert`. |
| `--restricted` | `INCUS_CADDY_RESTRICTED` | `false` | Restrict enrolled certificate permissions to `--project`. |
| `--remote` | `INCUS_REMOTE` | | Connect using an existing remote from Incus CLI configuration. |
| `--use-remote` | `INCUS_CADDY_USE_REMOTE` | `false` | Allow Incus CLI configuration files (`~/.config/incus`) to be used. |
| `--project` | `INCUS_CADDY_PROJECTS` | | Monitored Incus project(s). Can be repeated. If empty, monitors all visible projects. |
| `--caddy-instance` | `INCUS_CADDY_INSTANCES` | | Target Caddy server specification in `label:project:instance` format (repeatable). |
| `--os-path` | `INCUS_CADDY_OS_PATH` | | Target local Caddyfile in `[label:]path` format (repeatable). Defaults label to `caddy`. |
| `--caddyfile-path` | `INCUS_CADDY_CADDYFILE_PATH` | `/config/Caddyfile` | Path to the active Caddyfile inside the Caddy container. |
| `--custom-templates-dir` | `INCUS_CADDY_CUSTOM_TEMPLATES_DIR` | | Local path to directory containing custom vhost templates. |
| `--global-template` | `INCUS_CADDY_GLOBAL_TEMPLATE` | | Path to custom global Caddyfile template or inline template in `label:path-or-template` format (repeatable). |
| `--debounce-window` | `INCUS_CADDY_DEBOUNCE_WINDOW` | `250ms` | Quiet period before flushing burst events to avoid redundant reloads. |
| `--http-address` | `INCUS_CADDY_HTTP_ADDRESS` | `:9153` | Listening address for `/health` and `/ready` endpoints. Empty disables HTTP server. |
| `--exclude` | `INCUS_CADDY_EXCLUDE` | | Optional chain stage to exclude (e.g. `http` or `debounce`). Repeatable. |
| `--log` | `INCUS_CADDY_LOG` | `INFO` | Log level: `TRACE`, `DEBUG`, `INFO`, `WARN`, `ERROR`. |
| `--pprof` | `INCUS_CADDY_PPROF` | `false` | Enable Go runtime `/debug/pprof` endpoints on `--http-address`. |
| `--workers` | `INCUS_CADDY_WORKERS` | `16` | Maximum concurrent Incus API reads during fleet enrichment sweeps. |
| `--read-timeout` | `INCUS_CADDY_READ_TIMEOUT` | `10s` | Timeout budget for a single instance read from the Incus daemon. |
| `--sweep-project-delay` | `INCUS_CADDY_SWEEP_PROJECT_DELAY` | `30s` | Delay between consecutive sweeps across projects. |
| `--sweep-read-delay` | `INCUS_CADDY_SWEEP_READ_DELAY` | `5s` | Delay between reads within a single project sweep. |

---

## Deployment Target Modes

`caddy-config` supports two target deployment models:

### 1. Remote Incus Instance (`--caddy-instance`)

Used when Caddy runs in an isolated Incus container or VM. Configuration is deployed over Incus SFTP directly into the underlying storage volume (or container rootfs) and validated/reloaded via `incus exec`:

```bash
caddy-config run \
  --caddy-instance edge:default:caddy \
  --caddyfile-path /config/Caddyfile
```

### 2. Local OS / Co-located Deployment (`--os-path`)

Used when `caddy-config` runs alongside Caddy on the same host, container, or VM:

```bash
# Default label prefix "caddy" -> /etc/caddy/Caddyfile
caddy-config run --os-path /etc/caddy/Caddyfile

# Custom label prefix "edge"
caddy-config run --os-path edge:/etc/caddy/Caddyfile

# Multiple local targets
caddy-config run \
  --os-path public:/etc/caddy/Caddyfile \
  --os-path internal:/etc/caddy/internal.caddyfile
```

In this mode:
- Staging writes to `.<base>.tmp` and atomically swaps using `os.Rename`.
- Configuration syntax is validated locally via `caddy validate --config <staging> --adapter caddyfile`.
- Caddy is reloaded locally via `caddy reload --config <path> --adapter caddyfile`. If Caddy is not currently running, the validated file remains on disk for Caddy to use upon startup.

---

## Authentication Methods

`caddy-config` supports three authentication methods to connect to Incus:

### 1. One-Time Trust Token (Recommended for Containers)

Generate a token on the Incus host:
```bash
incus config trust add caddy-config
```

Provide the token via a `.env` file (loaded automatically by `incus-compose` without `--os-env`):
```bash
echo "INCUS_TOKEN=eyJzZXJ2ZXJfbmFtZSI6..." > .env
```

And reference it via a Compose secret mounted to `/run/secrets/token`:
```yaml
secrets:
  token:
    environment: INCUS_TOKEN
```

On first run, `caddy-config` enrolls with Incus, generates its own TLS client certificate, and stores it in `--data-dir` (`/var/lib/caddy-config`). On subsequent boots, it authenticates using the stored certificate.

### 2. Pre-Generated Client Certificate & Key

If you manage certificates externally:
```bash
caddy-config run \
  --incus https://10.0.1.1:8443 \
  --client-cert /etc/ssl/caddy-config.crt \
  --client-key /etc/ssl/caddy-config.key \
  --caddy-instance edge:default:caddy
```

### 3. Incus CLI Configuration (`--remote`)

When running directly on a machine where the `incus` CLI is configured (`~/.config/incus/config.yml`):
```bash
caddy-config run \
  --remote ict-daily-dev01-main \
  --use-remote \
  --caddy-instance edge:default:caddy
```

---

## Observability Endpoints

When `--http-address` is configured (default `:9153`), `caddy-config` serves the following HTTP endpoints:

### `/health` (Liveness)
- **Method**: `GET`
- **Response**: `200 OK`
- Indicates the `caddy-config` HTTP server is running and listening.

### `/ready` (Readiness)
- **Method**: `GET`
- **Response**:
  - `200 OK` when the chain state is **Warm** (`ChainWarm`). This means the initial fleet sweep is complete, instance IP addresses are discovered, and the Caddyfile has been deployed.
  - `503 Service Unavailable` when the chain state is **Cold** (`ChainCold`) during initial startup synchronization or network disconnections.

### `/debug/pprof/` (Profiling)
- Available when `--pprof` is set to `true`.
- Exposes standard Go `net/http/pprof` endpoints for goroutine inspection, CPU profiling, and heap allocation analysis.

---

## Chain Tuning & Optimization

- **`--debounce-window` (default `250ms`)**:
  When scaling services or restarting clusters, dozens of events may arrive in rapid succession. Debouncing buffers burst events within the quiet window, performing a single atomic Caddy validation and reload.
- **`--workers` (default `16`)**:
  Controls how many instance states are fetched in parallel from Incus during sweeps. On large clusters (100+ instances), increasing workers reduces the initial sweep duration.
- **`--exclude`**:
  Allows omitting optional pipeline stages:
  - `--exclude http`: Disables the HTTP `/health` and `/ready` server entirely.
  - `--exclude debounce`: Disables event debouncing for immediate zero-latency event processing.
