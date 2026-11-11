---
description: Internal design, security boundaries, single-goroutine confinement, and storage volume SFTP deployer.
editor: markdown
published: true
title: Architecture & Security
leafwiki_id: cuWjDPlvR
leafwiki_title: Architecture & Security
leafwiki_created_at: "2026-09-15T22:05:21Z"
leafwiki_updated_at: "2026-09-15T22:05:21Z"
leafwiki_creator_id: system
leafwiki_last_author_id: system
---

# Architecture & Security

`incus-templated` is designed around strict process isolation, single-goroutine state confinement, and direct storage volume manipulation over the Incus API.

---

## Two-Process Security Model

`incus-templated` separates control-plane responsibilities from the data plane across two isolated processes:

```mermaid
flowchart TD
    subgraph ControlPlane["Control Plane (Private)"]
        CC["incus-templated Controller"]
        Credentials[("Incus Client Certificate<br/>(/var/lib/incus-templated)")]
        CC --- Credentials
    end

    subgraph DataPlane["Data Plane (Public)"]
        Caddy["Caddy Reverse Proxy"]
        Ports["Public Ports 80 / 443"]
        Ports --- Caddy
    end

    IncusDaemon["Incus Daemon API<br/>(HTTPS / Unix Socket)"]

    CC -->|"1. Subscribes to events<br/>2. Discovers instances"| IncusDaemon
    CC -->|"3. SFTP directly to storage volume"| StorageVolume[("Incus Storage Volume<br/>(incus-templated)")]
    CC -->|"4. Exec 'caddy fmt' & 'caddy reload'"| Caddy

    StorageVolume -.->|mounted at /config| Caddy
```

### Process Separation Boundaries

| Property | `incus-templated` | `caddy` |
|---|---|---|
| **Role** | Control-plane event listener & config reconciler | Data-plane HTTP/HTTPS reverse proxy |
| **Network Exposure** | None (internal `:9153` healthcheck only) | Public ports `80` and `443` |
| **Incus Credentials** | Holds client TLS certificate or trust token | **None** (zero access to Incus API) |
| **Caddy Admin API** | Accesses via in-container exec (`caddy reload`) | Bound to `localhost:2019` inside container |
| **Disk Access** | Writes to storage volume via Incus SFTP | Mounts `/config` and `/data` storage volumes |

Because Caddy holds no Incus credentials and its Admin API is bound strictly to `localhost:2019`, a compromised backend service or proxy vulnerability cannot access the Incus API or manipulate container definitions.

---

## Storage Volume Resolution & Direct SFTP Deployer

Caddy persists its configuration in an Incus custom storage volume (e.g. `incus-templated` mounted to `/config`).

Rather than requiring `incus-templated` to mount the storage volume locally on the host or in its own container, `incus-templated` communicates entirely through the Incus API:

```mermaid
sequenceDiagram
    participant P as incus-templated Plugin
    participant API as Incus API
    participant Vol as Incus Storage Volume
    participant C as Caddy Container

    P->>API: GetInstance(project, instance)
    API-->>P: Instance device configuration
    Note over P: resolveVolume inspects disk devices<br/>attached to /config

    alt Custom Storage Volume Attached
        P->>API: GetStoragePoolVolumeFileSFTP(pool, "custom", name)
        API-->>P: sftp.Client (volume root)
    else No Volume Device
        P->>API: GetInstanceFileSFTP(project, instance)
        API-->>P: sftp.Client (container root)
    end

    P->>Vol: Create & write /Caddyfile.tmp
    
    alt Container is Running
        P->>API: ExecInstance("caddy fmt --overwrite /Caddyfile.tmp")
        API->>C: caddy fmt --overwrite
        C-->>API: exit code 0
        API-->>P: Formatting and validation succeeded
        P->>Vol: sftp.PosixRename("/Caddyfile.tmp", "/Caddyfile")
        P->>API: ExecInstance("caddy reload --config /config/Caddyfile")
        API->>C: caddy reload
    else Container is Stopped (Cold Staging)
        P->>Vol: sftp.PosixRename("/Caddyfile.tmp", "/Caddyfile")
        Note over P,Vol: Staged directly onto volume.<br/>Caddy boots with valid config on start.
    end
```

### Key Deployment Invariants

1. **Volume Resolution (`resolveVolume`)**:
   `deploy()` inspects the instance's expanded devices to locate a disk device whose mount path matches the parent directory of the target `path` (default `/config` with the Caddy preset). It extracts the `pool` and volume `source` name.
2. **Cold Staging (Reboot & Crash Survival)**:
   If Caddy is stopped, `incus-templated` still updates the Caddyfile on the storage volume. When Caddy starts up or reboots, it reads the updated configuration immediately.
3. **Atomic Swapping**:
   `incus-templated` never writes directly over the active `Caddyfile`. It writes to `<path>.tmp` and only renames via `sftp.PosixRename` after the target's `format` and `validate` commands succeed in-container.
4. **Validation Isolation**:
   If an invalid route or template syntax error occurs, rendering or a check command fails. The active file remains completely untouched.

### Local Deployment (`--target <label>,path=<path>`)

When `incus-templated` runs alongside Caddy on the same host, container, or VM:
- **Filesystem Staging**: Writes directly to `.<base>.tmp` in the target directory and atomically swaps via `os.Rename`.
- **Local Commands**: Runs the target's `format`, `validate` and `reload` on the local machine. A failed reload is retried; the file is already in place for the next start.
- **Unified Event Pipeline**: Reconciles alongside instance targets within the exact same single-goroutine loop.

---

## Concurrency Model: Single Goroutine Confinement (Rule A4)

All state in `templated.Plugin` (`instances`, `lastDeployed`, `chain`) is strictly confined to `p.Run(ctx)`'s single goroutine:

```mermaid
flowchart LR
    subgraph ieventChain["ievent Chain"]
        Next["next(ev)"]
    end

    Event[Incoming Event] --> Handle["p.Handle(ev)"]
    Handle --> Next
    Handle -->|non-blocking send| Inbox[("p.inbox (chan *Event)")]
    
    subgraph RunLoop["p.Run(ctx) Single Goroutine"]
        Inbox --> RunSelect{select}
        CommandIn[p.commandIn] --> RunSelect
        Ctx[ctx.Done] --> RunSelect
        
        RunSelect -->|event| Process["processEvent()"]
        Process --> Reconcile["reconcile()"]
    end
```

- **No Mutexes**: `Plugin` contains zero mutexes (`sync.Mutex` or `sync.RWMutex`) and zero atomic flags.
- **Sequential Event Processing**: Events delivered by `Handle()` are placed into a buffered inbox channel (`p.inbox`) and forwarded along the `ievent` chain immediately.
- **Ordered Reconciles**: `processEvent()` and `reconcile()` execute exclusively on the `Run()` goroutine. Race conditions between concurrent state updates and Caddy reloads are impossible by design.
- **Graceful Drain**: When the chain receives a shutdown command (`iutil.CommandDrain`), `p.Run` drains remaining events in `p.inbox` via `drainInbox()`, forwards the drain command to `p.commandOut`, and exits cleanly.

---

## Warm Gating on Chain State

`incus-templated` coordinates with the `ievent` enricher to avoid flapping routes during startup or daemon reconnection:

- **Cold State (`iutil.ChainCold`)**:
  When the process starts, the enricher performs an initial fleet sweep of all monitored projects. During this sweep, instance events are recorded into `p.instances`, but **all Caddy deployments are gated and paused**.
- **Warm State (`iutil.ChainWarm`)**:
  Once the sweep finishes, the enricher emits `ActionSweepEnd`. `incus-templated` transitions to `ChainWarm` and performs a single initial `reconcile()` deploying the complete routing table.
- **Disconnection Handling**:
  If the Incus event stream disconnects, the state drops to `ChainCold`. Caddy retains its last known good configuration on the storage volume while `incus-templated` reconnects and re-synchronizes the fleet.
