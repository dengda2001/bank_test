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

cat <<EOF
Starting TrueLayer demo
  URL:          http://localhost${TL_ADDR}
  Redirect URI: ${TL_REDIRECT_URI}
  Environment:  ${TL_ENV}
  Log file:     ${TL_LOG_FILE}
  Token file:   ${TL_TOKEN_FILE}

Open http://localhost${TL_ADDR} and click "Connect bank".
EOF

exec go run ./cmd/truelayer-demo
