-- 0001_core.sql
-- Core schema. Portable between SQLite and PostgreSQL:
--   ids are ULID text, timestamps are RFC3339 UTC text, JSON is stored as text.
-- Module tables use their own prefix (wp_, dns_, mail_, waf_, resell_) and live in module migrations.
-- schema_migrations is created by the runner (internal/store), not by a migration.
-- SQLite needs foreign keys enabled per connection (see docs/CONFIG.md), not via a PRAGMA here.

-- ---------------------------------------------------------------- IAM
-- Quota plan. Applies to a team; limits flow down to its Sites.
CREATE TABLE packages (
    id            TEXT PRIMARY KEY,
    name          TEXT NOT NULL,
    max_sites     INTEGER,                           -- NULL = unlimited
    max_cpu_milli INTEGER,
    max_mem_mb    INTEGER,
    max_disk_mb   INTEGER,
    max_databases INTEGER,
    max_mailboxes INTEGER,
    created_at    TEXT NOT NULL
);

CREATE TABLE teams (
    id          TEXT PRIMARY KEY,
    name        TEXT NOT NULL,
    parent_id   TEXT REFERENCES teams(id),          -- reseller hierarchy (mod/resell)
    package_id  TEXT REFERENCES packages(id),
    created_at  TEXT NOT NULL
);

CREATE TABLE users (
    id            TEXT PRIMARY KEY,
    email         TEXT NOT NULL UNIQUE,
    password_hash TEXT NOT NULL,
    totp_secret   TEXT,                              -- encrypted
    disabled      INTEGER NOT NULL DEFAULT 0,
    created_at    TEXT NOT NULL
);

-- role: owner | admin | developer | viewer | billing
CREATE TABLE memberships (
    team_id  TEXT NOT NULL REFERENCES teams(id) ON DELETE CASCADE,
    user_id  TEXT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    role     TEXT NOT NULL CHECK (role IN ('owner','admin','developer','viewer','billing')),
    PRIMARY KEY (team_id, user_id)
);

CREATE TABLE api_tokens (
    id           TEXT PRIMARY KEY,
    user_id      TEXT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    team_id      TEXT REFERENCES teams(id) ON DELETE CASCADE,
    name         TEXT NOT NULL,
    token_hash   TEXT NOT NULL UNIQUE,
    scopes       TEXT NOT NULL DEFAULT '[]',         -- JSON array
    expires_at   TEXT,
    last_used_at TEXT,
    created_at   TEXT NOT NULL
);

CREATE TABLE audit_log (
    id          TEXT PRIMARY KEY,
    at          TEXT NOT NULL,
    team_id     TEXT,
    actor_type  TEXT NOT NULL CHECK (actor_type IN ('user','token','system','agent')),
    actor_id    TEXT,
    action      TEXT NOT NULL,                       -- e.g. site.deploy
    target_type TEXT,
    target_id   TEXT,
    ip          TEXT,
    detail      TEXT NOT NULL DEFAULT '{}'           -- JSON, secrets redacted
);
CREATE INDEX audit_team_at ON audit_log(team_id, at);

-- ---------------------------------------------------------------- Nodes
CREATE TABLE nodes (
    id            TEXT PRIMARY KEY,
    name          TEXT NOT NULL UNIQUE,
    status        TEXT NOT NULL CHECK (status IN ('pending','online','offline','draining')),
    public_ip     TEXT,
    arch          TEXT,
    cpu_milli     INTEGER,
    mem_mb        INTEGER,
    disk_mb       INTEGER,
    agent_version TEXT,
    cert_serial   TEXT,
    last_seen_at  TEXT,
    labels        TEXT NOT NULL DEFAULT '{}',        -- JSON
    created_at    TEXT NOT NULL
);

CREATE TABLE enrollment_tokens (
    id         TEXT PRIMARY KEY,
    token_hash TEXT NOT NULL UNIQUE,
    node_name  TEXT,
    expires_at TEXT NOT NULL,
    used_at    TEXT
);

-- ---------------------------------------------------------------- Sites
-- type decides which spec shape is valid (validated in Go, see internal/core/site).
CREATE TABLE sites (
    id             TEXT PRIMARY KEY,
    team_id        TEXT NOT NULL REFERENCES teams(id),
    node_id        TEXT REFERENCES nodes(id),
    parent_site_id TEXT REFERENCES sites(id),        -- staging clone points at its source
    slug           TEXT NOT NULL,                    -- unique per team, used in names
    name           TEXT NOT NULL,
    type           TEXT NOT NULL CHECK (type IN
                   ('container','compose','php','wordpress','node','python','static','proxy')),
    state          TEXT NOT NULL CHECK (state IN
                   ('draft','provisioning','running','stopped','degraded','failed','suspended','deleting','deleted')),
    desired_state  TEXT NOT NULL DEFAULT 'running' CHECK (desired_state IN ('running','stopped','deleted')),
    spec           TEXT NOT NULL DEFAULT '{}',       -- JSON, shape depends on type
    spec_revision  INTEGER NOT NULL DEFAULT 1,
    limits         TEXT NOT NULL DEFAULT '{}',       -- JSON: cpu_milli, mem_mb, pids, disk_mb, io
    system_user    TEXT,                             -- per-site Linux user (php, wordpress)
    system_uid     INTEGER,
    created_by     TEXT REFERENCES users(id),
    created_at     TEXT NOT NULL,
    updated_at     TEXT NOT NULL,
    deleted_at     TEXT,
    UNIQUE (team_id, slug)
);
CREATE INDEX sites_node ON sites(node_id);
CREATE INDEX sites_team_state ON sites(team_id, state);

CREATE TABLE site_domains (
    id          TEXT PRIMARY KEY,
    site_id     TEXT NOT NULL REFERENCES sites(id) ON DELETE CASCADE,
    hostname    TEXT NOT NULL UNIQUE,
    is_primary  INTEGER NOT NULL DEFAULT 0,
    redirect_to TEXT,                                -- hostname to redirect to, optional
    tls_mode    TEXT NOT NULL DEFAULT 'acme-http' CHECK (tls_mode IN ('acme-http','acme-dns','custom','off')),
    path_prefix TEXT NOT NULL DEFAULT '/',
    created_at  TEXT NOT NULL
);

CREATE TABLE certificates (
    id          TEXT PRIMARY KEY,
    domain_id   TEXT NOT NULL REFERENCES site_domains(id) ON DELETE CASCADE,
    issuer      TEXT NOT NULL,
    not_before  TEXT NOT NULL,
    not_after   TEXT NOT NULL,
    custom_pem  TEXT,                                -- encrypted, only for tls_mode=custom
    status      TEXT NOT NULL CHECK (status IN ('pending','active','failed','expired'))
);

-- Plain env and secret env. Secrets are encrypted at rest and never returned by the API.
CREATE TABLE site_env (
    site_id    TEXT NOT NULL REFERENCES sites(id) ON DELETE CASCADE,
    key        TEXT NOT NULL,
    value      TEXT NOT NULL,                        -- ciphertext when is_secret=1
    is_secret  INTEGER NOT NULL DEFAULT 0,
    updated_at TEXT NOT NULL,
    PRIMARY KEY (site_id, key)
);

CREATE TABLE site_volumes (
    id         TEXT PRIMARY KEY,
    site_id    TEXT NOT NULL REFERENCES sites(id) ON DELETE CASCADE,
    name       TEXT NOT NULL,
    mount_path TEXT NOT NULL,
    size_mb    INTEGER,
    backed_up  INTEGER NOT NULL DEFAULT 1,
    UNIQUE (site_id, name)
);

CREATE TABLE site_cron (
    id         TEXT PRIMARY KEY,
    site_id    TEXT NOT NULL REFERENCES sites(id) ON DELETE CASCADE,
    schedule   TEXT NOT NULL,                        -- cron expression
    command    TEXT NOT NULL,
    enabled    INTEGER NOT NULL DEFAULT 1,
    last_run_at TEXT,
    last_status TEXT
);

-- SFTP / SSH access for hosted (php, wordpress, static) Sites. Jailed to the Site volume.
CREATE TABLE site_access_users (
    id           TEXT PRIMARY KEY,
    site_id      TEXT NOT NULL REFERENCES sites(id) ON DELETE CASCADE,
    username     TEXT NOT NULL UNIQUE,
    kind         TEXT NOT NULL CHECK (kind IN ('sftp','ssh')),
    password_hash TEXT,
    public_keys  TEXT NOT NULL DEFAULT '[]',         -- JSON array
    created_at   TEXT NOT NULL
);

-- Explicit network links between Sites (default is full isolation).
CREATE TABLE site_links (
    from_site_id TEXT NOT NULL REFERENCES sites(id) ON DELETE CASCADE,
    to_site_id   TEXT NOT NULL REFERENCES sites(id) ON DELETE CASCADE,
    alias        TEXT NOT NULL,
    PRIMARY KEY (from_site_id, to_site_id)
);

-- ---------------------------------------------------------------- Deployments
CREATE TABLE deployments (
    id           TEXT PRIMARY KEY,
    site_id      TEXT NOT NULL REFERENCES sites(id) ON DELETE CASCADE,
    revision     INTEGER NOT NULL,
    source_kind  TEXT NOT NULL CHECK (source_kind IN ('git','image','dockerfile','compose','archive','buildpack','push','template')),
    source_ref   TEXT,                               -- repo@sha, image:tag, archive id
    image        TEXT,                               -- resulting image reference
    status       TEXT NOT NULL CHECK (status IN ('queued','building','starting','live','superseded','failed','rolled_back')),
    triggered_by TEXT NOT NULL DEFAULT 'manual',     -- manual | webhook | cli | schedule | rollback
    log_ref      TEXT,
    started_at   TEXT,
    finished_at  TEXT,
    created_at   TEXT NOT NULL,
    UNIQUE (site_id, revision)
);

CREATE TABLE git_sources (
    site_id     TEXT PRIMARY KEY REFERENCES sites(id) ON DELETE CASCADE,
    provider    TEXT NOT NULL CHECK (provider IN ('github','gitlab','bitbucket','gitea','generic')),
    repo_url    TEXT NOT NULL,
    branch      TEXT NOT NULL DEFAULT 'main',
    deploy_key  TEXT,                                -- encrypted private key
    webhook_secret TEXT,
    auto_deploy INTEGER NOT NULL DEFAULT 1
);

-- ---------------------------------------------------------------- Databases
CREATE TABLE db_instances (
    id        TEXT PRIMARY KEY,
    node_id   TEXT NOT NULL REFERENCES nodes(id),
    site_id   TEXT REFERENCES sites(id),             -- the Site that runs the engine
    engine    TEXT NOT NULL CHECK (engine IN ('mariadb','mysql','postgres','redis')),
    version   TEXT NOT NULL,
    status    TEXT NOT NULL,
    created_at TEXT NOT NULL
);

CREATE TABLE databases (
    id          TEXT PRIMARY KEY,
    instance_id TEXT NOT NULL REFERENCES db_instances(id) ON DELETE CASCADE,
    owner_site_id TEXT REFERENCES sites(id) ON DELETE SET NULL,
    name        TEXT NOT NULL,
    UNIQUE (instance_id, name)
);

CREATE TABLE db_users (
    id           TEXT PRIMARY KEY,
    database_id  TEXT NOT NULL REFERENCES databases(id) ON DELETE CASCADE,
    username     TEXT NOT NULL,
    password_enc TEXT NOT NULL,
    UNIQUE (database_id, username)
);

-- ---------------------------------------------------------------- Backups
CREATE TABLE backup_targets (
    id        TEXT PRIMARY KEY,
    team_id   TEXT NOT NULL REFERENCES teams(id),
    kind      TEXT NOT NULL CHECK (kind IN ('local','s3')),
    config    TEXT NOT NULL,                         -- JSON, credentials encrypted
    created_at TEXT NOT NULL
);

CREATE TABLE backup_policies (
    id              TEXT PRIMARY KEY,
    site_id         TEXT REFERENCES sites(id) ON DELETE CASCADE,   -- NULL = panel state
    target_id       TEXT NOT NULL REFERENCES backup_targets(id),
    schedule        TEXT NOT NULL,
    retention_count INTEGER NOT NULL DEFAULT 7,
    include_volumes INTEGER NOT NULL DEFAULT 1,
    include_databases INTEGER NOT NULL DEFAULT 1,
    restore_test    INTEGER NOT NULL DEFAULT 0,
    enabled         INTEGER NOT NULL DEFAULT 1
);

CREATE TABLE backups (
    id          TEXT PRIMARY KEY,
    policy_id   TEXT REFERENCES backup_policies(id) ON DELETE SET NULL,
    site_id     TEXT REFERENCES sites(id) ON DELETE SET NULL,
    status      TEXT NOT NULL CHECK (status IN ('running','ok','failed')),
    size_bytes  INTEGER,
    checksum    TEXT,
    location    TEXT,
    started_at  TEXT NOT NULL,
    finished_at TEXT,
    error       TEXT
);

CREATE TABLE restore_tests (
    id          TEXT PRIMARY KEY,
    backup_id   TEXT NOT NULL REFERENCES backups(id) ON DELETE CASCADE,
    scratch_site_id TEXT,
    status      TEXT NOT NULL CHECK (status IN ('running','passed','failed')),
    detail      TEXT,
    started_at  TEXT NOT NULL,
    finished_at TEXT
);

-- ---------------------------------------------------------------- Observability
-- Crash snapshot: captured by the agent when a container dies abnormally.
CREATE TABLE incidents (
    id          TEXT PRIMARY KEY,
    site_id     TEXT NOT NULL REFERENCES sites(id) ON DELETE CASCADE,
    node_id     TEXT NOT NULL REFERENCES nodes(id),
    container   TEXT NOT NULL,
    kind        TEXT NOT NULL CHECK (kind IN ('crash','oom','unhealthy','restart_loop')),
    exit_code   INTEGER,
    oom_killed  INTEGER NOT NULL DEFAULT 0,
    restart_count INTEGER NOT NULL DEFAULT 0,
    last_logs   TEXT,                                -- last N lines
    last_stats  TEXT,                                -- JSON array of final samples
    at          TEXT NOT NULL
);
CREATE INDEX incidents_site_at ON incidents(site_id, at);

CREATE TABLE log_lines (
    site_id   TEXT NOT NULL,
    container TEXT NOT NULL,
    at        INTEGER NOT NULL,                      -- unix nanoseconds
    stream    TEXT NOT NULL CHECK (stream IN ('stdout','stderr')),
    line      TEXT NOT NULL
);
CREATE INDEX log_site_at ON log_lines(site_id, at);

CREATE TABLE metric_samples (
    site_id   TEXT NOT NULL,
    container TEXT NOT NULL,
    at        INTEGER NOT NULL,                      -- unix seconds
    cpu_milli INTEGER,
    mem_bytes INTEGER,
    net_rx    INTEGER,
    net_tx    INTEGER,
    blk_r     INTEGER,
    blk_w     INTEGER
);
CREATE INDEX metric_site_at ON metric_samples(site_id, at);

CREATE TABLE alert_rules (
    id        TEXT PRIMARY KEY,
    team_id   TEXT NOT NULL REFERENCES teams(id),
    site_id   TEXT REFERENCES sites(id) ON DELETE CASCADE,
    kind      TEXT NOT NULL,                         -- crash | oom | disk | cert_expiry | backup_failed | cpu | mem
    threshold TEXT,
    channels  TEXT NOT NULL DEFAULT '[]',            -- JSON array of channel ids
    enabled   INTEGER NOT NULL DEFAULT 1
);

-- ---------------------------------------------------------------- Templates
CREATE TABLE templates (
    id          TEXT PRIMARY KEY,
    slug        TEXT NOT NULL UNIQUE,
    title       TEXT NOT NULL,
    source      TEXT NOT NULL CHECK (source IN ('builtin','caprover','coolify','community','custom')),
    site_type   TEXT NOT NULL,
    spec        TEXT NOT NULL,                       -- JSON template with variables
    variables   TEXT NOT NULL DEFAULT '[]',
    updated_at  TEXT NOT NULL
);

-- ---------------------------------------------------------------- Modules
CREATE TABLE modules (
    name        TEXT PRIMARY KEY,
    enabled     INTEGER NOT NULL DEFAULT 0,
    version     TEXT NOT NULL,
    config      TEXT NOT NULL DEFAULT '{}',
    enabled_at  TEXT
);
