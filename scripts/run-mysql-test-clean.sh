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

: "${MYSQL_DSN:?MYSQL_DSN is required; load it from .env or export it first}"

test_db="${RENTOPS_MYSQL_TEST_DB:-rentops_test}"
if [[ ! "$test_db" =~ ^[A-Za-z0-9_]+$ ]]; then
  printf 'RENTOPS_MYSQL_TEST_DB must contain only letters, digits, and underscores\n' >&2
  exit 1
fi

project_user="${MYSQL_DSN%%:*}"
if [[ -z "$project_user" || "$project_user" == "$MYSQL_DSN" ]]; then
  printf 'MYSQL_DSN must use user:password@tcp(host:port)/database format\n' >&2
  exit 1
fi

dsn_base="${MYSQL_DSN%%\?*}"
dsn_query=""
if [[ "$MYSQL_DSN" == *\?* ]]; then
  dsn_query="?${MYSQL_DSN#*\?}"
fi
dsn_prefix="${dsn_base%/*}"
if [[ "$dsn_prefix" == "$dsn_base" ]]; then
  printf 'MYSQL_DSN database path could not be replaced\n' >&2
  exit 1
fi
test_dsn="${dsn_prefix}/${test_db}${dsn_query}"

admin_host="${MYSQL_ADMIN_HOST:-127.0.0.1}"
admin_port="${MYSQL_ADMIN_PORT:-3306}"
admin_user="${MYSQL_ADMIN_USER:-root}"
admin_args=(--protocol=tcp --host="$admin_host" --port="$admin_port" --user="$admin_user")

mysql_admin() {
  if [[ -n "${MYSQL_ADMIN_PASSWORD:-}" ]]; then
    MYSQL_PWD="$MYSQL_ADMIN_PASSWORD" mysql "${admin_args[@]}" "$@"
  else
    mysql "${admin_args[@]}" "$@"
  fi
}

reset_test_database() {
  mysql_admin --batch --skip-column-names -e "DROP DATABASE IF EXISTS \`$test_db\`; CREATE DATABASE \`$test_db\` CHARACTER SET utf8mb4 COLLATE utf8mb4_unicode_ci; GRANT ALL PRIVILEGES ON \`$test_db\`.* TO '$project_user'@'127.0.0.1'; FLUSH PRIVILEGES;"
}

reset_test_database
trap reset_test_database EXIT

if [[ "$#" -eq 0 ]]; then
  set -- ./cmd/truelayer-demo -run '^Test(FigmaDomain|ExternalInvoice|RentArrangement|StructuredTenant|ManualBalanceReason|ManualBalanceSettlesOnlyTheOutstandingRentOnMySQL)$' -count=1
fi

printf 'Running MySQL-backed tests against %s (cleaned before and after)\n' "$test_db"
RENTOPS_MYSQL_TEST_DSN="$test_dsn" GOCACHE="${GOCACHE:-/tmp/bank-go-cache}" go test "$@"
