# PRD: DeployPanel

Status: Draft v0.1 · Owner: Kashvit · Date: 2026-11-10

## 1. Problem

Self-hosters and small hosting operators run two disconnected toolchains:

- **PaaS tools** (CapRover, Coolify, Dokploy, Dokku) for containerized apps. Good deploy UX, weak on traditional PHP/WordPress hosting, email, DNS and multi-tenant quotas.
- **Hosting panels** (CloudPanel, CyberPanel) for sites, per-site users, email and resellers. Weak on containers, Git deploys and observability.

Gaps shared by all of them: thin observability (logs, stats, crash context), a large panel attack surface (public admin ports, SSH-key custody, docker.sock exposure), and backups that are never restore-tested.

## 2. Vision

One lightweight panel where every workload is a **Site**: a container app, Compose stack, PHP site, WordPress, Node/Python app, static site or reverse proxy. All Sites share one deploy, SSL, logs, backup and access-control path.

## 3. Goals and non-goals

### Goals
- G1. Idle footprint of the control plane under 150 MB RAM (single Go binary, SQLite default).
- G2. Best-in-class built-in observability: live logs, ctop-style stats, search, alerts, crash snapshots.
- G3. Secure by default: no public admin port, outbound-only agents, RBAC, 2FA, audit log, encrypted secrets.
- G4. Restore-tested backups (S3 for volumes and DB dumps, panel-state backup, "test restore").
- G5. Migration paths: importers for CapRover, Coolify, CloudPanel and CyberPanel.
- G6. API-first: UI, CLI, webhooks and Terraform provider use the same API.

### Non-goals (v1)
- Building a mail server (integrate an existing one).
- Kubernetes as a first-class runtime (K3s driver is post-v1, demand permitting).
- Hosted SaaS offering.
- OpenLiteSpeed / LSCache support.
- Billing (a FOSSBilling hook is optional and post-v1).

## 4. Personas

| ID | Persona | Needs | Phase |
|---|---|---|---|
| P1 | Solo dev / small team shipping container apps | Git deploy, Compose, SSL, logs, backups | Primary, v0.1 |
| P2 | Agency / freelancer hosting client WordPress and PHP sites | Per-site users, PHP versions, staging, SFTP, backups | v0.2 |
| P3 | Small hosting operator / reseller | Packages, quotas, resellers, email, DNS | v0.4+ |

Decision needed (see section 9): P1 is the recommended primary persona so v0.1 ships as the best single-server PaaS before the hosting layer.

## 5. Competitive feature map

| Capability | CapRover | Coolify | Dokploy | Dokku | CloudPanel | CyberPanel | DeployPanel |
|---|---|---|---|---|---|---|---|
| Docker apps | Yes | Yes | Yes | Yes | No | Limited | Yes |
| Compose | Limited | Yes | Yes | No | No | No | Yes |
| git push deploy | No | No | No | Yes | No | No | Yes |
| Buildpacks | No | Yes | Yes | Yes | No | No | Yes |
| One-click templates | Yes | Yes | Yes | Plugins | Yes | Yes | Yes (unified spec) |
| Multi-server | Swarm | SSH | Swarm/SSH | No | No | No | Agents |
| Teams / RBAC | No | Yes | Yes | No | Yes | Yes | Yes |
| PHP sites, per-site users | No | No | No | No | Yes | Yes | Yes |
| WordPress manager | No | No | No | No | Partial | Yes | Yes |
| Email hosting | No | No | No | No | No | Yes | Module |
| DNS | No | No | No | No | No | Yes | Module |
| Resellers / packages | No | No | No | No | No | Yes | v0.4 |
| Built-in monitoring | Basic | Sentinel | Basic | No | Basic | Basic | Core feature |
| S3 backups | No | Yes | Yes | Plugin | Yes | Yes | Yes + restore test |

Competitor facts come from two third-party articles and should be re-verified against project docs before any public comparison.

## 6. Functional requirements

Priority: M = must for the milestone, S = should, C = could. Milestones match ROADMAP.md.

### Sites (all types)
| ID | Requirement | Pri | Milestone |
|---|---|---|---|
| F-S1 | Create, start, stop, restart, delete a Site of any supported type | M | v0.1 |
| F-S2 | Site has one or more domains, automatic SSL via ACME | M | v0.1 |
| F-S3 | Environment variables and encrypted secrets per Site | M | v0.1 |
| F-S4 | Deployment history, rollback to previous release | M | v0.1 |
| F-S5 | Per-site resource limits (CPU, RAM, disk, PIDs) | M | v0.2 |
| F-S6 | Per-site cron jobs | S | v0.2 |
| F-S7 | Persistent volumes per Site | M | v0.1 |

### Deploy sources
| ID | Requirement | Pri | Milestone |
|---|---|---|---|
| F-D1 | Deploy from Dockerfile, Compose file, prebuilt image | M | v0.1 |
| F-D2 | Deploy from Git (GitHub, GitLab, Bitbucket, Gitea) with webhooks | M | v0.2 |
| F-D3 | `git push` remote with buildpacks | S | v0.2 |
| F-D4 | CLI deploy (`dpctl deploy`) | M | v0.1 |
| F-D5 | `captain-definition` importer | S | v1.0 |

### Hosting layer
| ID | Requirement | Pri | Milestone |
|---|---|---|---|
| F-H1 | PHP site type: PHP version per site, per-site Linux user, jailed SFTP | M | v0.2 |
| F-H2 | WordPress: one-click install, staging clone, auto-update policy, object and page cache | M | v0.2 |
| F-H3 | Managed databases (MariaDB, PostgreSQL, Redis) with per-site users and phpMyAdmin/Adminer | M | v0.2 |
| F-H4 | File manager and vhost/proxy config editor with validation | S | v0.3 |
| F-H5 | Packages with quotas, resellers and customer roles | M | v0.4 |
| F-H6 | DNS module (PowerDNS, Cloudflare and Route 53 sync) | S | v0.4 |
| F-H7 | Mail module (integrate Stalwart or similar: domains, mailboxes, DKIM, SPF, DMARC) | S | v1.0 |

### Platform
| ID | Requirement | Pri | Milestone |
|---|---|---|---|
| F-P1 | Multi-server via outbound mTLS agents, no SSH key custody | M | v0.3 |
| F-P2 | Teams, RBAC, 2FA (TOTP, WebAuthn), API tokens | M | v0.3 |
| F-P3 | Audit log of every mutating action | M | v0.3 |
| F-P4 | Template catalog with a unified spec; import CapRover and Coolify catalogs | S | v0.2 to v1.0 |
| F-P5 | Swarm runtime driver | C | v1.0 |
| F-P6 | Importers: CapRover, Coolify, CloudPanel, CyberPanel | S | v1.0 |

### Observability and operations
| ID | Requirement | Pri | Milestone |
|---|---|---|---|
| F-O1 | Live log streaming and search per Site | M | v0.1 |
| F-O2 | Live CPU, RAM, network, disk stats per container (ctop-style) | M | v0.1 |
| F-O3 | Crash snapshots: last N log lines and stats captured at container exit | M | v0.1 |
| F-O4 | Persistent log and metric retention in SQLite with configurable TTL | M | v0.1 |
| F-O5 | Alerts (crash, OOM, disk, cert expiry, backup failure) to webhook, email, Slack | S | v0.3 |
| F-B1 | Scheduled backups: volumes, DB dumps, panel state, to S3 or local | M | v0.2 |
| F-B2 | "Test restore" into a scratch Site and report pass/fail | S | v1.0 |

### Security
| ID | Requirement | Pri | Milestone |
|---|---|---|---|
| F-X1 | Admin UI served only over HTTPS on the panel domain; no raw admin port on the public interface | M | v0.1 |
| F-X2 | Handle Docker's published-port bypass of host firewalls (explicit firewall manager) | M | v0.2 |
| F-X3 | WAF module (Coraza with OWASP CRS) and CrowdSec integration | S | v0.3 |
| F-X4 | Signed release artifacts and SBOM | M | v0.1 |
| F-X5 | Tenant isolation for PHP and WordPress: per-site user, cgroups, read-only rootfs, no shared docker.sock | M | v0.2 |

## 7. Non-functional requirements

| ID | Requirement |
|---|---|
| N1 | Control plane idle memory under 150 MB with SQLite, node agent under 40 MB |
| N2 | Install on a fresh Ubuntu/Debian host with one command in under 5 minutes |
| N3 | Panel upgrades are zero-downtime for hosted Sites |
| N4 | Panel outage must not take hosted Sites down (agents and proxy keep running) |
| N5 | All state migrations are forward-only and tested on a snapshot |
| N6 | Supported: Linux amd64 and arm64, Docker Engine 25+ |

## 8. Success metrics

- Time from fresh VPS to first deployed app under 10 minutes.
- Idle control-plane RSS under 150 MB (measured, published method).
- 100% of scheduled backups verified by at least a checksum, and a restore test on a schedule for opted-in Sites.
- Zero public admin ports in default install (checked by installer self-test).
- Importer success rate on a corpus of real CapRover and Coolify exports (target 90% of apps imported without manual edits).

## 9. Open questions

1. **Primary persona for v0.1:** P1 (PaaS) is recommended. Confirm.
2. **License:** Apache-2.0 or AGPL-3.0? Affects ecosystem and any future commercial edition. (The name is decided: DeployPanel, always written in full. Domain, GitHub org and trademark checks are still pending.)
3. **Mail:** integrate which server (Stalwart, Mailcow, Mailu)? Defer to v1.0 or drop?
4. **Runtime for PHP:** one PHP-FPM container per site (recommended) vs native pools.
5. **Control plane database:** SQLite only, or SQLite plus optional Postgres for larger fleets?
6. **Existing monitor tool:** fold the Go/SQLite log and stats monitor into the node agent's collector?

## 10. Risks

| Risk | Impact | Mitigation |
|---|---|---|
| Scope creep across six products | Never ships | v0.1 is single-node PaaS only; modules are optional and off by default |
| Multi-tenant PHP isolation | Cross-tenant compromise | Container-per-site, cgroups, read-only rootfs, user namespaces, security review before v0.2 GA |
| Panel as high-value target (docker.sock, root agent) | Fleet compromise | Least-privilege agent, mTLS, signed releases, no inbound ports on nodes |
| Template and importer maintenance | Ongoing cost | Import upstream catalogs, community-owned templates |
| Mail deliverability | Support burden | Integrate, do not build; clear docs on IP reputation |
| Multi-node networking | Support burden | Start with host-routed ingress per node; overlay networking only in the Swarm driver |
