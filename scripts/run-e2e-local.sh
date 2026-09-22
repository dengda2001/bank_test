#!/usr/bin/env bash
# Run the real-HTTP E2E acceptance suite against a disposable local environment.
#
# The script creates a throwaway MySQL database, seeds two isolated accounts
# through the application's own startup seeding, starts the app against only
# that database, runs the room-rent E2E flow, and then drops the database again.
#
# It never touches an existing database: every created identifier is derived
# from the run ID and must match the `rentops_e2e_*` prefix or the script stops.
#
set -euo pipefail

REPO_ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
APP_PORT="${APP_PORT:-18080}"
BASE_URL="http://127.0.0.1:${APP_PORT}"
TARGET_NAME="rentops-e2e-local"

RUN_ID="rentops-e2e-$(date -u +%Y%m%d-%H%M%S)-$(openssl rand -hex 4)"
MYSQL_HOST="${MYSQL_HOST:-127.0.0.1}"
MYSQL_PORT="${MYSQL_PORT:-53306}"
MYSQL_ADMIN_CMD="${MYSQL_ADMIN_CMD:-}"
DB_NAME="$(printf '%s' "$RUN_ID" | tr '-' '_')"
DB_USER="rentops_e2e_$(openssl rand -hex 6)"
DB_PASSWORD="$(openssl rand -hex 16)"
PRIMARY_USER="${RUN_ID}-primary"
PRIMARY_PASSWORD="$(openssl rand -hex 12)"
SECOND_USER="${RUN_ID}-second"
SECOND_PASSWORD="$(openssl rand -hex 12)"

TOKEN_KEY="$(openssl rand -hex 16)"
RUNTIME_DIR="$(mktemp -d "${TMPDIR:-/tmp}/rentops-e2e-runtime-XXXXXX")"
REPORT_PATH="${RUNTIME_DIR}/report.json"
APP_LOG="${RUNTIME_DIR}/app.log"
APP_PID=""

if [[ "$DB_NAME" != rentops_e2e_* ]]; then
	echo "refusing to run: database name ${DB_NAME} is outside the disposable namespace" >&2
	exit 2
fi
case "$MYSQL_HOST" in
	127.0.0.1 | localhost | ::1) ;;
	*)
		echo "refusing to run: MYSQL_HOST must be loopback, got ${MYSQL_HOST}" >&2
		exit 2
		;;
esac

DATABASE_DROPPED=0

stop_app() {
	if [[ -n "$APP_PID" ]] && kill -0 "$APP_PID" 2>/dev/null; then
		kill "$APP_PID" 2>/dev/null || true
		wait "$APP_PID" 2>/dev/null || true
	fi
	APP_PID=""
}

cleanup() {
	stop_app
	if [[ "$DATABASE_DROPPED" != "1" ]]; then
		echo "leftover disposable database: ${DB_NAME} (drop with: sudo mysql -e \"DROP DATABASE \\\`${DB_NAME}\\\`; DROP USER IF EXISTS '${DB_USER}'@'${MYSQL_HOST}';\")" >&2
	fi
}
trap cleanup EXIT

mysql_root() {
	# shellcheck disable=SC2086
	$MYSQL_ADMIN_RESOLVED --batch --skip-column-names -e "$1"
}

resolve_mysql_admin() {
	if [[ -n "$MYSQL_ADMIN_CMD" ]]; then
		MYSQL_ADMIN_RESOLVED="$MYSQL_ADMIN_CMD"
		return
	fi
	if mysql --batch --skip-column-names -h "$MYSQL_HOST" -P "$MYSQL_PORT" -u root -e "SELECT 1" >/dev/null 2>&1; then
		MYSQL_ADMIN_RESOLVED="mysql -h ${MYSQL_HOST} -P ${MYSQL_PORT} -u root"
		return
	fi
	if sudo -n true >/dev/null 2>&1 && sudo -n mysql -e "SELECT 1" >/dev/null 2>&1; then
		MYSQL_ADMIN_RESOLVED="sudo mysql"
		return
	fi
	echo "unable to find an admin mysql client on ${MYSQL_HOST}:${MYSQL_PORT}." >&2
	echo "Set MYSQL_ADMIN_CMD, e.g. MYSQL_ADMIN_CMD='mysql -h 127.0.0.1 -P 53306 -u root' $0" >&2
	exit 2
}

resolve_mysql_admin

# The database name is generated above from the run ID, never taken from input,
# so the interpolation below cannot carry an operator-supplied identifier.
echo "==> creating disposable database ${DB_NAME}"
mysql_root "CREATE DATABASE \`${DB_NAME}\` CHARACTER SET utf8mb4 COLLATE utf8mb4_unicode_ci;
CREATE USER '${DB_USER}'@'${MYSQL_HOST}' IDENTIFIED BY '${DB_PASSWORD}';
GRANT ALL PRIVILEGES ON \`${DB_NAME}\`.* TO '${DB_USER}'@'${MYSQL_HOST}';
FLUSH PRIVILEGES;"

DSN="${DB_USER}:${DB_PASSWORD}@tcp(${MYSQL_HOST}:${MYSQL_PORT})/${DB_NAME}?charset=utf8mb4&parseTime=True&loc=UTC"

start_app() {
	local admin_user="$1"
	local admin_password="$2"
	local log_file="$3"

	TL_ADDR="127.0.0.1:${APP_PORT}" \
	TL_ENV=sandbox \
	TL_CLIENT_ID=e2e-local-client \
	TL_CLIENT_SECRET=e2e-local-secret \
	TL_REDIRECT_URI="${BASE_URL}/callback" \
	TL_LOG_FILE="${RUNTIME_DIR}/bank-results.jsonl" \
	TL_TOKEN_FILE="${RUNTIME_DIR}/token.json" \
	MYSQL_DSN="$DSN" \
	MIGRATIONS_DIR="${REPO_ROOT}/migrations" \
	BANK_TOKEN_ENCRYPTION_KEY="$TOKEN_KEY" \
	APP_SESSION_SECRET="e2e-local-session-secret" \
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

echo "==> building the application and the runner"
cd "$REPO_ROOT"
go build -o "${RUNTIME_DIR}/truelayer-demo" ./cmd/truelayer-demo
go build -o "${RUNTIME_DIR}/rentops-e2e" ./cmd/rentops-e2e

echo "==> seeding the second isolated account (${SECOND_USER})"
start_app "$SECOND_USER" "$SECOND_PASSWORD" "${RUNTIME_DIR}/seed-second.log"
stop_app

echo "==> starting the application with the primary account (${PRIMARY_USER})"
start_app "$PRIMARY_USER" "$PRIMARY_PASSWORD" "$APP_LOG"

echo "==> running the E2E acceptance suite"
RENTOPS_E2E_BASE_URL="$BASE_URL" \
RENTOPS_E2E_RUN_ID="$RUN_ID" \
RENTOPS_E2E_REPORT_PATH="$REPORT_PATH" \
RENTOPS_E2E_TARGET_NAME="$TARGET_NAME" \
RENTOPS_E2E_TARGET_ALLOWLIST="$TARGET_NAME" \
RENTOPS_E2E_DATABASE_ALLOWLIST="$DB_NAME" \
RENTOPS_E2E_MYSQL_DSN="$DSN" \
RENTOPS_E2E_USERNAME="$PRIMARY_USER" \
RENTOPS_E2E_PASSWORD="$PRIMARY_PASSWORD" \
RENTOPS_E2E_SECOND_USERNAME="$SECOND_USER" \
RENTOPS_E2E_SECOND_PASSWORD="$SECOND_PASSWORD" \
RENTOPS_E2E_CONFIRM_WRITES=I_UNDERSTAND_NON_PRODUCTION \
RENTOPS_E2E_CONFIRM_CLEANUP=I_UNDERSTAND_DELETE_RUN_ID_ONLY \
	"${RUNTIME_DIR}/rentops-e2e" -execute -report "$REPORT_PATH"

echo "==> report: ${REPORT_PATH}"
stop_app

STATUS="$(python3 -c 'import json,sys;print(json.load(open(sys.argv[1]))["status"])' "$REPORT_PATH")"
echo "==> run status: ${STATUS}"

if [[ "$STATUS" != "passed" ]]; then
	echo "run did not pass; keeping ${DB_NAME} and ${RUNTIME_DIR} for diagnosis" >&2
	exit 1
fi

if [[ "${KEEP_DATABASE:-0}" == "1" ]]; then
	echo "==> KEEP_DATABASE=1: leaving ${DB_NAME} and ${RUNTIME_DIR} in place"
	exit 0
fi

echo "==> dropping disposable database ${DB_NAME}"
mysql_root "DROP DATABASE \`${DB_NAME}\`;
DROP USER IF EXISTS '${DB_USER}'@'${MYSQL_HOST}';"
DATABASE_DROPPED=1
echo "==> run artifacts: ${RUNTIME_DIR}"
