#!/usr/bin/env bash
# Start a disposable, populated room-rent instance for the browser audit.
#
# This is the audit counterpart to scripts/run-e2e-local.sh, with one crucial
# difference: it does NOT run a destructive acceptance suite. It creates a
# throwaway MySQL database, starts the app against it, runs the room-plan seeder
# (scripts/audit/seed-workspace.mjs), and then STAYS RUNNING so a Playwright
# harness can attach to a live, populated instance.
#
# Safety, mirroring the E2E precedent:
#   * the database name is generated from the run ID and must match
#     `rentops_audit_*`, otherwise the script refuses to run;
#   * MYSQL_HOST must be loopback;
#   * the base URL is always the port this script started — never :8081 or
#     bank.ddpl.top, which are the production instance and database.
#
# Interaction model: the app is forked to the background and this script
# `wait`s on its PID. That keeps ownership of the process so a single Ctrl-C
# (SIGINT) runs the cleanup trap and guarantees both the app and the throwaway
# database are torn down. Run the harness from a second terminal against the
# printed base URL.
#
# Env overrides:
#   APP_PORT (default 18090), MYSQL_HOST (default 127.0.0.1),
#   MYSQL_PORT (default 53306), MYSQL_ADMIN_CMD (admin mysql client command),
#   AUDIT_RUN_ID (override the generated run ID; mainly for testing the
#   namespace guard), AUDIT_KEEP=1 (keep the database after Ctrl-C).

set -euo pipefail

REPO_ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
APP_HOST="${APP_HOST:-127.0.0.1}"
APP_PORT="${APP_PORT:-18090}"
BASE_URL="http://${APP_HOST}:${APP_PORT}"

RUN_ID="${AUDIT_RUN_ID:-rentops-audit-$(date -u +%Y%m%d-%H%M%S)-$(openssl rand -hex 4)}"
MYSQL_HOST="${MYSQL_HOST:-127.0.0.1}"
MYSQL_PORT="${MYSQL_PORT:-53306}"
MYSQL_ADMIN_CMD="${MYSQL_ADMIN_CMD:-}"
DB_NAME="$(printf '%s' "$RUN_ID" | tr '-' '_')"
DB_USER="rentops_audit_$(openssl rand -hex 6)"
DB_PASSWORD="$(openssl rand -hex 16)"
PRIMARY_USER="${RUN_ID}-primary"
PRIMARY_PASSWORD="$(openssl rand -hex 12)"
SECOND_USER="${RUN_ID}-second"
SECOND_PASSWORD="$(openssl rand -hex 12)"

TOKEN_KEY="$(openssl rand -hex 16)"
RUNTIME_DIR="$(mktemp -d "${TMPDIR:-/tmp}/rentops-audit-runtime-XXXXXX")"
APP_LOG="${RUNTIME_DIR}/app.log"
APP_PID=""

# A named guard so cleanup knows whether it may drop the database. It stays 0
# until the seeder has finished, so a failure keeps the database for diagnosis.
TEARDOWN_DB=0

# Refuse anything that is not this script's own disposable instance.
if [[ "$DB_NAME" != rentops_audit_* ]]; then
	echo "refusing to run: database name ${DB_NAME} is outside the disposable namespace" >&2
	exit 2
fi
if [[ "$APP_PORT" == "8081" || "$BASE_URL" == *bank.ddpl.top* ]]; then
	echo "refusing to run: the audit must not target the production instance (${BASE_URL})" >&2
	exit 2
fi
case "$MYSQL_HOST" in
	127.0.0.1 | localhost | ::1) ;;
	*)
		echo "refusing to run: MYSQL_HOST must be loopback, got ${MYSQL_HOST}" >&2
		exit 2
		;;
esac

cleanup() {
	if [[ -n "$APP_PID" ]] && kill -0 "$APP_PID" 2>/dev/null; then
		kill "$APP_PID" 2>/dev/null || true
		wait "$APP_PID" 2>/dev/null || true
	fi
	if [[ "$TEARDOWN_DB" != "1" || "${AUDIT_KEEP:-0}" == "1" ]]; then
		echo "" >&2
		echo "leftover disposable database: ${DB_NAME}" >&2
		echo "leftover runtime directory:   ${RUNTIME_DIR}" >&2
		echo "drop it with: ${ADMIN_INVOCATION_HINT:-mysql} -e \"DROP DATABASE \\\`${DB_NAME}\\\`; DROP USER IF EXISTS '${DB_USER}'@'${MYSQL_HOST}';\"" >&2
		return
	fi
	echo "==> dropping disposable database ${DB_NAME}" >&2
	mysql_admin "DROP DATABASE \`${DB_NAME}\`;
DROP USER IF EXISTS '${DB_USER}'@'${MYSQL_HOST}';" || true
	echo "==> runtime artifacts kept at ${RUNTIME_DIR}" >&2
}
trap cleanup EXIT

# Resolve an admin mysql client. Order: explicit MYSQL_ADMIN_CMD, a passwordless
# root over TCP, then passwordless sudo mysql (the E2E precedent). Failing all
# three is a loud error rather than a guess.
ADMIN_INVOCATION_HINT=""
mysql_admin() {
	local sql="$1"
	# shellcheck disable=SC2086
	$MYSQL_ADMIN_RESOLVED --batch --skip-column-names -e "$sql"
}

resolve_mysql_admin() {
	if [[ -n "$MYSQL_ADMIN_CMD" ]]; then
		MYSQL_ADMIN_RESOLVED="$MYSQL_ADMIN_CMD"
		ADMIN_INVOCATION_HINT="$MYSQL_ADMIN_CMD"
		return
	fi
	if mysql --batch --skip-column-names -h "$MYSQL_HOST" -P "$MYSQL_PORT" -u root -e "SELECT 1" >/dev/null 2>&1; then
		MYSQL_ADMIN_RESOLVED="mysql -h ${MYSQL_HOST} -P ${MYSQL_PORT} -u root"
		ADMIN_INVOCATION_HINT="mysql -h ${MYSQL_HOST} -P ${MYSQL_PORT} -u root"
		return
	fi
	if sudo -n true >/dev/null 2>&1 && sudo -n mysql -e "SELECT 1" >/dev/null 2>&1; then
		MYSQL_ADMIN_RESOLVED="sudo mysql"
		ADMIN_INVOCATION_HINT="sudo mysql"
		return
	fi
	echo "unable to find an admin mysql client on ${MYSQL_HOST}:${MYSQL_PORT}." >&2
	echo "Set MYSQL_ADMIN_CMD, e.g. MYSQL_ADMIN_CMD='mysql -h 127.0.0.1 -P 3306 -u root' $0" >&2
	exit 2
}
resolve_mysql_admin

echo "==> creating disposable database ${DB_NAME}"
mysql_admin "CREATE DATABASE \`${DB_NAME}\` CHARACTER SET utf8mb4 COLLATE utf8mb4_unicode_ci;
CREATE USER '${DB_USER}'@'${MYSQL_HOST}' IDENTIFIED BY '${DB_PASSWORD}';
GRANT ALL PRIVILEGES ON \`${DB_NAME}\`.* TO '${DB_USER}'@'${MYSQL_HOST}';
FLUSH PRIVILEGES;"

DSN="${DB_USER}:${DB_PASSWORD}@tcp(${MYSQL_HOST}:${MYSQL_PORT})/${DB_NAME}?charset=utf8mb4&parseTime=True&loc=UTC"

start_app() {
	local admin_user="$1"
	local admin_password="$2"
	local log_file="$3"

	TL_ADDR="${APP_HOST}:${APP_PORT}" \
	TL_ENV=sandbox \
	TL_CLIENT_ID=audit-local-client \
	TL_CLIENT_SECRET=audit-local-secret \
	TL_REDIRECT_URI="${BASE_URL}/callback" \
	TL_LOG_FILE="${RUNTIME_DIR}/bank-results.jsonl" \
	TL_TOKEN_FILE="${RUNTIME_DIR}/token.json" \
	MYSQL_DSN="$DSN" \
	MIGRATIONS_DIR="${REPO_ROOT}/migrations" \
	BANK_TOKEN_ENCRYPTION_KEY="$TOKEN_KEY" \
	APP_SESSION_SECRET="audit-local-session-secret" \
	APP_ADMIN_USERNAME="$admin_user" \
	APP_ADMIN_PASSWORD="$admin_password" \
		"${RUNTIME_DIR}/truelayer-demo" >>"$log_file" 2>&1 &
	APP_PID=$!

	for _ in $(seq 1 100); do
		if ! kill -0 "$APP_PID" 2>/dev/null; then
			echo "application exited during startup; see ${log_file}" >&2
			tail -20 "$log_file" >&2 || true
			exit 1
		fi
		local status
		status="$(curl -s -o /dev/null -w '%{http_code}' "${BASE_URL}/login-local" || true)"
		if [[ "$status" == "405" || "$status" == "200" ]]; then
			return 0
		fi
		sleep 0.2
	done
	echo "application did not become ready on ${BASE_URL}; see ${log_file}" >&2
	exit 1
}

stop_app() {
	if [[ -n "$APP_PID" ]] && kill -0 "$APP_PID" 2>/dev/null; then
		kill "$APP_PID" 2>/dev/null || true
		wait "$APP_PID" 2>/dev/null || true
	fi
	APP_PID=""
}

echo "==> building the application"
cd "$REPO_ROOT"
go build -o "${RUNTIME_DIR}/truelayer-demo" ./cmd/truelayer-demo

echo "==> seeding the second, empty account (${SECOND_USER}) for the empty-list branch"
start_app "$SECOND_USER" "$SECOND_PASSWORD" "${RUNTIME_DIR}/seed-second.log"
stop_app

echo "==> starting the application with the audit account (${PRIMARY_USER})"
start_app "$PRIMARY_USER" "$PRIMARY_PASSWORD" "$APP_LOG"

echo "==> seeding room, tenant and rent-plan data through the app"
AUDIT_BASE="$BASE_URL" AUDIT_USER="$PRIMARY_USER" AUDIT_PASS="$PRIMARY_PASSWORD" AUDIT_RUN_ID="$RUN_ID" \
	node "${REPO_ROOT}/scripts/audit/seed-workspace.mjs"

TEARDOWN_DB=1

cat <<BANNER

================================================================================
  Audit instance is running and populated.
================================================================================
  Base URL : ${BASE_URL}
  Account  : ${PRIMARY_USER}
  Password : ${PRIMARY_PASSWORD}

  Empty account (for the empty-list branch):
  Account  : ${SECOND_USER}
  Password : ${SECOND_PASSWORD}

  Run the harness from another terminal, e.g.:
    AUDIT_BASE=${BASE_URL} AUDIT_USER='${PRIMARY_USER}' AUDIT_PASS='${PRIMARY_PASSWORD}' \\
      node scripts/audit/audit.mjs

  Press Ctrl-C to stop the app and drop the disposable database.

  Application log: ${APP_LOG}
================================================================================

BANNER

# Block until interrupted; the EXIT trap tears everything down.
wait "$APP_PID"
