# ADR-0006: Web UI framework

Status: Proposed

## Context
The UI is a static SPA served by the control plane and must stay light.

## Decision
Open. Candidates: SvelteKit (static adapter), SolidJS, React with Vite. Decide after the API shape for Sites, logs and deployments is stable.

## Consequences
Blocks nothing in v0.1 because the CLI and API come first.
