# Configuration

`dpd` reads its settings from environment variables with the prefix `DP_`. Defaults live in `internal/config/config.go`, which is the source of truth if this page and the code ever disagree. A config file format is planned; until then the environment is the only source, which also fits systemd `EnvironmentFile` and containers.

`dpd` refuses to start on an invalid value and prints every problem it finds.

## Control plane (`dpd`)

| Variable | Default | Meaning |
|---|---|---|
| `DP_LISTEN` | `127.0.0.1:8080` | Admin UI and API. Must be a loopback address unless `DP_ALLOW_PUBLIC_LISTEN=true` |
| `DP_ALLOW_PUBLIC_LISTEN` | `false` | Opt in to binding `DP_LISTEN` on a non-loopback address. Needed inside containers |
| `DP_PUBLIC_URL` | empty | External URL of the panel, used for links and certificates |
| `DP_AGENT_LISTEN` | `127.0.0.1:8443` | Where node agents connect (mTLS). Set to a reachable address for multi-node installs |
| `DP_DB_DRIVER` | `sqlite` | `sqlite` only for now. `postgres` is planned and is rejected with a clear error |
| `DP_DB_PATH` | `/var/lib/deploypanel/dpd.db` | SQLite file path. The parent directory is created if missing |
| `DP_SECRET_KEY_FILE` | empty | Master key file for encrypting secrets. Mode `0600` or stricter, at least 32 bytes. Not set means secrets cannot be stored yet and `dpd` logs a warning |
| `DP_MODULES` | empty | Comma-separated optional modules to enable, for example `wordpress,dns,waf` |
| `DP_LOG_LEVEL` | `info` | `debug`, `info`, `warn` or `error` |
| `DP_LOG_TTL` | `7d` | How long site logs are kept. Accepts Go durations plus a `d` suffix (`36h`, `14d`) |
| `DP_METRIC_TTL` | `30d` | How long container metrics are kept |

### Secure defaults

- The admin listener stays on loopback. Put the panel behind the proxy on its own domain with TLS. Binding publicly needs an explicit opt-in, and `dpd` logs a warning when it happens.
- The agent listener is also loopback by default, so a single-node install exposes nothing. Opening it for multi-node use is a deliberate change.

### Creating the secret key

```
umask 077
head -c 48 /dev/urandom | base64 -w0 > /etc/deploypanel/secret.key
chown deploypanel:deploypanel /etc/deploypanel/secret.key
```

That gives 64 characters, which is above the 32-byte minimum. Back this file up separately from the database: without it, encrypted values (environment secrets, deploy keys, database passwords) cannot be read. Losing it is unrecoverable.

### SQLite connection settings

Applied by `internal/store/driver.go` on every connection, not by the migrations:

| Setting | Value | Why |
|---|---|---|
| foreign keys | on | SQLite ignores foreign keys unless enabled per connection |
| journal mode | WAL | Readers do not block the writer |
| busy timeout | 5000 ms | Wait for a lock instead of failing |
| synchronous | NORMAL | Safe with WAL, faster than FULL |
| transaction lock | immediate | Take the write lock at the start of a write transaction |

The driver is `mattn/go-sqlite3`, which needs CGO, so `dpd` is not a fully static binary yet (see `docs/adr/0001-single-binary-go.md`). `dp-agent` and `dpctl` build with `CGO_ENABLED=0`.

## Node agent (`dp-agent`)

The agent is a stub today and takes flags:

| Flag | Meaning |
|---|---|
| `--server` | Control plane agent address, for example `panel.example.com:8443` |
| `--enroll-token` | One-time enrollment token created in the panel |

The systemd unit reads these from `DP_SERVER` and `DP_ENROLL_TOKEN` in `/etc/deploypanel/agent.env`. After enrollment the agent stores its client certificate and no longer needs the token.

## Files and locations (systemd install)

| Path | Contents | Owner and mode |
|---|---|---|
| `/usr/local/bin/dpd`, `dp-agent`, `dpctl` | Binaries | root, 0755 |
| `/etc/deploypanel/dpd.env` | Control plane settings | root:deploypanel, 0640 |
| `/etc/deploypanel/agent.env` | Agent settings | root:dp-agent, 0640 |
| `/etc/deploypanel/secret.key` | Master key | deploypanel, 0600 |
| `/var/lib/deploypanel/` | SQLite database and state | deploypanel, 0750 |

## Planned

- A config file (YAML or TOML) layered under the environment.
- PostgreSQL support, which needs placeholder rebinding in the migration runner.
- Key rotation using the `secret_keys` table and the `v1:<key_id>:...` ciphertext prefix.
