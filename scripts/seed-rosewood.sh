#!/usr/bin/env bash
# Seed the local RentOps database with the Rosewood rent ledger.
#
# Source data: 收租明细_Rosewood_20260916.xlsx (repo root), 4 Dublin properties,
# 25 rooms, 8 months (2026-05 .. 2026-12). test-data/rosewood/extract.py turns
# that workbook into normalized JSON fixtures plus two generated SQL files; this
# script is the runner. It is deliberately bash + the mysql CLI only — there is
# no Node/npm step, matching scripts/run-audit-local.sh.
#
# Two stages, both idempotent (safe to re-run):
#
#   --stage=data   properties / rooms / tenants / tenancy_agreements /
#                  agreement_parties / tenant_payers, and the bank feed as
#                  unmatched payment_transactions.
#
#   --stage=match  the lazy rent obligations the app would create on first page
#                  load, the confirmation of the pre-matched bank transactions,
#                  and the backfill of rent_obligations.paid_amount_cents/status
#                  from those confirmed allocations. The backfill runs in the
#                  same transaction as the allocations and is the whole point of
#                  the stage: the dashboard sums the stored column without
#                  projecting, so without it 已收 reads 0
#                  (design.md §6.1, migrations/013_dedupe_rent_obligations.sql:106-136).
#
#   --stage=all    data, then match (the default).
#
# Safety, mirroring scripts/run-audit-local.sh:
#   * the target host must be loopback;
#   * the port must not be 8081 and the database must not be a remote one —
#     :8081 / bank.ddpl.top is the production instance against the production
#     database (scripts/audit/README.md:106-108);
#   * the database name must stay inside the `rentops*` namespace;
#   * all DELETEs are scoped inside the generated SQL to the four known addresses,
#     the normalized name set and the `ROSEWOOD-` reference prefix; this script
#     refuses to run a file that has lost those markers.
#
# Options (env var in parentheses):
#   --stage=data|match|all      (SEED_STAGE, default all)
#   --mysql-host=HOST           (MYSQL_HOST, default 127.0.0.1)
#   --mysql-port=PORT           (MYSQL_PORT, default 3306)
#   --mysql-user=USER           (MYSQL_USER, default root)
#   --database=NAME             (RENTOPS_DB, default rentops)
#   --uid=N                     (RENTOPS_UID, default: the first row of users)
#   -h, --help
#
# The mysql password, if any, is read from MYSQL_PWD or ~/.my.cnf by the client
# itself; it is never passed on the command line and never printed.

set -euo pipefail

REPO_ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
FIXTURE_DIR="${REPO_ROOT}/test-data/rosewood"
DATA_SQL="${FIXTURE_DIR}/seed-data.sql"
MATCH_SQL="${FIXTURE_DIR}/seed-match.sql"

STAGE="${SEED_STAGE:-all}"
MYSQL_HOST="${MYSQL_HOST:-127.0.0.1}"
MYSQL_PORT="${MYSQL_PORT:-3306}"
MYSQL_USER="${MYSQL_USER:-root}"
RENTOPS_DB="${RENTOPS_DB:-rentops}"
RENTOPS_UID="${RENTOPS_UID:-}"

usage() {
	# Print the header comment block: from line 2 up to the first real statement.
	awk 'NR == 1 { next } /^#/ { sub(/^# ?/, ""); print; next } { exit }' "${BASH_SOURCE[0]}"
	exit 0
}

for arg in "$@"; do
	case "$arg" in
		--stage=*) STAGE="${arg#*=}" ;;
		--mysql-host=*) MYSQL_HOST="${arg#*=}" ;;
		--mysql-port=*) MYSQL_PORT="${arg#*=}" ;;
		--mysql-user=*) MYSQL_USER="${arg#*=}" ;;
		--database=*) RENTOPS_DB="${arg#*=}" ;;
		--uid=*) RENTOPS_UID="${arg#*=}" ;;
		-h | --help) usage ;;
		*)
			echo "unknown argument: ${arg}" >&2
			echo "run with --help for the option list" >&2
			exit 2
			;;
	esac
done

case "$STAGE" in
	data | match | all) ;;
	*)
		echo "refusing to run: --stage must be data, match or all, got '${STAGE}'" >&2
		exit 2
		;;
esac

# --- Safety guards -----------------------------------------------------------

if [[ "$MYSQL_PORT" == "8081" ]]; then
	echo "refusing to run: port 8081 is the production instance" >&2
	exit 2
fi
case "$MYSQL_HOST" in
	127.0.0.1 | localhost | ::1) ;;
	*)
		echo "refusing to run: MYSQL_HOST must be loopback, got ${MYSQL_HOST}" >&2
		echo "only the local database may be seeded (prd.md R7)." >&2
		exit 2
		;;
esac
case "$RENTOPS_DB" in
	rentops | rentops_*) ;;
	*)
		echo "refusing to run: database '${RENTOPS_DB}' is outside the rentops namespace" >&2
		exit 2
		;;
esac

if ! command -v mysql >/dev/null 2>&1; then
	echo "refusing to run: the mysql client is not on PATH" >&2
	exit 2
fi

for stage_file in "$DATA_SQL" "$MATCH_SQL"; do
	if [[ ! -s "$stage_file" ]]; then
		echo "refusing to run: ${stage_file} is missing or empty." >&2
		echo "regenerate it with: python3 test-data/rosewood/extract.py" >&2
		exit 2
	fi
done

# The generated files scope every DELETE to the four addresses, the normalized
# name set and the ROSE-WOOD- prefix. If those markers are gone the file was
# replaced by something less careful, and this script must not run it.
if ! grep -q "rosewood:" "$DATA_SQL" || ! grep -q "rosewood:" "$MATCH_SQL"; then
	echo "refusing to run: a generated SQL file no longer carries the 'rosewood:' scope marker." >&2
	echo "a stale or hand-edited file could delete data outside the Rosewood set." >&2
	exit 2
fi

MYSQL_CMD=(mysql -h "$MYSQL_HOST" -P "$MYSQL_PORT" -u "$MYSQL_USER")

# --- Helpers -----------------------------------------------------------------

mysql_query() {
	"${MYSQL_CMD[@]}" --batch --skip-column-names "$RENTOPS_DB" -e "$1"
}

mysql_report() {
	"${MYSQL_CMD[@]}" "$RENTOPS_DB" -e "$1"
}

run_sql_file() {
	local file="$1"
	"${MYSQL_CMD[@]}" --batch --init-command="SET @uid=${UID_VALUE}" "$RENTOPS_DB" <"$file"
}

# --- Resolve the acting user -------------------------------------------------

if ! mysql_query "SELECT 1" >/dev/null 2>&1; then
	echo "cannot connect to ${MYSQL_USER}@${MYSQL_HOST}:${MYSQL_PORT}/${RENTOPS_DB}." >&2
	echo "check that the local MySQL is running and the database exists." >&2
	exit 2
fi

if [[ -z "$RENTOPS_UID" ]]; then
	RENTOPS_UID="$(mysql_query "SELECT id FROM users ORDER BY id LIMIT 1")"
fi
if [[ ! "$RENTOPS_UID" =~ ^[0-9]+$ ]]; then
	echo "refusing to run: --uid must be a numeric user id, got '${RENTOPS_UID}'" >&2
	exit 2
fi
UID_VALUE="$RENTOPS_UID"

if [[ "$(mysql_query "SELECT COUNT(*) FROM users WHERE id = ${UID_VALUE}")" != "1" ]]; then
	echo "refusing to run: no user with id ${UID_VALUE} in ${RENTOPS_DB}.users" >&2
	exit 2
fi

UID_NAME="$(mysql_query "SELECT username FROM users WHERE id = ${UID_VALUE}")"

echo "==> seeding ${RENTOPS_DB} on ${MYSQL_HOST}:${MYSQL_PORT} as user ${UID_VALUE} (${UID_NAME})"
echo "    stage: ${STAGE}"

# --- Preflight: migration 013 -------------------------------------------------

# Stage match leans on INSERT IGNORE against the unique key that migration 013
# adds. It also carries a NOT EXISTS guard, so it is safe without the key, but
# the app itself cannot stay deduplicated until 013 has run. Start the app once
# (runMigrations applies it at db.go:39) if this warns.
if ! mysql_query "
	SELECT COUNT(*) FROM information_schema.statistics
	 WHERE table_schema = '${RENTOPS_DB}'
	   AND table_name = 'rent_obligations'
	   AND index_name = 'idx_rent_obligations_lazy_tenant_period'" | grep -q '^[1-9]'; then
	echo "warning: migration 013 is not applied to ${RENTOPS_DB}." >&2
	echo "         rent_obligations is missing idx_rent_obligations_lazy_tenant_period," >&2
	echo "         so the app will re-create duplicate obligations on every page load." >&2
	echo "         start the app once (it runs migrations on startup), then re-run this." >&2
fi

# --- Stage data ---------------------------------------------------------------

if [[ "$STAGE" == "data" || "$STAGE" == "all" ]]; then
	echo "==> [data] leasing structure and bank feed"
	run_sql_file "$DATA_SQL"
fi

# --- Stage match --------------------------------------------------------------

if [[ "$STAGE" == "match" || "$STAGE" == "all" ]]; then
	echo "==> [match] obligations, confirmed allocations, paid/status backfill"
	run_sql_file "$MATCH_SQL"
fi

# --- Summary ------------------------------------------------------------------

echo "==> row counts (Rosewood rows only)"
mysql_report "
SELECT 'properties' AS table_name, COUNT(*) AS n FROM properties
  WHERE user_id = ${UID_VALUE} AND name IN ('116 Kimmage Rd W Dublin 12', '78 Old County Road Dublin 12', '72 Walkinstown Rd Dublin 12', '169 Windmill Park Dublin 12')
UNION ALL SELECT 'rooms', COUNT(*) FROM rooms r JOIN properties p ON p.id = r.property_id
  WHERE r.user_id = ${UID_VALUE} AND p.name IN ('116 Kimmage Rd W Dublin 12', '78 Old County Road Dublin 12', '72 Walkinstown Rd Dublin 12', '169 Windmill Park Dublin 12')
UNION ALL SELECT 'tenants', COUNT(*) FROM tenants
  WHERE user_id = ${UID_VALUE} AND room_address IN ('116 Kimmage Rd W Dublin 12', '78 Old County Road Dublin 12', '72 Walkinstown Rd Dublin 12', '169 Windmill Park Dublin 12')
UNION ALL SELECT 'tenancy_agreements', COUNT(*) FROM tenancy_agreements a JOIN rooms r ON r.id = a.room_id JOIN properties p ON p.id = r.property_id
  WHERE a.user_id = ${UID_VALUE} AND p.name IN ('116 Kimmage Rd W Dublin 12', '78 Old County Road Dublin 12', '72 Walkinstown Rd Dublin 12', '169 Windmill Park Dublin 12')
UNION ALL SELECT 'agreement_parties', COUNT(*) FROM agreement_parties ap JOIN tenancy_agreements a ON a.id = ap.agreement_id JOIN rooms r ON r.id = a.room_id JOIN properties p ON p.id = r.property_id
  WHERE ap.user_id = ${UID_VALUE} AND p.name IN ('116 Kimmage Rd W Dublin 12', '78 Old County Road Dublin 12', '72 Walkinstown Rd Dublin 12', '169 Windmill Park Dublin 12')
UNION ALL SELECT 'tenant_payers', COUNT(*) FROM tenant_payers tp JOIN tenants t ON t.id = tp.tenant_id
  WHERE tp.user_id = ${UID_VALUE} AND t.room_address IN ('116 Kimmage Rd W Dublin 12', '78 Old County Road Dublin 12', '72 Walkinstown Rd Dublin 12', '169 Windmill Park Dublin 12')
UNION ALL SELECT 'payment_transactions', COUNT(*) FROM payment_transactions
  WHERE user_id = ${UID_VALUE} AND stable_transaction_key LIKE 'rosewood:%';"

if [[ "$STAGE" == "match" || "$STAGE" == "all" ]]; then
	echo "==> obligations: one per (tenant, month), no duplicates"
	mysql_report "
SELECT COUNT(*) AS obligations,
       SUM(o.status = 'paid') AS paid,
       SUM(o.status = 'partial') AS partial,
       SUM(o.status = 'overdue') AS overdue,
       SUM(o.status = 'open') AS open
  FROM rent_obligations o JOIN tenants t ON t.id = o.tenant_id
 WHERE o.user_id = ${UID_VALUE} AND o.rent_charge_id IS NULL
   AND t.room_address IN ('116 Kimmage Rd W Dublin 12', '78 Old County Road Dublin 12', '72 Walkinstown Rd Dublin 12', '169 Windmill Park Dublin 12');"

	mysql_report "
SELECT 'duplicate (tenant, month) groups' AS check_name, COUNT(*) AS n FROM (
  SELECT o.user_id, o.tenant_id, o.period_month
    FROM rent_obligations o JOIN tenants t ON t.id = o.tenant_id
   WHERE o.user_id = ${UID_VALUE} AND o.rent_charge_id IS NULL
     AND t.room_address IN ('116 Kimmage Rd W Dublin 12', '78 Old County Road Dublin 12', '72 Walkinstown Rd Dublin 12', '169 Windmill Park Dublin 12')
   GROUP BY 1, 2, 3 HAVING COUNT(*) > 1) d;"

	echo "done. Open /rent-dashboard as ${UID_NAME} to see the months; the first"
	echo "page load triggers the app's own lazy obligation pass on top of this seed."
fi
