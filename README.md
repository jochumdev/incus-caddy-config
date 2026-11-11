# incus-templated

[![CI](https://github.com/lxc/incus-templated/actions/workflows/test.yml/badge.svg?branch=main)](https://github.com/lxc/incus-templated/actions)
[![Coverage 90%](https://img.shields.io/badge/coverage-91%25-brightgreen)](https://github.com/lxc/incus-templated/actions)
[![License Apache 2.0](https://img.shields.io/badge/license-Apache%202.0-blue.svg)](LICENSE)

Render any Go template from [Incus](https://linuxcontainers.org/incus/) instance labels, and deploy it with optional format, validate and reload steps, driven by instance events via `ievent`. A [Caddy](https://caddyserver.com/) reverse proxy preset ships built in; Prometheus `file_sd` targets, HAProxy backends, or DNS zones are a template away.

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

  api:
    image: docker.io/library/busybox:latest
    command: httpd -f -p 8080
    labels:
      edge: "api.example.com,upstream=8080"

volumes:
  caddy-config:
  caddy-data:
```

```bash
incus-compose up -d
```

With `--target edge,preset=caddy,instance=caddy,project=default`, `incus-templated` watches your Incus fleet, discovers running instances labeled `edge`, and configures reverse proxy routes into Caddy with zero manual configuration.

---

## How It Works

`incus-templated` communicates with Incus exclusively over the Incus API (events, storage volume SFTP, and container exec). With the Caddy preset:

```mermaid
flowchart TD
    IncusDaemon["Incus Daemon (API & Events)"] -->|instance lifecycle events| Chain["ievent Chain<br/>(source &rarr; debounce &rarr; enricher)"]
    Chain --> Plugin["incus-templated Controller"]

    subgraph Storage["Incus Storage Pool"]
        Vol[("custom/caddy-config volume<br/>mounted to /config")]
    end

    subgraph Caddy["Serving Caddy Container"]
        CaddyProc["caddy:2.11.4-alpine<br/>(admin localhost:2019)"]
    end

    Plugin -->|"1. Write staged config via SFTP"| Vol
    Plugin -->|"2. Exec 'caddy fmt'"| CaddyProc
    Plugin -->|"3. Atomic rename (Caddyfile.tmp &rarr; Caddyfile)"| Vol
    Plugin -->|"4. Exec 'caddy reload'"| CaddyProc

    Vol -.->|mounted at /config| CaddyProc
```

1. **Monitors Fleet**: Subscribes to the Incus event stream and tracks instances carrying target labels.
2. **Direct Storage Volume SFTP**: Connects directly to the underlying Incus storage volume holding the target path over SFTP (`conn.GetStoragePoolVolumeFileSFTP`).
3. **Safe Validation & Formatting**: Renders the template in-memory, stages to `<path>.tmp`, and runs the target's `format` and `validate` commands (`caddy fmt`, `caddy validate`).
4. **Atomic Swapping & Reload**: Atomically replaces the file using `sftp.PosixRename` and runs the target's `reload` command (`caddy reload`) via container exec.
5. **Reboot & Crash Survival**: If Caddy restarts or the host reboots, Caddy immediately starts up using the persisted configuration on the storage volume. If Caddy is stopped during reconcile, `incus-templated` stages directly onto the volume so it boots with the updated config on next start.

---

## Features

- **Any Template, Any Consumer**: Each target renders one Go template from the instances carrying its label: name, project, cluster location, labels, config, devices and addresses. Format, validate and reload are each optional commands, so a file watched by its consumer (Prometheus `file_sd`) needs none.
- **Dual Deployment Modes**: Deploys into an Incus instance over SFTP (`--target <label>,instance=<inst>,project=<proj>`), or to the local filesystem (`--target <label>,path=<path>`). Targets are repeatable, comma/space-separated, or listed in a YAML `--config` file.
- **Zero Admin Port Exposure**: With the Caddy preset, Caddy's Admin API listens strictly on container loopback (`localhost:2019`). No admin port is published to the host or exposed to the network.
- **Direct Storage Volume SFTP**: Writes directly to the underlying storage volume over the Incus API. `incus-templated` needs no local volume mount or host filesystem access.
- **Single Goroutine Concurrency**: Concurrency model strictly confined to a single goroutine (Rule A4). Zero mutexes, race-free event processing, and orderly reconciliation.
- **Warm Gating**: Reconciliations are suppressed while the event chain is cold (`ChainCold`). Deployments only trigger after the initial fleet sweep completes (`ChainWarm`), eliminating route churn during daemon reconnects.
- **Automatic Load Balancing**: With the Caddy preset, multiple instances sharing the same domain label are automatically aggregated and sorted into a single load-balanced `reverse_proxy` directive.
- **Custom Vhost Templating**: The Caddy preset supports custom site blocks per vhost from `--templates-dir` or inline, and a whole custom Caddyfile template per target.
- **Observability**: Built-in HTTP endpoints on `:9153` for liveness (`/health`), fleet readiness (`/ready`), and Go runtime profiling (`/debug/pprof`).

---

## Quick Start

### 1. Generate an Incus Trust Token

```bash
incus config trust add incus-templated
```

Save the token into `.env` (loaded automatically by `incus-compose` without `--os-env`):

```bash
echo "INCUS_TOKEN=<token>" > .env
```

### 2. Run with `incus-compose`

Create `compose.yaml`:

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
      INCUS_TEMPLATED_TARGETS: "edge,preset=caddy,instance=caddy-1,project=default"
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

Start the stack:

```bash
incus-compose up -d
```

### 3. Route Your First Service

Add labels to any container or virtual machine:

```yaml
services:
  web:
    image: docker.io/library/nginx:alpine
    labels:
      edge: "web.example.test,upstream=80"
```

Deploy the service:

```bash
incus-compose up -d web
```

Caddy immediately discovers the instance and routes traffic:

```bash
curl -H "Host: web.example.test" http://127.0.0.1/
```

---

## Caddy Labels Reference

For a Caddy target bound to prefix `edge` (`--target edge,preset=caddy,instance=caddy,project=default` or `--target edge,preset=caddy,path=/etc/caddy/Caddyfile`):

| Label in `compose.yaml` | Description                                                                                                                           |
| ----------------------- | ------------------------------------------------------------------------------------------------------------------------------------- |
| `edge`                  | **(Required)** Domain name(s) with optional flags (e.g. `,upstream=8080`, `,network=incusbr0`, `,redir=https://new.com`, `,template=custom.caddyfile`). Multiple domains separated by spaces. |
| `edge.redirs`           | Alternate redirection domain(s) mapping to primary domain (e.g. `www.example.com,no-uri`).                                            |
| `edge.service`          | Custom service name override (defaults to `user.incus-compose.service`).                                                              |

---

## Documentation

Full documentation is available at [caddy.incus-compose.org](https://caddy.incus-compose.org):

- **[Getting Started](https://caddy.incus-compose.org/getting-started)** - Installation, prerequisites, and first deployment
- **[Architecture & Security](https://caddy.incus-compose.org/architecture)** - Process isolation, volume resolution, and concurrency model
- **[Caddy Labels & Routing](https://caddy.incus-compose.org/labels)** - Complete label specification and load balancing
- **[Templates](https://caddy.incus-compose.org/templates)** - Template data, funcs, Prometheus and cluster examples, and the Caddy preset
- **[Configuration Reference](https://caddy.incus-compose.org/configuration)** - Targets, presets, the YAML config file, canonical `INCUS_TEMPLATED_*` environment variables, and HTTP endpoints
- **[Deployment & Operations](https://caddy.incus-compose.org/deployment)** - Production compose stacks, reboot survival, and troubleshooting

---

## Support & Community

- **Bug Reports & Issues**: [github.com/lxc/incus-templated/issues](https://github.com/lxc/incus-templated/issues)
- **Community Forum**: [discuss.linuxcontainers.org](https://discuss.linuxcontainers.org)

---

## Contributing

Contributions are welcome! Please read [CONTRIBUTING.md](CONTRIBUTING.md) and [AGENTS.md](AGENTS.md) before submitting pull requests.

---

## License

[Apache 2.0](LICENSE)
