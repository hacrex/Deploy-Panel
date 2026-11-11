# API conventions

Status: Draft v0.1. These conventions apply to the DeployPanel control plane API (`api/openapi.yaml`), used by the UI, `dpctl`, webhooks and the planned Terraform provider. Only `GET /healthz` is implemented today; `/v1/auth/*` and sites routes are drafted but not yet served.

## 1. Base URL and versioning

- Default listener: `http://127.0.0.1:8080` (loopback only; expose through the panel proxy with TLS).
- API prefix: `/v1`. One major version at a time; breaking changes bump to `/v2` and are announced in release notes.
- `GET /healthz` is unversioned and unauthenticated; it returns database status and the binary version.
- All other endpoints require authentication (cookie or bearer token).

## 2. Authentication

Two schemes, both registered in `api/openapi.yaml`:

| Scheme | Header / cookie | Used by |
|---|---|---|
| `cookieAuth` | `Cookie: dp_session=<value>` | Web UI, `dpctl` after `login` |
| `bearerAuth` | `Authorization: Bearer <token>` | API tokens, Terraform provider, service accounts |

- `POST /v1/auth/login` accepts `{email, password, totp?}` and sets an `HttpOnly`, `Secure`, `SameSite=Lax` `dp_session` cookie; returns `Me`.
- `POST /v1/auth/logout` clears the session and revokes it in the `sessions` table.
- `GET /v1/me` returns the current user and their team memberships/roles.
- Login is rate-limited; failures return `429`. Wrong credentials return `401` without revealing whether the email exists.
- Sessions: cookie token hashed (`token_hash` in `sessions`), `mfa_verified` flag, `expires_at`, `revoked_at`, sliding `last_seen_at` on use.
- API tokens: `token_hash` unique, `scopes` JSON array, `expires_at`, `last_used_at`; scoped to a user and optional team.

## 3. Identifiers and timestamps

- IDs are ULIDs: 26 characters, `[0-9A-HJKMNP-TV-Z]`, sortable by creation time. Example: `01JC2Z5K8X3A7M9Q4T6V1W2YBD`. Path parameters named `{siteId}`, `{nodeId}`, etc., all validated against the ULID pattern.
- Timestamps are RFC 3339 in UTC. Stored as text; `created_at`/`updated_at` set by the control plane, never by the client.

## 4. Request and response format

- Content-Type: `application/json` for request bodies and responses, except streams (section 7).
- Errors use a stable envelope:

```json
{
  "code": "site.invalid_spec",
  "message": "human readable message",
  "details": { "field": "spec.wp.version", "reason": "unsupported" }
}
```

`code` is machine-readable (`domain.action.reason`, lowercase, dots); `message` is for display; `details` is optional structured data. Never return stack traces or internal state in errors.
- Secrets are write-only: accepted on write, never returned afterward, and redacted in logs and audit (`detail` defaults to `{}` with secrets stripped).

## 5. Pagination

List endpoints use cursor pagination:

| Parameter | Type | Default | Notes |
|---|---|---|---|
| `limit` | integer | 50 | 1–200 |
| `cursor` | string | — | Opaque cursor; absent means first page |

Response:

```json
{ "items": [...], "next_cursor": "..." }
```

`next_cursor` is absent on the last page. Lists are newest-first where natural (deployments, incidents, log lines).

## 6. Idempotency

- Mutating jobs sent to agents carry a unique `idempotency_key`; re-enqueueing the same key returns the existing job (no duplicate work).
- Agent jobs are idempotent and carry a revision; agents ack by revision and report observed state.
- `POST` creation endpoints that are not naturally idempotent should document whether retries are safe.

## 7. Streams

- `GET /v1/sites/{siteId}/logs` returns `text/event-stream`; one `log` event per line. Query params: `follow` (default true), `tail` (default 200), `q` (substring filter).
- Client disconnect ends the stream; no server-side reconnection logic is required.

## 8. Authorization model

- Every request resolves to a caller: user (from session) or token (from bearer auth).
- A Site belongs to exactly one team. Access checks the caller's `memberships` for that team:
  - `owner`/`admin`: all actions including delete and member management.
  - `developer`: deploy, env, logs, backups; no delete, no member management.
  - `viewer`: read and logs; no secrets.
  - `billing`: team and package data only.
- Reseller hierarchy (`teams.parent_id`): a parent team's admins can act on child teams; package limits apply to the child and cannot exceed the parent's.
- Every mutating action is written to `audit_log` in the same transaction as the change (`actor_type` ∈ user|token|system|agent, secrets redacted).

## 9. Operational notes

- The admin listener must stay loopback unless the operator sets `DP_ALLOW_PUBLIC_LISTEN=true`; CI rejects non-loopback binds without the opt-in.
- CORS is intentionally minimal: the UI is served by the control plane itself, so cross-origin requests are not expected. If exposed behind a proxy, the proxy handles TLS and any CORS policy.
- `DP_PUBLIC_URL` is used for links in emails/API responses and for ACME challenges; it must be reachable from the public internet for ACME HTTP-01.
