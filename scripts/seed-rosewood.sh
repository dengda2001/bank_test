#!/usr/bin/env bash
# The Rosewood seed targets the pre-014 tenancy schema and is retired.
# Use scripts/run-audit-local.sh to create a disposable room-rent-plan fixture.

set -euo pipefail

cat >&2 <<'MESSAGE'
This seed targets the retired tenancy schema and is disabled after migration 014.
It cannot be applied to the room-rent-plan model.

For a current disposable fixture, use:
  scripts/run-audit-local.sh
MESSAGE
exit 2
