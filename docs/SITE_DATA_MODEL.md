# Site data model

Source of truth for the SQL is `migrations/0001_core.sql`. Go types are in `internal/core/site`. This page explains the intent.

## 1. Core idea

Everything the panel hosts is a **Site**. A Site has a type, an owner team, a node, domains, a spec, limits, and a state machine. Type-specific details live in `spec` (JSON) and are validated in Go, so adding a Site type does not require a schema change.

## 2. Entity overview

```
Team 1---* Membership *---1 User
Team 1---1 Package
Team 1---* Site
Node 1---* Site
Site 1---* SiteDomain 1---* Certificate
Site 1---* Deployment
Site 1---* SiteEnv
Site 1---* SiteVolume
Site 1---* SiteCron
Site 1---* SiteAccessUser (sftp/ssh)
Site *---* Site            (site_links, explicit network allow)
Site 0..1---1 Site         (parent_site_id: staging -> production)
Site 1---0..1 GitSource
Site 1---* BackupPolicy 1---* Backup 1---* RestoreTest
Site 1---* Incident (crash snapshots)
DbInstance 1---* Database 1---* DbUser
Template                   (spec + variables, instantiated into a Site)
```

## 3. Site types and spec shapes

| type | spec fields (JSON) | Runtime unit |
|---|---|---|
| `container` | `image` or `build{context,dockerfile}`, `port`, `command`, `healthcheck`, `replicas` | 1 container |
| `compose` | `file` (inline or ref), `primary_service`, `primary_port` | Compose project |
| `php` | `php_version`, `docroot`, `fpm{max_children,...}`, `extensions[]`, `ini{}` | PHP-FPM container |
| `wordpress` | all `php` fields plus `wp{version,multisite,auto_update,cache}`, `database_id` | PHP-FPM container plus managed DB |
| `node` | `runtime_version`, `start`, `build`, `port`, `healthcheck` | Buildpack container |
| `python` | `runtime_version`, `start`, `build`, `port`, `healthcheck` | Buildpack container |
| `static` | `root`, `spa_fallback`, `headers{}` | Caddy file server |
| `proxy` | `upstream` (site id or URL), `preserve_host`, `headers{}` | Caddy route |

Validation rule: `spec` must unmarshal into the Go struct for the Site's `type` with unknown fields rejected. Changing `spec` bumps `spec_revision`; the reconciler applies the new revision.

## 4. State machine

`state` is the observed state. `desired_state` is what the user or an admin asked for.

```
draft -> provisioning -> running <-> stopped
                |           |  \
                v           v   v
              failed     degraded  suspended
                                        
any -> deleting -> deleted (soft; row kept for the restore window)
```

- `suspended`: set by an admin or quota enforcement, reversible, resources stopped but data kept.
- `degraded`: running but a health check, dependency or volume check is failing.
- Soft delete keeps `deleted_at` and backups until the retention window passes.

## 5. Ownership and authorization

- A Site belongs to exactly one team. Roles come from `memberships`:
  - `owner`, `admin`: everything including delete and member management.
  - `developer`: deploy, env, logs, backups; no delete, no member management.
  - `viewer`: read, logs; no secrets.
  - `billing`: team and package data only.
- Reseller hierarchy: `teams.parent_id`. A parent team's admins can act on child teams. Package limits apply to the child team and cannot exceed the parent's.
- Secrets (`site_env.is_secret=1`, deploy keys, DB passwords) are encrypted and never returned by the API after write.

## 6. Resource limits

`limits` JSON on the Site, capped by the team's Package:

```json
{"cpu_milli": 1000, "mem_mb": 512, "pids": 256, "disk_mb": 5120, "io_weight": 100}
```

The agent maps these to cgroup v2 settings. Disk is enforced with a project quota or an XFS/ext4 quota on the Site volume where available, otherwise monitored and alerted.

## 7. Domains and TLS

- A Site has many `site_domains`; exactly one may be primary.
- `tls_mode`: `acme-http`, `acme-dns` (wildcards, needs DNS credentials), `custom`, `off`.
- `redirect_to` makes a domain a redirect to another hostname (www to apex).
- `path_prefix` allows several Sites on one hostname, routed by path.

## 8. Deployments

- Every spec change that needs a build or restart creates a `deployment` with a monotonically increasing `revision` per Site.
- Statuses: `queued`, `building`, `starting`, `live`, `superseded`, `failed`, `rolled_back`.
- Only one deployment is `live` per Site. Rollback promotes a previous revision's image without rebuilding.
- `source_kind` keeps the origin: git, image, dockerfile, compose, archive, buildpack, git push, template.

## 9. Staging and linked Sites

- `parent_site_id` links a staging Site to its production Site. WordPress staging push and pull use this link.
- `site_links` is the only way two Sites can reach each other on the network. Default is isolation.

## 10. Databases

Managed DB engines run as Sites (type `container`) owned by the platform. `db_instances` tracks the engine, `databases` the logical databases, `db_users` the credentials. A Site references a database by id in its spec and receives connection details as env vars at deploy time.

## 11. Backups

- A `backup_policy` targets one Site (or the panel state when `site_id` is NULL) and one `backup_target`.
- A `backup` records outcome, size, checksum and location.
- A `restore_test` records an automated restore into a scratch Site and its pass or fail result.

## 12. Observability tables

- `incidents`: crash snapshots (last log lines, final stats, exit code, OOM flag).
- `log_lines`, `metric_samples`: TTL-pruned time series. Behind interfaces so large installs can swap storage.
- `alert_rules`: rule definitions. Channels are linked through `alert_rule_channels` to `notification_channels`.

## 13. Extension rules for modules

- Module tables are prefixed (`wp_`, `dns_`, `mail_`, `waf_`, `resell_`) and ship their own migrations.
- A module attaches data to a Site by `site_id` foreign key in its own table, never by adding columns to `sites`.
- Anything shared by several Site types goes in core. Anything specific to one feature goes in the module.

## 14. Sample rows

```json
{
  "id": "01JC2Z5K8X3A7M9Q4T6V1W2YBD",
  "team_id": "01JC2Z4...",
  "node_id": "01JC2Z4...",
  "slug": "blog",
  "name": "Company blog",
  "type": "wordpress",
  "state": "running",
  "desired_state": "running",
  "spec_revision": 7,
  "system_user": "s_blog",
  "limits": {"cpu_milli": 1000, "mem_mb": 768, "pids": 256, "disk_mb": 10240},
  "spec": {
    "php_version": "8.3",
    "docroot": "/public",
    "wp": {"version": "6.8", "auto_update": "minor", "cache": ["page", "object"]},
    "database_id": "01JC2Z6..."
  }
}
```

## 15. Platform tables (migration 0002)

| Table | Purpose |
|---|---|
| `sessions` | Login sessions: hashed cookie token, MFA flag, expiry, revocation |
| `jobs` | Control plane to agent queue. Unique `idempotency_key`, retry counts, expiry (agents reject expired jobs) |
| `notification_channels` | Webhook, email, Slack, Discord, Telegram targets, credentials encrypted |
| `alert_rule_channels` | Which channels an alert rule notifies. Replaces the JSON list formerly on `alert_rules` |
| `webhooks`, `webhook_deliveries` | Inbound (Git providers) and outbound webhooks with delivery history |
| `registry_credentials` | Private image registry logins per team |
| `settings` | Global and per-team key/value settings. `scope_id` is `''` for global so the key stays portable |
| `secret_keys` | Key metadata only (id, status, fingerprint). Key material lives in `DP_SECRET_KEY_FILE` |
| `event_outbox`, `event_deliveries` | Durable event bus: events are written with the change that caused them and fanned out to handlers with per-handler retry |

Encrypted values use the prefix `v1:<key_id>:<nonce>:<ciphertext>` so keys can be rotated and old values still decrypt until re-encrypted.
