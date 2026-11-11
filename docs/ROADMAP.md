# Roadmap

Each milestone ends in something installable and testable on a fresh VPS. Requirement IDs refer to PRD.md.

| Milestone | Theme | Scope | Exit criteria |
|---|---|---|---|
| v0.1 | Best single-server PaaS | F-S1 to S4, S7, D1, D4, O1 to O4, X1, X4. Dockerfile, Compose and image deploys, Caddy SSL, logs, stats, crash snapshots, CLI | Fresh VPS to deployed app in under 10 minutes; idle RSS under 150 MB; installer leaves no public admin port |
| v0.2 | Hosting layer | F-S5, S6, D2, D3, H1 to H3, B1, X2, X5, P4 (partial). PHP and WordPress sites, per-site users, SFTP, cron, S3 backups, Git webhooks, buildpacks, git push | WordPress Site with staging clone and scheduled S3 backup; tenant isolation review passed |
| v0.3 | Teams and multi-server | F-P1 to P3, H4, O5, X3. Agents on more nodes, RBAC, 2FA, audit, alerts, WAF | Two-node install; role matrix tests pass; alert on crash reaches Slack |
| v0.4 | Operators | F-H5, H6. Packages, quotas, resellers, DNS module | Reseller creates customer team with package; quota blocks over-limit Site |
| v1.0 | Migration and polish | F-D5, H7, P5, P6, B2. Importers, mail module integration, Swarm driver, restore tests | Importers hit the 90% target on the sample corpus; restore test runs on schedule |

## Ordering rules

1. No milestone starts a new module until the previous milestone's exit criteria pass.
2. Security work (isolation review, signed releases) lands before the feature it protects, not after.
3. Importers are built against real exports collected during v0.1 to v0.3 from CapRover, Coolify, CloudPanel and CyberPanel installs.

## First 4 weeks (v0.1 skeleton)

| Week | Deliverable |
|---|---|
| 1 | Repo, CI, migrations runner, SQLite store, IAM basics (users, sessions), `dpd` serves API and health |
| 2 | Agent enrollment and mTLS stream, Docker runtime driver `Apply`/`Remove`, Caddy driver |
| 3 | Site create and deploy for image, Dockerfile and Compose, deployment history, rollback |
| 4 | Logs and stats streaming, crash snapshots, `dpctl`, installer script, signed release pipeline |

## Scaffold status against week 1

| Item | State |
|---|---|
| Repo, CI, release workflow | Done (workflows written, not yet run on GitHub) |
| Migrations runner and SQLite store | Done, tested against real SQLite |
| Config with secure-default validation | Done, tested |
| Health endpoint | Done |
| Installer, systemd units, Dockerfiles | Written. Installer exercised with `--prefix` staging only; images and units not run on a real host |
| IAM basics (users, sessions, login) | Not started. Tables exist |
| Agent enrollment and mTLS stream | Not started. Contract drafted in `proto/` |
