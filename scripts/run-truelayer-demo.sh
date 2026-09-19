#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$ROOT_DIR"

if [[ -f ".env" ]]; then
  set -a
  # shellcheck disable=SC1091
  source ".env"
  set +a
fi

export TL_ENV="${TL_ENV:-sandbox}"
export TL_ADDR="${TL_ADDR:-:8080}"
export TL_REDIRECT_URI="${TL_REDIRECT_URI:-http://localhost:8080/callback}"
export TL_LOG_FILE="${TL_LOG_FILE:-bank-data.jsonl}"
export TL_TOKEN_FILE="${TL_TOKEN_FILE:-truelayer-token.json}"
export MIGRATIONS_DIR="${MIGRATIONS_DIR:-migrations}"

missing=()
if [[ -z "${TL_CLIENT_ID:-}" ]]; then
  missing+=("TL_CLIENT_ID")
fi
if [[ -z "${TL_CLIENT_SECRET:-}" ]]; then
  missing+=("TL_CLIENT_SECRET")
fi

if (( ${#missing[@]} > 0 )); then
  printf 'Missing required environment variable(s): %s\n\n' "${missing[*]}" >&2
  cat >&2 <<'EOF'
Create a local .env file, for example:

TL_ENV=sandbox
TL_CLIENT_ID=your-client-id
TL_CLIENT_SECRET=your-client-secret
TL_REDIRECT_URI=http://localhost:8080/callback

Optional:
TL_PROVIDERS=...
TL_PROVIDER_ID=...
TL_AUTH_URL=https://auth.truelayer.com/?...
TL_FROM=2026-01-01
TL_LOG_FILE=bank-data.jsonl
TL_TOKEN_FILE=truelayer-token.json
EOF
  exit 1
fi

if [[ -z "${MYSQL_DSN:-}" && -z "${DATABASE_URL:-}" ]]; then
  printf 'Missing database configuration: set MYSQL_DSN or DATABASE_URL\n' >&2
  exit 1
fi

if [[ "${TL_ENV}" == "live" && -z "${BANK_TOKEN_ENCRYPTION_KEY:-}" ]]; then
  printf 'Missing BANK_TOKEN_ENCRYPTION_KEY for live token storage\n' >&2
  exit 1
fi

stop_existing_listener() {
  local port="$1"
  local pids

  if ! command -v lsof >/dev/null 2>&1; then
    printf 'Cannot check port %s: lsof is not installed\n' "$port" >&2
    return 1
  fi

  pids="$(lsof -tiTCP:"$port" -sTCP:LISTEN 2>/dev/null || true)"
  if [[ -z "$pids" ]]; then
    return 0
  fi

  printf 'Stopping process(es) listening on port %s: %s\n' "$port" "${pids//$'\n'/ }" >&2
  while IFS= read -r pid; do
    [[ -n "$pid" ]] && kill "$pid" 2>/dev/null || true
  done <<< "$pids"

  for _ in {1..50}; do
    if ! lsof -tiTCP:"$port" -sTCP:LISTEN >/dev/null 2>&1; then
      return 0
    fi
    sleep 0.1
  done

  printf 'Port %s is still occupied after stopping the existing process\n' "$port" >&2
  return 1
}

port="${TL_ADDR##*:}"
if [[ ! "$port" =~ ^[0-9]+$ ]]; then
  printf 'Cannot determine the listening port from TL_ADDR=%s\n' "$TL_ADDR" >&2
  exit 1
fi
if ! stop_existing_listener "$port"; then
  exit 1
fi

cat <<EOF
Starting TrueLayer demo
  URL:          http://localhost${TL_ADDR}
  Redirect URI: ${TL_REDIRECT_URI}
  Environment:  ${TL_ENV}
  Database:     ${MYSQL_DSN:-${DATABASE_URL}}
  Log file:     ${TL_LOG_FILE}
  Token file:   ${TL_TOKEN_FILE}

Open http://localhost${TL_ADDR} and click "Connect bank".
EOF

exec go run ./cmd/truelayer-demo
