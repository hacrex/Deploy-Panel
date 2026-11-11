# ADR-0005: Optional modules, off by default

Status: Proposed

## Context
Mail, DNS, WAF and reseller features are large and not needed by every user. Bundling them in core would break the footprint goal.

## Decision
Core covers Sites, nodes, IAM, observability, backups and databases. Everything else is a module with its own tables, routes, jobs and optional sidecars.

## Consequences
Small default install and clear ownership. Needs a stable module interface and an event bus early.
