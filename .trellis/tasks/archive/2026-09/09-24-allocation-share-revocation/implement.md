# Implementation

- [x] Add a distinct action kind and exact-share revoke service with owner scope, source/obligation locking, idempotency, and audit.
- [x] Add POST handler, error mapping, and safe return behavior.
- [x] Add allocation ID and exact share fields to evidence data.
- [x] Test split source, same-tenant two-month source, last-share source, stale/foreign/non-rent rows, repeated request, and queue eligibility.

Validation: focused MySQL transaction tests plus `go test ./...` and `go vet ./...`. Inspect action/projection code before editing because it contains unrelated uncommitted work.
