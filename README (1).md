# web

Placeholder for the DeployPanel UI, a static single-page app served by `dpd` and built only on the REST API in `api/openapi.yaml`.

The framework is still open (ADR-0006). Nothing here should be built until the Sites, deployments and logs endpoints are stable, because the CLI and API come first.

When it exists, the build output is embedded into the `dpd` binary so the panel stays a single file.
