# ADR-0002: Outbound mTLS agents instead of SSH

Status: Proposed

## Context
Several panels keep SSH keys to every managed server, so one panel compromise exposes the fleet and nodes need inbound access.

## Decision
Each node runs an agent that dials out to the control plane over gRPC with mTLS. Jobs are signed and the agent enforces an allowlist of job types.

## Consequences
No inbound ports or SSH keys on nodes. The control plane can still push jobs, so signing and allowlists are mandatory. Agent upgrades need their own safe update path.
