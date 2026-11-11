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

`incus-templated` is configured via command-line flags or environment variables, and optionally a YAML file (`--config`) for targets.

Every flag maps to exactly **one canonical environment variable** prefixed with `INCUS_TEMPLATED_*` (plus the standard `INCUS_REMOTE`).

---

## Configuration Flags & Environment Variables

| Flag | Canonical Env Var | Default | Description |
|---|---|---|---|
| `--incus` | `INCUS_TEMPLATED_INCUS` | | URL of the Incus API (e.g. `https://127.0.0.1:8443`). |
| `--server-fingerprint` | `INCUS_TEMPLATED_SERVER_FINGERPRINT` | | Optional SHA-256 fingerprint to pin the Incus server certificate. |
| `--token` | `INCUS_TEMPLATED_TOKEN` | | One-time trust token. If omitted, reads from `--secrets-dir/token`. |
| `--data-dir` | `INCUS_TEMPLATED_DATA_DIR` | `/var/lib/incus-templated` | Persistent directory storing the enrolled client TLS certificate. |
| `--secrets-dir` | `INCUS_TEMPLATED_SECRETS_DIR` | `/run/secrets` | Directory holding secret files (e.g. `/run/secrets/token`). |
| `--client-cert` | `INCUS_TEMPLATED_CLIENT_CERT` | | Path to an existing client TLS certificate (requires `--client-key`). |
| `--client-key` | `INCUS_TEMPLATED_CLIENT_KEY` | | Path to the private key for `--client-cert`. |
| `--restricted` | `INCUS_TEMPLATED_RESTRICTED` | `false` | Restrict enrolled certificate permissions to `--project`. |
| `--remote` | `INCUS_REMOTE` | | Connect using an existing remote from Incus CLI configuration. |
| `--use-remote` | `INCUS_TEMPLATED_USE_REMOTE` | `false` | Allow Incus CLI configuration files (`~/.config/incus`) to be used. |
| `--project` | `INCUS_TEMPLATED_PROJECTS` | | Monitored Incus project(s). Can be repeated. If empty, monitors all visible projects. |
| `--target` | `INCUS_TEMPLATED_TARGETS` | | Target in `<label>,(instance=<inst>,project=<proj>\|path=<path>)[,preset=<name>\|template=<file>][,format=<cmd>][,validate=<cmd>][,reload=<cmd>][,<key>=<value>...]` format (repeatable or comma/space-separated). See [Targets](#targets). |
| `--config` | `INCUS_TEMPLATED_CONFIG` | | YAML file with more targets and `templates_dir`. See [Config file](#config-file). |
| `--templates-dir` | `INCUS_TEMPLATED_TEMPLATES_DIR` | | Directory relative `template=` paths are resolved against. Overrides `templates_dir` from `--config`. |
| `--debounce-window` | `INCUS_TEMPLATED_DEBOUNCE_WINDOW` | `250ms` | Quiet period before flushing burst events to avoid redundant reloads. |
| `--http-address` | `INCUS_TEMPLATED_HTTP_ADDRESS` | `:9153` | Listening address for `/health` and `/ready` endpoints. Empty disables HTTP server. |
| `--exclude` | `INCUS_TEMPLATED_EXCLUDES` | | Optional chain stage to exclude (e.g. `http` or `debounce`). Repeatable. |
| `--log` | `INCUS_TEMPLATED_LOG` | `INFO` | Log level: `TRACE`, `DEBUG`, `INFO`, `WARN`, `ERROR`. |
| `--pprof` | `INCUS_TEMPLATED_PPROF` | `false` | Enable Go runtime `/debug/pprof` endpoints on `--http-address`. |
| `--workers` | `INCUS_TEMPLATED_WORKERS` | `16` | Maximum concurrent Incus API reads during fleet enrichment sweeps. |
| `--read-timeout` | `INCUS_TEMPLATED_READ_TIMEOUT` | `10s` | Timeout budget for a single instance read from the Incus daemon. |
| `--sweep-project-delay` | `INCUS_TEMPLATED_SWEEP_PROJECT_DELAY` | `30s` | Delay between consecutive sweeps across projects. |
| `--sweep-read-delay` | `INCUS_TEMPLATED_SWEEP_READ_DELAY` | `5s` | Delay between reads within a single project sweep. |

---

## Targets

A target is one rendered file. It names:

- a **label**: instances carrying `user.label.<label>` (or any key below it, e.g. `user.label.<label>.0.domain`) feed it;
- **where it lands**: `instance=<inst>,project=<proj>` writes inside an Incus instance (path defaults to the preset's, or set `path=`), `path=<path>` alone writes to the local filesystem;
- **the template**: `preset=<name>` or `template=<file>`;
- optional **commands**: `format`, `validate`, `reload`;
- any other `key=value`: passed to the template as `.Flags`.

### Deployment flow

1. Render the template; if the bytes match what was last deployed, stop.
2. Write `<path>.tmp` (over SFTP into the instance's storage volume or rootfs, or to the local filesystem).
3. Run `format`, then `validate`, each against the staging file. A non-zero exit aborts and keeps the live file untouched.
4. Atomically rename `<path>.tmp` over `<path>`.
5. Run `reload`.

Commands are argv, not shell: no shell is needed in the target instance. Two placeholders are substituted in every argument: `{staging}` (the `.tmp` file) and `{path}` (the live file). Instance targets run them through `incus exec` inside that instance, local targets run them on the host. When the target instance is stopped, the file is still swapped in and the commands are skipped, so it boots with the new file.

A failed step is retried with backoff, up to 10 attempts.

### Presets

`preset=caddy` fills in whatever the target leaves unset:

| Field | Value |
|---|---|
| template | the built-in [Caddy template](templates#caddy-preset) |
| path | `/config/Caddyfile` (instance targets only; local targets must set `path=`) |
| format | `caddy fmt --overwrite {staging}` |
| validate | `caddy validate --config {staging}` |
| reload | `caddy reload --config {path} --adapter caddyfile` |

Set a field to override it, or to an empty value to turn the step off: `validate=` on a flag, `validate: []` in YAML.

### Examples

```bash
# Caddy in an Incus container.
incus-templated run --target edge,preset=caddy,instance=caddy,project=default

# Caddy on the same host, reloaded by systemd instead.
incus-templated run --target edge,preset=caddy,path=/etc/caddy/Caddyfile,reload='systemctl reload caddy'

# Prometheus file_sd: no commands, Prometheus watches the file.
incus-templated run --templates-dir /etc/incus-templated/templates \
  --target prom,path=/etc/prometheus/sd/incus.json,template=file_sd.json.tmpl,job=incus

# Several targets in one environment variable.
INCUS_TEMPLATED_TARGETS="public,preset=caddy,instance=caddy-prod,project=default internal,preset=caddy,instance=caddy-dev,project=infra" incus-templated run
```

Values containing spaces or commas are quoted: `reload='caddy reload --config {path}'`.

### Config file

Long commands read better in YAML. `--config` takes a file with `templates_dir` and a `targets` list using the same fields; commands are lists. Targets from the file are added before those from `--target`.

```yaml
templates_dir: /etc/incus-templated/templates

targets:
  - label: edge
    preset: caddy
    instance: caddy
    project: default

  - label: prom
    path: /etc/prometheus/sd/incus.json
    template: file_sd.json.tmpl
    flags:
      job: incus

  - label: lb
    path: /etc/haproxy/haproxy.cfg
    template: haproxy.cfg.tmpl
    validate: [haproxy, -c, -f, "{staging}"]
    reload: [systemctl, reload, haproxy]
```

Every target is checked at startup: a label is required, `instance` needs `project`, a path is required, no two targets may write the same file, and the template file must exist. A bad target stops `incus-templated` before it connects.

---

## Authentication Methods

`incus-templated` supports three authentication methods to connect to Incus:

### 1. One-Time Trust Token (Recommended for Containers)

Generate a token on the Incus host:
```bash
incus config trust add incus-templated
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

On first run, `incus-templated` enrolls with Incus, generates its own TLS client certificate, and stores it in `--data-dir` (`/var/lib/incus-templated`). On subsequent boots, it authenticates using the stored certificate.

### 2. Pre-Generated Client Certificate & Key

If you manage certificates externally:
```bash
incus-templated run \
  --incus https://10.0.1.1:8443 \
  --client-cert /etc/ssl/incus-templated.crt \
  --client-key /etc/ssl/incus-templated.key \
  --target edge,preset=caddy,instance=caddy,project=default
```

### 3. Incus CLI Configuration (`--remote`)

When running directly on a machine where the `incus` CLI is configured (`~/.config/incus/config.yml`):
```bash
incus-templated run \
  --remote ict-daily-dev01-main \
  --use-remote \
  --target edge,preset=caddy,instance=caddy,project=default
```

---

## Observability Endpoints

When `--http-address` is configured (default `:9153`), `incus-templated` serves the following HTTP endpoints:

### `/health` (Liveness)
- **Method**: `GET`
- **Response**: `200 OK`
- Indicates the `incus-templated` HTTP server is running and listening.

### `/ready` (Readiness)
- **Method**: `GET`
- **Response**:
  - `200 OK` when the chain state is **Warm** (`ChainWarm`). This means the initial fleet sweep is complete, instance IP addresses are discovered, and every target has been rendered.
  - `503 Service Unavailable` when the chain state is **Cold** (`ChainCold`) during initial startup synchronization or network disconnections.

### `/debug/pprof/` (Profiling)
- Available when `--pprof` is set to `true`.
- Exposes standard Go `net/http/pprof` endpoints for goroutine inspection, CPU profiling, and heap allocation analysis.

---

## Chain Tuning & Optimization

- **`--debounce-window` (default `250ms`)**:
  When scaling services or restarting clusters, dozens of events may arrive in rapid succession. Debouncing buffers burst events within the quiet window, performing a single render, validation, and reload per target.
- **`--workers` (default `16`)**:
  Controls how many instance states are fetched in parallel from Incus during sweeps. On large clusters (100+ instances), increasing workers reduces the initial sweep duration.
- **`--exclude`**:
  Allows omitting optional pipeline stages:
  - `--exclude http`: Disables the HTTP `/health` and `/ready` server entirely.
  - `--exclude debounce`: Disables event debouncing for immediate zero-latency event processing.
