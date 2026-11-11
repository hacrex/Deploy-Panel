# Architecture

Status: Draft v0.1. See also SITE_DATA_MODEL.md, MODULES.md, adr/.

## 1. Overview

```
 Web UI / dpctl CLI / git push / REST + webhooks
                    |
        +-----------v------------+
        |  Control plane (dpd) |   single Go binary
        |  API | scheduler | auth|   SQLite (default) or Postgres
        |  module host           |
        +-----------+------------+
                    |  gRPC over mTLS (agent dials OUT)
     +--------------+---------------+
     |                              |
+----v------------------+   +-------v-----------------+
| Node agent (dp-agent)|  | Node agent              |
| Runtime driver (Docker) |  | ...                     |
| Proxy driver (Caddy)    |  +-------------------------+
| Collector (logs, stats) |
| Optional module sidecars|
|  mail | dns | waf | cache
+-------------------------+
```

The control plane stores desired state. Agents reconcile each node toward that state and stream logs, stats and events back. If the control plane is down, agents, proxy and Sites keep running (N4 in the PRD).

## 2. Components

### 2.1 Control plane (`cmd/dpd`)
- HTTP API (REST + SSE for streams), embedded web UI assets.
- Auth: sessions, API tokens, TOTP/WebAuthn, RBAC.
- Scheduler: picks a Node for each Site, issues deploy and backup jobs.
- Reconciler: compares desired vs reported state per Node.
- Module host: loads optional modules (section 5).
- Storage: SQLite WAL by default. Postgres is a planned alternative using the same migrations (the runner needs placeholder rebinding first).
- Configuration: `DP_*` environment variables, see CONFIG.md. The admin listener must be loopback unless an operator opts in.

### 2.2 Node agent (`cmd/dp-agent`)
- Connects outbound to the control plane over mTLS. Nodes expose no panel port.
- Enrolls once with a short-lived token, receives a client certificate, then rotates it.
- Applies Site specs through a **RuntimeDriver** (Docker first) and a **ProxyDriver** (Caddy first).
- Collector: tails container logs, samples stats, writes crash snapshots, ships batches to the control plane with a local ring buffer so short disconnects lose nothing.
- Needs docker.sock. It is the only component with that access (see section 7).

### 2.3 CLI (`cmd/dpctl`)
Thin client over the REST API: `login`, `sites`, `deploy`, `logs`, `exec`, `backup`, `import`.

### 2.4 Web UI
Static SPA served by the control plane. Talks only to the REST API. Framework choice is open (ADR-0006 proposed).

## 3. Site lifecycle

```
 draft -> provisioning -> running <-> stopped
              |             |  \
              v             v   v
            failed       degraded  suspended (admin or quota)
                            |
                            v
                         deleting -> deleted (soft, retained for restore window)
```

A Site holds a **desired state** (spec, revision) and an **observed state** (reported by the agent). The reconciler closes the gap.

### Deploy flow
1. Source resolves to a build input: Git ref, uploaded archive, Compose file, image, or `git push`.
2. Control plane creates a `Deployment` (revision N) in state `queued`.
3. Scheduler assigns a build Node (usually the target Node).
4. Agent builds (buildpack, Dockerfile) or pulls the image, tags it `site/<id>:<rev>`.
5. Agent starts the new revision next to the old one, waits for the health check, then flips the proxy route.
6. Old revision is stopped after a drain period and kept for rollback. Failure at any step keeps the old revision serving.

## 4. Runtime model

| Site type | Runtime unit | Notes |
|---|---|---|
| `container` | One container from an image or build | Default for P1 |
| `compose` | A Compose project | Agent drives `compose` semantics; Site owns the project |
| `php` | One PHP-FPM container per Site, shared Caddy | Version per Site, per-Site uid, jailed SFTP |
| `wordpress` | `php` Site plus managed DB, cache and updater | Staging clone is a second Site linked by `parent_site_id` |
| `node` / `python` | Container built by buildpack | Same as `container` with a type-specific preset |
| `static` | Served by Caddy from a Site volume | No app container |
| `proxy` | Caddy route to an upstream | Upstream may be another Site or an external URL |

All types expose the same operations (deploy, start, stop, logs, stats, backup), so features are written once.

### Isolation for hosted PHP/WordPress
- One container and one Linux uid per Site, user namespaces enabled.
- cgroup v2 limits from the Site's Package (CPU, memory, pids, IO).
- Read-only root filesystem, writable only on the Site volume and tmpfs.
- Dropped capabilities, seccomp default profile, no docker.sock, no host mounts.
- Per-Site network: Sites cannot reach each other unless an explicit `link` exists.
- SFTP via a separate jailed service chrooted to the Site volume.

## 5. Module boundaries

Core is small. Everything that is not needed for "deploy and run a Site" is a module with an interface, own tables prefixed by module name, and an off-by-default sidecar if it needs a process.

| Module | Owns | Depends on | Default |
|---|---|---|---|
| `core/site` | Sites, domains, deployments, env | none | on |
| `core/node` | Nodes, enrollment, agent protocol | none | on |
| `core/iam` | Users, teams, roles, tokens, audit | none | on |
| `core/observe` | Logs, stats, crash snapshots, alerts | node | on |
| `core/backup` | Policies, targets, backups, restore tests | site, node | on |
| `core/db` | Managed DB instances and users | site, node | on |
| `mod/wordpress` | WP install, staging, updates, cache | site, db | off until used |
| `mod/dns` | Zones, records, provider sync | site | off |
| `mod/mail` | Mail domains, mailboxes, DKIM | dns | off |
| `mod/waf` | Coraza rules, CrowdSec bridge | site | off |
| `mod/resell` | Packages, quotas, reseller hierarchy | iam, site | off |
| `mod/import` | CapRover, Coolify, CloudPanel, CyberPanel importers | site | off |

Rules:
1. Modules talk to core only through Go interfaces in `internal/core/*` and the event bus. Core never imports `mod/*`.
2. A module may not read another module's tables. It calls that module's interface.
3. Each module registers: migrations, API routes, event handlers, agent job types, UI manifest.
4. A module's agent work runs as a sidecar container managed through the normal runtime driver, not as code in the agent binary.

See MODULES.md for interfaces.

## 6. Agent protocol (summary)

gRPC over mTLS, agent is the client. See `proto/deploypanel/agent/v1/agent.proto`.
- `Enroll`: one-time token, returns client cert.
- `Connect`: bidirectional stream. Control plane sends `Job` messages (apply Site spec, run backup, exec); agent sends `Event`, `LogBatch`, `StatsBatch`, `JobResult`.
- Jobs are idempotent and carry a revision number. Agent acks by revision and reports observed state.
- Heartbeat every 15 s. A Node is `offline` after 60 s without one.

## 7. Security model

Trust boundaries: Internet, panel admin, tenant Site, agent host, control plane.

| Threat | Control |
|---|---|
| Public admin port exposed | Panel UI only via its own domain behind Caddy with TLS. Installer refuses to bind 3000-style ports publicly. |
| Control plane holds SSH keys to every server | Not used. Agents connect out. Compromise of the control plane can still issue jobs, so jobs are signed and agents enforce an allowlist of job types. |
| Docker bypasses host firewall | Firewall manager writes rules to `DOCKER-USER`; published ports default to localhost unless routed through the proxy. |
| docker.sock abuse | Only the agent mounts it. Tenant containers never do. Agent runs as a dedicated user in the `docker` group with a restricted job set. |
| Tenant breakout or cross-tenant access | See section 4 isolation list. |
| Secret exposure | Secrets encrypted at rest with a key from a KMS file or env, never logged, redacted in audit. |
| Supply chain | Signed releases, SBOM, pinned base images, reproducible builds where feasible. |
| Account takeover | 2FA, session binding, API token scopes, audit log, login rate limits. |

## 8. Observability

- Agent tails container logs through the Docker API and stores to the control plane in batches.
- Control plane keeps logs and metrics in SQLite tables with TTL pruning. Large fleets can switch to external storage via an interface (`LogStore`, `MetricStore`).
- Crash snapshot: on container `die` with non-zero exit or OOM, the agent stores the last N log lines, final stats samples, exit code, OOM flag and restart count as an `incident`.
- Alerts evaluate rules over events and metrics and fan out to webhooks, email and chat.

## 9. Backup and restore

- Backup unit per Site: volumes, DB dumps, Site spec. Panel state backup is separate.
- Targets: local path, S3-compatible. Encrypted client-side (age or restic).
- Restore test: restore the latest backup into a scratch Site on a chosen Node, run the Site's health check, report, and delete the scratch Site.

## 10. Networking and proxy

- Each Node runs Caddy managed by the agent through its admin API. ACME for all Site domains.
- Wildcard domains per Node supported with DNS-01 via DNS module or provider plugins.
- Cloudflare in front is supported by design (the CapRover limitation does not apply because routes are explicit).
- Cross-node routing: v0.3 routes by DNS to the owning Node. An overlay network arrives only with the Swarm driver.

## 11. Technology choices

| Area | Choice | Why |
|---|---|---|
| Language | Go | Single binary, low RAM, strong Docker ecosystem. `dpd` needs CGO for SQLite today; `dp-agent` and `dpctl` are static |
| Store | SQLite (WAL), optional Postgres | Meets footprint goal; Postgres for scale |
| Proxy | Caddy | Automatic TLS, dynamic JSON API |
| Agent transport | gRPC + mTLS | Streams, typed contract, outbound-only |
| Builds | Nixpacks or Cloud Native Buildpacks, Dockerfile | Heroku-style deploys and git push |
| WAF | Coraza | Embeddable, OWASP CRS |
| Backups | restic or age + S3 | Encrypted, dedup, widely trusted |
| Mail (module) | Integrate Stalwart or similar | Do not build deliverability |

## 12. Repository layout

```
cmd/dpd           control plane entrypoint
cmd/dp-agent      node agent entrypoint
cmd/dpctl         CLI
internal/config   DP_* environment configuration and validation
internal/store    SQLite open, connection settings, migration runner
internal/core/    site, node, iam, observe, backup, db (interfaces + services)
internal/module/  module host and registry
internal/boundary tests enforcing core and module import rules
internal/runtime/ RuntimeDriver interface + docker driver
internal/proxy/   ProxyDriver interface + caddy driver
mod/              optional modules (wordpress, dns, mail, waf, resell, import)
migrations/       forward-only SQL, embedded into dpd
proto/            agent contract (proto/deploypanel/agent/v1)
api/              OpenAPI
deploy/           Dockerfiles, dev compose, systemd units, installer, env example
scripts/          schema and API consistency checks
web/              UI (placeholder)
docs/             PRD, architecture, data model, modules, config, roadmap, brand, ADRs
```
