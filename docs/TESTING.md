# Testing

Status: Draft v0.1. Covers Go unit/integration tests plus the repo's schema and API consistency checks. Goal: every milestone ends in something installable and testable on a fresh VPS (see ROADMAP.md exit criteria).

## 1. Test layers

| Layer | What it covers | Where |
|---|---|---|
| Unit | Config validation, secret key checks, spec validation, state machine transitions | `*_test.go` next to each package (config, core/site, store) |
| Store / migration | Forward-only SQL applied to real SQLite; downgrade detection (`ErrDatabaseNewer`); migration filename validation | `internal/store/store_test.go` |
| Auth | Password hashing, session expiry/revocation, cookie attributes, login rate limiting, role matrix | `internal/core/iam/` (to be created with login code) |
| API contract | `api/openapi.yaml` valid; handler behavior matches the draft (status codes, error codes, payload shapes) | `scripts/api_check.py` plus smoke tests |
| Schema | Migration files are portable SQL (`?` placeholders, no driver-specific PRAGMAs) | `scripts/schema_check.py` |
| Boundary | Core never imports `mod/*`; modules talk through interfaces only | `internal/boundary/` |

## 2. Running tests

```bash
make ci          # gofmt, vet, tests, schema check, API check
go test ./...    # Go tests only
```

Tests must run without a network connection and without Docker. Store tests use a real SQLite database in `t.TempDir()` so migration behavior is exercised against the actual driver.

## 3. Config tests (`internal/config`)

- Empty environment loads and defaults validate.
- `DP_LISTEN` non-loopback is rejected unless `DP_ALLOW_PUBLIC_LISTEN=true`.
- `DP_LOG_LEVEL`, `DP_DB_DRIVER`, `DP_MODULES`, `DP_LOG_TTL`/`DP_METRIC_TTL` invalid values are rejected.
- `CheckSecretKey`: missing file, group/world-readable file, and sub-32-byte file are all rejected; `0600` passes; no file configured is a no-op.

## 4. Store and migration tests

- `Open` creates the database file; `Migrate` applies 0001 and 0002 in order and returns the applied versions.
- Re-running `Migrate` applies nothing (idempotent).
- A database with a migration version this binary doesn't know fails with `ErrDatabaseNewer` (downgrade protection).
- Bad migration file names fail with a clear message.
- All tables and indexes from 0001/0002 exist after migration; foreign keys are on (`_foreign_keys=1`).

## 5. Auth tests (with login/IAM code)

- `bcrypt`/`argon2id` password hashing: correct password verifies, wrong password does not; cost is configurable.
- Session creation sets a valid cookie token hash; session lookup by hash works; `mfa_verified`, `expires_at`, `revoked_at` enforced.
- Session expiry: expired sessions rejected; revoked sessions rejected; `last_seen_at` slides on use.
- Login failures are counted and rate-limited; `429` after the threshold; `401` does not leak email existence.
- Role matrix: each role can/cannot perform the expected Site actions (developer cannot delete, viewer cannot see secrets, billing cannot mutate).
- API tokens: scopes enforced, expiry enforced, `last_used_at` updated on use.

## 6. API contract tests

- `api/openapi.yaml` parses and passes `scripts/api_check.py`.
- Smoke tests (against a real `dpd` on loopback):
  - `GET /healthz` → 200 `{status: ok, version}`.
  - `POST /v1/auth/login` with valid credentials → 200 + `dp_session` cookie + `Me`.
  - `POST /v1/auth/login` with bad password → 401.
  - `GET /v1/me` with session → 200; without session → 401.
  - `POST /v1/auth/logout` → 204; subsequent `GET /v1/me` → 401.
- Every error response matches the `Error` schema (`code` + `message`).

## 7. Security testing

Before v0.2 GA:
- Tenant isolation review: two Sites on the same node cannot reach each other unless a `site_link` exists; PHP/WordPress containers cannot reach the Docker socket or other Sites' volumes.
- Role matrix tests pass for all five roles across all mutating endpoints.
- Secrets are never returned by any GET endpoint and are redacted in `audit_log.detail`.
- Admin listener refuses non-loopback binds by default (config test covers this).

## 8. CI

`.github/workflows` runs `make ci` on push and pull request: `gofmt -l`, `go vet`, `go test ./...`, `scripts/schema_check.py`, `scripts/api_check.py`. Release workflow builds `bin/dpd`, `bin/dp-agent`, `bin/dpctl` and signs artifacts with an SBOM (planned).

## 9. Local smoke test

```bash
make build                              # bin/dpd (needs CGO and a C compiler)
DP_DB_PATH=/tmp/dpd.db ./bin/dpd        # GET http://127.0.0.1:8080/healthz
```

`dpd` refuses to listen on a non-loopback address unless `DP_ALLOW_PUBLIC_LISTEN` is set.
