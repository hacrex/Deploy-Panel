# Module boundaries

## 1. What a module is

A module is a Go package under `mod/<name>` that implements `module.Module` and registers: migrations, API routes, event handlers, agent job types and a UI manifest. Heavy work runs in a sidecar container started through the runtime driver, never inside the agent binary.

```go
type Module interface {
    Name() string
    Version() string
    Requires() []string          // other module names, core modules always available
    Migrations() fs.FS           // embedded SQL, prefixed tables
    Init(ctx context.Context, h Host) error
    Close() error
}
```

`Host` gives a module controlled access to core: Sites, Nodes, IAM checks, Events, Jobs, Secrets, Logger.

## 2. Dependency rules

1. `core/*` never imports `mod/*`.
2. Modules import `internal/core/*` interfaces and the event types only.
3. A module may not query another module's tables. Use that module's Go interface.
4. Cross-module calls go through the `Host.Module(name)` lookup, which returns an interface or an error if the module is disabled.
5. Disabling a module leaves its data in place and stops its routes, jobs and sidecars.

## 3. Core services

| Service | Package | Responsibility |
|---|---|---|
| SiteService | `internal/core/site` | CRUD, state machine, spec validation, domains, env |
| DeployService | `internal/core/site` | Deployments, rollback, build job dispatch |
| NodeService | `internal/core/node` | Enrollment, health, placement |
| IAMService | `internal/core/iam` | Authn, RBAC, tokens, audit |
| ObserveService | `internal/core/observe` | Logs, metrics, incidents, alerts |
| BackupService | `internal/core/backup` | Policies, targets, runs, restore tests |
| DBService | `internal/core/db` | Managed DB instances and users |
| TemplateService | `internal/core/site` | Templates to Site specs |

## 4. Driver interfaces (replaceable implementations)

```go
// internal/runtime
type RuntimeDriver interface {
    Apply(ctx context.Context, spec SiteRuntimeSpec) (ApplyResult, error)
    Remove(ctx context.Context, siteID string) error
    Logs(ctx context.Context, siteID string, opts LogOptions) (io.ReadCloser, error)
    Stats(ctx context.Context, siteID string) (<-chan StatsSample, error)
    Exec(ctx context.Context, siteID string, cmd []string) (ExecResult, error)
    Events(ctx context.Context) (<-chan RuntimeEvent, error)
}

// internal/proxy
type ProxyDriver interface {
    SetRoutes(ctx context.Context, routes []Route) error
    EnsureCertificates(ctx context.Context, domains []Domain) error
}
```

Planned implementations: `docker` (v0.1), `swarm` (v1.0), `k3s` (post v1). Proxy: `caddy` (v0.1), `traefik` (optional).

## 5. Optional modules

### mod/wordpress
- **Owns:** `wp_*` tables (installs, update policies, staging links metadata).
- **Needs:** `core/site`, `core/db`.
- **Exposes:** `WordPressService` (Install, CloneToStaging, PushStaging, SetAutoUpdate, SearchReplace, FlushCache).
- **Sidecars:** none (WP-CLI runs in the Site's own container).
- **Events consumed:** `site.deployed`, `backup.completed`.

### mod/dns
- **Owns:** `dns_zones`, `dns_records`, `dns_providers`.
- **Needs:** `core/site`.
- **Exposes:** `DNSService` (EnsureRecord, DeleteRecord, SyncZone, IssueDNS01).
- **Sidecars:** PowerDNS (optional); Cloudflare and Route 53 are API-only.
- **Used by:** wildcard ACME, `mod/mail`.

### mod/mail
- **Owns:** `mail_domains`, `mail_mailboxes`, `mail_aliases`, `mail_dkim_keys`.
- **Needs:** `mod/dns`.
- **Exposes:** `MailService` (EnsureDomain, CreateMailbox, RotateDKIM, CheckDeliverability).
- **Sidecars:** an integrated mail server (Stalwart or equivalent). The panel does not implement SMTP or IMAP.

### mod/waf
- **Owns:** `waf_policies`, `waf_events`.
- **Needs:** `core/site`.
- **Exposes:** `WAFService` (SetPolicy, Events, BlockIP).
- **Sidecars:** Coraza in the Caddy build, CrowdSec bouncer.

### mod/resell
- **Owns:** `resell_*` tables (reseller settings, branding, per-team overrides). Uses `packages` and `teams.parent_id` from core.
- **Needs:** `core/iam`, `core/site`.
- **Exposes:** `QuotaService` (Check, Enforce, Usage), hooks that veto Site creation when a quota is exceeded.

### mod/import
- **Owns:** `import_runs`, `import_mappings`.
- **Needs:** `core/site`.
- **Exposes:** `Importer` per source (`caprover`, `coolify`, `cloudpanel`, `cyberpanel`): `Plan(ctx, input) (Plan, error)` then `Apply(ctx, plan)`. Always dry-run first.

## 6. Events

An in-process event bus with a durable outbox for module handlers, stored in `event_outbox` and `event_deliveries` (migration 0002).

| Event | Emitted by | Typical consumers |
|---|---|---|
| `site.created`, `site.deleted` | site | dns, waf, resell |
| `site.deployed`, `site.failed` | site | wordpress, observe |
| `site.domain.added` | site | dns, waf |
| `node.offline` | node | observe |
| `incident.created` | observe | alerts |
| `backup.completed`, `backup.failed` | backup | observe |
| `quota.exceeded` | resell | site, observe |

## 7. Agent job types

Core: `site.apply`, `site.remove`, `site.exec`, `build.run`, `backup.run`, `restore.run`, `proxy.sync`.
Modules register additional job types with a name prefix (`wordpress.*`, `dns.*`, `mail.*`). The agent runs only job types in its signed allowlist for the enabled modules.

## 8. UI manifest

Each module ships a manifest describing nav entries, Site tabs and settings pages. The UI renders what is enabled; there is no hard-coded module UI in core.
