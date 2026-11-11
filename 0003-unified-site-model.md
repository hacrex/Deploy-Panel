# ADR-0003: Unified Site model for all workloads

Status: Proposed

## Context
PaaS tools model apps and hosting panels model websites. Supporting both with separate models would duplicate deploy, SSL, backup and access control.

## Decision
Everything hosted is a Site with a type. Type-specific configuration lives in a validated JSON spec.

## Consequences
One code path for lifecycle, domains, backups and logs. New types need no schema change. The spec validators become the contract to keep stable and versioned.
