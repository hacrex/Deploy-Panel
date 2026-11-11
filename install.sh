#!/usr/bin/env bash
# DeployPanel installer for Linux hosts with systemd.
#
# Status: skeleton. It has been syntax-checked and exercised with --prefix staging,
# but not run on a real host. Read it before running it as root.
#
# Run it from a checkout (it copies the unit files next to it). Examples:
#   sudo ./install.sh --role control --bin-dir ./bin --public-url https://panel.example.com
#   sudo ./install.sh --role agent   --bin-dir ./bin --server panel.example.com:8443 --token <enroll-token>
#   ./install.sh --role control --bin-dir ./bin --prefix /tmp/stage     # stage files, no root, no services
set -euo pipefail

ROLE=""
BIN_DIR=""
RELEASE_URL=""
VERSION=""
PUBLIC_URL=""
SERVER=""
TOKEN=""
PREFIX=""
DRY_RUN=0
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"

usage() {
  cat <<'EOF'
Usage: install.sh --role control|agent [options]

  --role control|agent   what to install on this host (required)
  --bin-dir DIR          directory containing dpd / dp-agent / dpctl binaries
  --release-url URL      base URL of a release, used when --bin-dir is not given.
                         Expects dpd_linux_<arch>, dp-agent_linux_<arch>, dpctl_linux_<arch>
                         and SHA256SUMS. Checksums are verified.
  --public-url URL       control: external URL of the panel (DP_PUBLIC_URL)
  --server HOST:PORT     agent: control plane agent address
  --token TOKEN          agent: one-time enrollment token
  --prefix DIR           stage everything under DIR; skips user creation, systemctl and the self-test
  --dry-run              print what would be done
  -h, --help             show this help
EOF
}

say() { printf '==> %s\n' "$*"; }
die() { printf 'error: %s\n' "$*" >&2; exit 1; }
run() { if [ "$DRY_RUN" = 1 ]; then printf '[dry-run] %s\n' "$*"; else "$@"; fi; }

while [ $# -gt 0 ]; do
  case "$1" in
    --role)        ROLE="${2:-}"; shift 2 ;;
    --bin-dir)     BIN_DIR="${2:-}"; shift 2 ;;
    --release-url) RELEASE_URL="${2:-}"; shift 2 ;;
    --public-url)  PUBLIC_URL="${2:-}"; shift 2 ;;
    --server)      SERVER="${2:-}"; shift 2 ;;
    --token)       TOKEN="${2:-}"; shift 2 ;;
    --prefix)      PREFIX="${2:-}"; shift 2 ;;
    --dry-run)     DRY_RUN=1; shift ;;
    -h|--help)     usage; exit 0 ;;
    *)             usage >&2; die "unknown option: $1" ;;
  esac
done

case "$ROLE" in control|agent) ;; *) usage >&2; die "--role must be control or agent" ;; esac
[ -n "$BIN_DIR" ] || [ -n "$RELEASE_URL" ] || die "give --bin-dir or --release-url"
if [ "$ROLE" = agent ]; then
  [ -n "$SERVER" ] || die "--server is required for the agent"
  [ -n "$TOKEN" ]  || die "--token is required for the agent"
fi

STAGED=0
[ -n "$PREFIX" ] && STAGED=1
if [ "$STAGED" = 0 ] && [ "$DRY_RUN" = 0 ] && [ "$(id -u)" -ne 0 ]; then
  die "run as root, or use --prefix to stage files without root"
fi
if [ "$STAGED" = 0 ] && [ "$DRY_RUN" = 0 ]; then
  command -v systemctl >/dev/null || die "systemd is required"
  [ "$ROLE" = agent ] && { command -v docker >/dev/null || die "Docker is required on agent nodes"; }
fi

BIN_DEST="$PREFIX/usr/local/bin"
ETC="$PREFIX/etc/deploypanel"
UNIT_DEST="$PREFIX/etc/systemd/system"

case "$(uname -m)" in
  x86_64|amd64)  ARCH=amd64 ;;
  aarch64|arm64) ARCH=arm64 ;;
  *) die "unsupported architecture: $(uname -m)" ;;
esac

WANTED="dpd dpctl"
[ "$ROLE" = agent ] && WANTED="dp-agent"

fetch_binaries() {
  local tmp name
  tmp="$(mktemp -d)"
  trap 'rm -rf "$tmp"' RETURN
  say "downloading release files from $RELEASE_URL"
  curl -fsSL "$RELEASE_URL/SHA256SUMS" -o "$tmp/SHA256SUMS"
  for name in $WANTED; do
    curl -fsSL "$RELEASE_URL/${name}_linux_${ARCH}" -o "$tmp/${name}_linux_${ARCH}"
  done
  ( cd "$tmp" && grep -E " (${WANTED// /|})_linux_${ARCH}\$" SHA256SUMS | sha256sum -c - ) \
    || die "checksum verification failed"
  say "checksums verified. Also verify the release signature before trusting it."
  for name in $WANTED; do
    install -D -m 0755 "$tmp/${name}_linux_${ARCH}" "$BIN_DEST/$name"
  done
}

copy_binaries() {
  local name
  for name in $WANTED; do
    [ -f "$BIN_DIR/$name" ] || die "$BIN_DIR/$name not found"
    run install -D -m 0755 "$BIN_DIR/$name" "$BIN_DEST/$name"
  done
}

ensure_user() { # name, extra useradd args
  local name="$1"; shift
  [ "$STAGED" = 1 ] && return 0
  if ! id "$name" >/dev/null 2>&1; then
    run useradd --system --no-create-home --shell /usr/sbin/nologin "$@" "$name"
  fi
}

install_units() {
  local unit
  for unit in "$@"; do
    [ -f "$SCRIPT_DIR/systemd/$unit" ] || die "missing $SCRIPT_DIR/systemd/$unit"
    run install -D -m 0644 "$SCRIPT_DIR/systemd/$unit" "$UNIT_DEST/$unit"
  done
}

install_control() {
  ensure_user deploypanel
  run install -d -m 0750 "$ETC"
  if [ "$STAGED" = 0 ] && [ "$DRY_RUN" = 0 ]; then chgrp deploypanel "$ETC"; fi

  local key="$ETC/secret.key"
  if [ ! -e "$key" ]; then
    say "creating master key $key"
    if [ "$DRY_RUN" = 1 ]; then
      echo "[dry-run] generate 48 random bytes into $key with mode 0600"
    else
      ( umask 077; head -c 48 /dev/urandom | base64 -w0 > "$key" )
      chmod 0600 "$key"
      [ "$STAGED" = 0 ] && chown deploypanel:deploypanel "$key"
    fi
    say "BACK UP $key separately from the database. Without it, encrypted secrets cannot be read."
  fi

  local env="$ETC/dpd.env"
  if [ ! -e "$env" ]; then
    say "writing $env"
    if [ "$DRY_RUN" = 0 ]; then
      cp "$SCRIPT_DIR/dpd.env.example" "$env"
      if [ -n "$PUBLIC_URL" ]; then
        sed -i "s|^# DP_PUBLIC_URL=.*|DP_PUBLIC_URL=$PUBLIC_URL|" "$env"
      fi
      chmod 0640 "$env"
      [ "$STAGED" = 0 ] && chgrp deploypanel "$env"
    fi
  else
    say "$env exists, leaving it unchanged"
  fi
  install_units dpd.service
}

install_agent() {
  ensure_user dp-agent --groups docker
  run install -d -m 0750 "$ETC"
  local env="$ETC/agent.env"
  say "writing $env"
  if [ "$DRY_RUN" = 0 ]; then
    ( umask 027; printf 'DP_SERVER=%s\nDP_ENROLL_TOKEN=%s\n' "$SERVER" "$TOKEN" > "$env" )
    [ "$STAGED" = 0 ] && chgrp dp-agent "$env"
  fi
  install_units dp-agent.service
}

# Fails the install if the admin port is reachable from outside the host.
selftest_control() {
  local env="$ETC/dpd.env" listen port i=0
  listen="$(sed -n 's/^DP_LISTEN=//p' "$env" | tail -n1)"
  listen="${listen:-127.0.0.1:8080}"
  port="${listen##*:}"

  say "self-test: waiting for dpd on $listen"
  until curl -fsS "http://$listen/healthz" >/dev/null 2>&1; do
    i=$((i + 1)); [ "$i" -ge 20 ] && die "dpd did not become healthy; see: journalctl -u dpd"
    sleep 1
  done

  if command -v ss >/dev/null; then
    local exposed
    exposed="$(ss -Hltn "sport = :$port" | awk '{print $4}' | grep -v -E '^(127\.|\[::1\]|::1)' || true)"
    if [ -n "$exposed" ]; then
      die "admin port $port is listening on a non-loopback address: $exposed"
    fi
    say "self-test: admin port $port is loopback only"
  else
    say "self-test: 'ss' not found, could not check for public listeners; check manually"
  fi
}

say "installing DeployPanel ($ROLE) for $ARCH"
if [ -n "$BIN_DIR" ]; then
  copy_binaries
elif [ "$DRY_RUN" = 1 ]; then
  echo "[dry-run] download $WANTED from $RELEASE_URL and verify SHA256SUMS"
else
  fetch_binaries
fi

if [ "$ROLE" = control ]; then install_control; else install_agent; fi

if [ "$STAGED" = 1 ] || [ "$DRY_RUN" = 1 ]; then
  say "staged only: no users created, no services started"
  exit 0
fi

run systemctl daemon-reload
if [ "$ROLE" = control ]; then
  run systemctl enable --now dpd.service
  selftest_control
  say "done. Put the panel behind your proxy with TLS: it listens on loopback only."
else
  run systemctl enable --now dp-agent.service
  say "done. Check enrollment with: journalctl -u dp-agent"
fi
