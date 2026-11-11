-- 0002_platform.sql
-- Platform tables the core services depend on: login sessions, the agent job queue,
-- notification channels, webhooks, private registry credentials, settings and
-- encryption key metadata.
-- Portable between SQLite and PostgreSQL (same rules as 0001).

-- ---------------------------------------------------------------- Auth sessions
CREATE TABLE sessions (
    id           TEXT PRIMARY KEY,
    user_id      TEXT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    token_hash   TEXT NOT NULL UNIQUE,               -- hash of the session cookie value
    mfa_verified INTEGER NOT NULL DEFAULT 0,
    ip           TEXT,
    user_agent   TEXT,
    created_at   TEXT NOT NULL,
    last_seen_at TEXT NOT NULL,
    expires_at   TEXT NOT NULL,
    revoked_at   TEXT
);
CREATE INDEX sessions_user ON sessions(user_id);
CREATE INDEX sessions_expires ON sessions(expires_at);

-- ---------------------------------------------------------------- Agent job queue
-- Control plane to agent work. Jobs are idempotent: idempotency_key is unique, and
-- re-enqueueing the same key returns the existing job.
CREATE TABLE jobs (
    id              TEXT PRIMARY KEY,
    node_id         TEXT NOT NULL REFERENCES nodes(id) ON DELETE CASCADE,
    site_id         TEXT REFERENCES sites(id) ON DELETE CASCADE,
    type            TEXT NOT NULL,                   -- site.apply, backup.run, wordpress.install, ...
    revision        INTEGER NOT NULL DEFAULT 0,
    payload         TEXT NOT NULL DEFAULT '{}',      -- JSON, secrets referenced by id not inlined
    status          TEXT NOT NULL CHECK (status IN
                    ('queued','sent','acked','running','succeeded','failed','expired','cancelled')),
    idempotency_key TEXT NOT NULL UNIQUE,
    attempts        INTEGER NOT NULL DEFAULT 0,
    max_attempts    INTEGER NOT NULL DEFAULT 3,
    result          TEXT,                            -- JSON from the agent
    error           TEXT,
    created_at      TEXT NOT NULL,
    scheduled_at    TEXT NOT NULL,                   -- not before this time
    expires_at      TEXT NOT NULL,                   -- agent must reject after this
    sent_at         TEXT,
    acked_at        TEXT,
    finished_at     TEXT
);
CREATE INDEX jobs_node_status ON jobs(node_id, status);
CREATE INDEX jobs_site ON jobs(site_id);
CREATE INDEX jobs_due ON jobs(status, scheduled_at);

-- ---------------------------------------------------------------- Notifications
CREATE TABLE notification_channels (
    id         TEXT PRIMARY KEY,
    team_id    TEXT NOT NULL REFERENCES teams(id) ON DELETE CASCADE,
    kind       TEXT NOT NULL CHECK (kind IN ('webhook','email','slack','discord','telegram')),
    name       TEXT NOT NULL,
    config     TEXT NOT NULL,                        -- JSON, credentials encrypted
    enabled    INTEGER NOT NULL DEFAULT 1,
    created_at TEXT NOT NULL,
    UNIQUE (team_id, name)
);

-- Which channels an alert rule notifies. Replaces the JSON list that 0001 kept on alert_rules,
-- so channel ids have real foreign keys and deleting a channel cleans up its links.
CREATE TABLE alert_rule_channels (
    rule_id    TEXT NOT NULL REFERENCES alert_rules(id) ON DELETE CASCADE,
    channel_id TEXT NOT NULL REFERENCES notification_channels(id) ON DELETE CASCADE,
    PRIMARY KEY (rule_id, channel_id)
);
CREATE INDEX alert_rule_channels_channel ON alert_rule_channels(channel_id);
ALTER TABLE alert_rules DROP COLUMN channels;

-- ---------------------------------------------------------------- Webhooks
-- inbound: Git providers calling us to trigger a deploy. outbound: we call a URL on events.
CREATE TABLE webhooks (
    id               TEXT PRIMARY KEY,
    team_id          TEXT NOT NULL REFERENCES teams(id) ON DELETE CASCADE,
    site_id          TEXT REFERENCES sites(id) ON DELETE CASCADE,
    direction        TEXT NOT NULL CHECK (direction IN ('inbound','outbound')),
    url              TEXT,                           -- outbound target
    secret           TEXT NOT NULL,                  -- encrypted, HMAC signing or verification
    events           TEXT NOT NULL DEFAULT '[]',     -- JSON array of event names
    enabled          INTEGER NOT NULL DEFAULT 1,
    last_delivery_at TEXT,
    last_status      INTEGER,
    created_at       TEXT NOT NULL
);
CREATE INDEX webhooks_site ON webhooks(site_id);

CREATE TABLE webhook_deliveries (
    id          TEXT PRIMARY KEY,
    webhook_id  TEXT NOT NULL REFERENCES webhooks(id) ON DELETE CASCADE,
    event       TEXT NOT NULL,
    status_code INTEGER,
    error       TEXT,
    duration_ms INTEGER,
    at          TEXT NOT NULL
);
CREATE INDEX webhook_deliveries_hook_at ON webhook_deliveries(webhook_id, at);

-- ---------------------------------------------------------------- Private registries
CREATE TABLE registry_credentials (
    id           TEXT PRIMARY KEY,
    team_id      TEXT NOT NULL REFERENCES teams(id) ON DELETE CASCADE,
    name         TEXT NOT NULL,
    registry     TEXT NOT NULL,                      -- ghcr.io, registry.example.com, ...
    username     TEXT NOT NULL,
    password_enc TEXT NOT NULL,
    created_at   TEXT NOT NULL,
    UNIQUE (team_id, name)
);

-- ---------------------------------------------------------------- Settings
-- scope 'global' uses scope_id '' ; scope 'team' uses the team id. A non-null scope_id
-- keeps the primary key portable (NULL in a composite key behaves differently per database).
CREATE TABLE settings (
    scope      TEXT NOT NULL CHECK (scope IN ('global','team')),
    scope_id   TEXT NOT NULL DEFAULT '',
    key        TEXT NOT NULL,
    value      TEXT NOT NULL,
    updated_at TEXT NOT NULL,
    PRIMARY KEY (scope, scope_id, key)
);

-- ---------------------------------------------------------------- Encryption keys
-- Metadata only. Key material lives in the file named by DP_SECRET_KEY_FILE, never in the database.
-- Ciphertext columns carry the key id in their value prefix (v1:<key_id>:<nonce>:<ciphertext>)
-- so keys can be rotated and old values still decrypt until re-encrypted.
CREATE TABLE secret_keys (
    id          TEXT PRIMARY KEY,
    status      TEXT NOT NULL CHECK (status IN ('active','retired')),
    fingerprint TEXT NOT NULL,                       -- hash of the key, to detect a wrong key file
    created_at  TEXT NOT NULL,
    retired_at  TEXT
);

-- ---------------------------------------------------------------- Event outbox
-- Durable in-process event bus (docs/MODULES.md section 6). Core writes an event in the same
-- transaction as the change that caused it; a dispatcher then fans it out to module handlers.
-- event_deliveries tracks each handler separately so one failing module does not block others.
CREATE TABLE event_outbox (
    id           TEXT PRIMARY KEY,                   -- ULID, sorts by creation time
    topic        TEXT NOT NULL,                      -- site.deployed, node.offline, ...
    aggregate_type TEXT,
    aggregate_id TEXT,
    payload      TEXT NOT NULL DEFAULT '{}',         -- JSON
    created_at   TEXT NOT NULL
);
CREATE INDEX event_outbox_topic_created ON event_outbox(topic, created_at);

CREATE TABLE event_deliveries (
    event_id     TEXT NOT NULL REFERENCES event_outbox(id) ON DELETE CASCADE,
    handler      TEXT NOT NULL,                      -- module or core handler name
    status       TEXT NOT NULL CHECK (status IN ('pending','delivered','failed','dead')),
    attempts     INTEGER NOT NULL DEFAULT 0,
    next_attempt_at TEXT NOT NULL,
    last_error   TEXT,
    delivered_at TEXT,
    PRIMARY KEY (event_id, handler)
);
CREATE INDEX event_deliveries_due ON event_deliveries(status, next_attempt_at);
