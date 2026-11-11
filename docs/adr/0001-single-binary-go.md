# ADR-0001: Go single-binary control plane with SQLite default

Status: Proposed

## Context
The panel must idle under 150 MB and install in minutes. Competing tools run several services (app, Postgres, Redis).

## Decision
Write the control plane in Go as one binary. Use SQLite in WAL mode by default. Keep SQL portable so PostgreSQL works for larger fleets.

## Consequences
Smallest footprint and simplest install. SQLite write concurrency limits very large fleets, so logs and metrics sit behind store interfaces and can move elsewhere. Migrations must avoid SQLite-only or Postgres-only features.

### Update (driver choice)
The SQLite driver is `github.com/mattn/go-sqlite3`, which needs CGO, so `dpd` is a single binary but not a fully static one. `dp-agent` and `dpctl` stay static. The driver is isolated in `internal/store/driver.go`. Moving to a pure-Go driver such as `modernc.org/sqlite` is a one-file change and would restore a static `dpd`; decide before the first release because it affects images (glibc base vs static) and cross-compilation.
