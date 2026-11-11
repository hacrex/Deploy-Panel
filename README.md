# DeployPanel

> **Deploy anything. See everything. Trust the defaults.**

A lightweight, self-hosted web panel that runs everything as a **Site**: container apps, Compose stacks, PHP and WordPress sites, Node and Python apps, static sites and reverse proxies. It combines the deploy experience of CapRover, Coolify, Dokploy and Dokku with the hosting features of CloudPanel and CyberPanel, and adds built-in observability, secure-by-default access and restore-tested backups.

Status: scaffold. Docs and data model are drafted, code is a skeleton.

## Read in this order

0. [docs/BRAND.md](docs/BRAND.md): name, story, voice and vocabulary
1. [docs/PRD.md](docs/PRD.md): problem, goals, personas, requirements, risks, open questions
2. [docs/ARCHITECTURE.md](docs/ARCHITECTURE.md): components, deploy flow, isolation, security, protocol
3. [docs/SITE_DATA_MODEL.md](docs/SITE_DATA_MODEL.md): the Site entity and everything attached to it
4. [docs/MODULES.md](docs/MODULES.md): core vs optional modules, interfaces, events
5. [docs/ROADMAP.md](docs/ROADMAP.md): milestones and the first four weeks
   [docs/CONFIG.md](docs/CONFIG.md): every `DP_*` setting, key handling, file locations
   [docs/THREAT_MODEL.md](docs/THREAT_MODEL.md): assets, trust boundaries and threat catalog
   [docs/TEMPLATE_SPEC.md](docs/TEMPLATE_SPEC.md): site template catalog and unified spec
   [docs/API_CONVENTIONS.md](docs/API_CONVENTIONS.md): REST API conventions (auth, errors, pagination)
   [docs/TESTING.md](docs/TESTING.md): test layers, CI and local smoke test
6. [docs/adr/](docs/adr/): decisions, all currently Proposed

## Layout

```
cmd/dpd             control plane: config, SQLite, migrations, /healthz
cmd/dp-agent        node agent (stub)
cmd/dpctl           CLI (stub)
internal/config     DP_* environment config with secure-default validation (tested)
internal/store      SQLite open and forward-only migration runner (tested)
internal/core/site  Site types, spec validation, state machine (tested)
internal/module     module interface and dependency ordering (tested)
internal/boundary   test that enforces core/module import rules
internal/runtime    RuntimeDriver interface
internal/proxy      ProxyDriver interface
migrations/         forward-only SQL (core + platform tables), embedded in dpd
proto/              agent gRPC contract (buf)
api/                OpenAPI draft
deploy/             Dockerfiles, dev compose, systemd units, installer, env example
scripts/            schema and API checks
mod/                optional modules (none yet)
web/                UI placeholder
docs/               PRD, architecture, data model, modules, config, roadmap, brand, ADRs
.github/            CI, release and dependabot
```

## Try it

```
make ci            # gofmt, vet, tests, schema check, API check
make build         # bin/dpd (needs CGO and a C compiler), bin/dp-agent, bin/dpctl
DP_DB_PATH=/tmp/dpd.db ./bin/dpd    # GET http://127.0.0.1:8080/healthz
```

Settings are `DP_*` environment variables, see [docs/CONFIG.md](docs/CONFIG.md). `dpd` refuses to listen on a non-loopback address unless you opt in.

Requires Go 1.24, a C compiler (SQLite driver), and Python 3 with PyYAML for the checks. Replace `github.com/OWNER/deploypanel` in `go.mod` and the proto `go_package` once the repo location is chosen.

## Open decisions

See PRD section 9: primary persona for v0.1, license, mail server choice, PHP runtime model, SQLite-only vs optional Postgres, and whether to fold the existing log and stats monitor into the agent collector. The name is decided: DeployPanel, always written in full.
