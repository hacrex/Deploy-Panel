# Security policy

DeployPanel manages servers, so its own security is a core feature, not an afterthought.

## Supported versions

The project is pre-release (scaffold stage). There are no supported versions yet. Once v0.1 ships, this table will list which releases receive security fixes.

| Version | Supported |
|---|---|
| main (unreleased) | Fixes land on main only |

## Reporting a vulnerability

Please report privately and do not open a public issue.

1. Use GitHub private vulnerability reporting on this repository (Security tab, "Report a vulnerability"), once the repository is published.
2. Or email the maintainers at `TODO: set a security contact address before publishing`.

Include what you found, how to reproduce it, the affected version or commit, and your assessment of impact. A proof of concept helps but is not required.

## What to expect

| Step | Target |
|---|---|
| Acknowledge your report | within 3 working days |
| Initial assessment and severity | within 10 working days |
| Fix or mitigation plan | agreed with you, based on severity |
| Public disclosure | coordinated, by default no later than 90 days after the report |

We will credit you in the advisory unless you prefer to stay anonymous.

## Scope

In scope:
- The control plane (`dpd`), node agent (`dp-agent`) and CLI (`dpctl`).
- The agent protocol, enrollment and job signing.
- Authentication, sessions, API tokens, RBAC and tenant isolation.
- Secrets handling, backups and the installer.
- Official container images and release artifacts.

Out of scope:
- Vulnerabilities in third-party software DeployPanel deploys for users, unless DeployPanel makes them exploitable through its own defaults.
- Findings that need an already fully compromised host or administrator account.
- Denial of service through unrealistic resource use on a self-managed server.
- Social engineering and physical attacks.

## Design rules we hold ourselves to

These come from `docs/ARCHITECTURE.md`. A change that breaks one is a security bug.

- No public admin port by default. The admin listener stays on loopback unless an operator opts in.
- Nodes expose no panel port and the control plane holds no SSH keys. Agents connect out over mTLS.
- Jobs sent to agents are signed and limited to an allowlist of job types.
- Only the agent touches the Docker socket. Hosted workloads never receive it.
- Secrets are encrypted at rest, never returned by the API after write, and redacted in logs and the audit log.
- Hosted PHP and WordPress sites run isolated: own user, resource limits, read-only root filesystem, no shared socket.
- Releases are signed and ship with an SBOM.

## Hardening checklist for operators

- Put the panel behind the proxy on its own domain with TLS and keep `DP_LISTEN` on loopback.
- Keep `DP_SECRET_KEY_FILE` at mode 0600 and back it up separately from the database.
- Enable 2FA for every admin account and use scoped API tokens.
- Remember that Docker published ports bypass host firewalls such as UFW; use the panel's firewall manager rather than relying on UFW rules alone.
- Keep the control plane and agents updated.
