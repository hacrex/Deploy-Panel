# Threat model

Status: Draft v0.1. Aligns with `SECURITY.md` design rules, `ARCHITECTURE.md` section 7 (security model) and PRD goals G3, N4, F-X1 to F-X5.

## 1. Purpose and scope

DeployPanel manages servers, so its own security is a core feature. This document enumerates the threats to the control plane, the agents and the hosted Sites, and the controls that mitigate them. It is a living document: every new feature or module gets a threat review before it reaches GA, per the roadmap ordering rule "security work lands before the feature it protects".

In scope:
- Control plane `dpd`: API, auth, sessions, scheduler, reconciler, module host, SQLite store.
- Node agent `dp-agent`: enrollment, mTLS stream, job execution, Docker socket access, collector.
- CLI `dpctl` and web UI as clients of the API.
- Hosted Sites (containers, Compose, PHP, WordPress, static, proxy) and their isolation.

Out of scope:
- Vulnerabilities in third-party software DeployPanel deploys for users, unless DeployPanel's own defaults make them exploitable.
- Findings requiring an already fully compromised host or administrator account.
- Denial of service through unrealistic resource use on a self-managed server.
- Social engineering and physical attacks.

## 2. Assets we protect

| Asset | Why it matters |
|---|---|
| Panel admin session (`dp_session` cookie) | Full control of the panel and every Site on it. |
| Users, teams, memberships, API tokens | Identity and RBAC; token theft impersonates a user or team. |
| Secrets at rest (site env secrets, deploy keys, DB passwords, registry credentials, TLS custom certs) | Decrypting them gives access to user workloads and connected services. |
| Control plane host and SQLite database | Contains all state; file access bypasses API authorization. |
| `DP_SECRET_KEY_FILE` | Master encryption key; without it ciphertext cannot be rotated or decrypted. |
| Agent host and `docker.sock` | Agent is the only component with socket access; compromise can run arbitrary containers. |
| Jobs sent to agents | A forged or replayed job can mutate any Site on any node. |
| Enrollment tokens | Short-lived tokens that bootstrap a node's mTLS identity. |
| Audit log | Forensic trail; must be append-only and redacted. |

## 3. Trust boundaries and actors

Trust boundaries:
- **Internet ↔ panel admin**: UI/API reachable only via the panel's own domain behind TLS; raw admin port not on the public interface.
- **Panel ↔ agent**: gRPC over mTLS; the agent dials out, nodes expose no panel port.
- **Control plane ↔ tenant Site**: Sites are isolated from each other by default (no network links unless explicitly created).
- **Agent ↔ Docker socket**: only the agent mounts it; tenant containers never do.
- **Panel ↔ operator host**: installer, systemd units, key file at `DP_SECRET_KEY_FILE`.

Actors:
- **Operator**: installs and runs the panel; holds the key file and server access.
- **Panel user**: belongs to a team with a role (owner, admin, developer, viewer, billing).
- **Tenant / hosted workload**: code running inside a customer Site.
- **Agent / node**: runs sites on behalf of the control plane.
- **External attacker**: unauthenticated network attacker, compromised tenant, compromised node.

## 4. Threat catalog

| # | Threat | Asset | Impact | Primary controls | Status |
|---|---|---|---|---|---|
| T1 | Public admin port exposed to the internet | Panel admin | Unauthenticated or credential-stuffed access to every Site | Panel UI only via its own domain with TLS; `DP_LISTEN` loopback unless `DP_ALLOW_PUBLIC_LISTEN` opt-in; installer self-test checks for open admin ports (F-X1, F-X4) | Implemented (default), installer not yet run on real host |
| T2 | Control plane compromise | Users, tokens, secrets, jobs, DB | Attacker can impersonate any user, issue jobs to all agents, read ciphertext | Loopback listener, no SSH key custody, signed jobs with allowlist, least-privilege agent, key file at 0600, key rotation metadata in `secret_keys` | Partially (tables + config exist; auth not yet implemented) |
| T3 | docker.sock abuse by a tenant container | Agent host, all Sites on the node | Arbitrary containers, host escape | Only the agent mounts the socket; tenant containers use read-only rootfs, dropped capabilities, no host mounts, user namespaces (F-X5) | Designed, not implemented |
| T4 | Tenant cross-site breakout (PHP/WordPress) | Other tenants' Sites and data | Read/modify another tenant's files or DB | One container + one Linux uid per Site, cgroup v2 limits, read-only rootfs, per-Site network isolation, explicit `site_links` only (F-X5) | Designed, not implemented |
| T5 | Secret exposure at rest | Site secrets, keys, certs | Credentials to user workloads and external services | Encrypted at rest with `v1:<key_id>:<nonce>:<ciphertext>` prefix, key material only in `DP_SECRET_KEY_FILE`, never returned by API, redacted in logs and audit | Schema exists; encryption service not yet implemented |
| T6 | Secret leakage in logs or audit | Site secrets | Credentials in plain text | Secrets redacted before logging and before audit write | Designed |
| T7 | Session hijacking / cookie theft | `dp_session` | Impersonate a logged-in user | `HttpOnly`, `Secure`, `SameSite=Lax` cookie, short absolute expiry, sliding `last_seen_at`, revocation column, binding check on use | Table exists; implementation not started |
| T8 | Credential stuffing / brute force on login | User accounts | Account takeover | bcrypt/argon2 hashing, per-endpoint rate limit (login returns 429), account lockout after failures, TOTP/WebAuthn 2FA (F-P2) | Not started |
| T9 | Stolen or over-scoped API token | Teams, Sites | API access beyond intended scope | `token_hash` unique, scopes stored as JSON array, `expires_at`, `last_used_at`, scoped tokens, token can be rotated/revoked | Table exists; implementation not started |
| T10 | Rogue or impersonated agent | Nodes, Sites | Unauthorized job execution or fake state reports | mTLS mutual auth, enrollment one-time token, client certificate rotation, jobs signed, agent enforces job-type allowlist | Contract drafted in `proto/`; not implemented |
| T11 | Job tampering or replay | Sites, backups, deploys | Mutating workloads out of band | Jobs signed, `idempotency_key` unique, `expires_at` (agents reject expired), status state machine, result verification | Table exists; implementation not started |
| T12 | SQL injection / IDOR | All data | Read or modify any row | Parameterized queries (`?` placeholders), ULID path IDs validated, ownership/team checks in every handler, foreign keys ON DELETE CASCADE | Not started |
| T13 | Supply chain: compromised release or base image | Control plane, agents | Backdoored binary or image | Signed releases, SBOM, pinned base images, reproducible builds where feasible, Dependabot for dependencies | Designed (F-X4) |
| T14 | Backup theft or untested restore | Site data, panel state | Data loss or extortion | Client-side encrypted backups to S3/local, scheduled restore tests with pass/fail reporting | Schema exists; not implemented |
| T15 | Reseller / parent team privilege escalation | Child teams' Sites and quotas | Parent admin reaches beyond its customers | `teams.parent_id` hierarchy, package caps flow down and cannot be exceeded by child, role-scoped membership checks | Table exists; not implemented |

Likelihood ratings are not assigned yet; add them during the v0.2 tenant isolation review.

## 5. Security assumptions

- The operator's host is not fully compromised; if it is, all bets are off (key file, DB and root access are out of scope).
- `DP_SECRET_KEY_FILE` is backed up separately from the database and kept at mode 0600.
- The panel is reached through its own domain with TLS; `DP_LISTEN` stays loopback by default.
- Docker published ports bypass host firewalls; they default to localhost unless routed through the panel proxy.
- Agents and Sites keep running during a control plane outage (N4); the outage window is a blind spot for new jobs and alerts, not for existing workloads.

## 6. Review cadence

- Every new module and every new job type gets a threat entry before GA.
- Tenant isolation review (cross-site PHP/WordPress, docker.sock) must pass before v0.2 GA.
- Revisit this model after the first real-host install and after any security advisory.
