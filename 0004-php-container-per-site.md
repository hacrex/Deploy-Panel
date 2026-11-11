# ADR-0004: One PHP-FPM container per hosted Site

Status: Proposed

## Context
Hosting panels use native PHP pools. That shares a kernel and filesystem view across tenants and mixes with the container runtime.

## Decision
Run one PHP-FPM container per Site with its own uid, user namespace, cgroup limits and read-only root, behind shared Caddy. Defer OpenLiteSpeed.

## Consequences
Consistent runtime and stronger isolation. Higher memory per Site than shared pools, mitigated by FPM ondemand and limits. LSCache is not available.
