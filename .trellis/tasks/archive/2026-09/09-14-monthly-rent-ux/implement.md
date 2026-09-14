# Implementation plan draft

Status: product decision confirmed; pending final artifact review and implementation authorization.

1. Automatic month allocation policy resolved: user selects unclear months, preserving tenant association. Review final plan before task activation. Keep one integrated task because payer association and monthly dashboard share the accounting acceptance criteria; implement and verify in the slices below.
2. Load `trellis-before-dev` and relevant backend guidelines; inspect schema migration runner, auth routes, all template/control implementations and testing infrastructure.
3. Implement durable payer relationships, transaction tenant association and historical/future reconciliation together. Add meaningful regression tests for missing IDs, partial batches, conflicts, duplicate submission, isolation, currency and month boundaries.
4. Implement rent-month dashboard metrics, pending-transaction counts and links, default landing route, and unsettled-first tenant presentation. Test accounting distinctions and empty states.
5. Implement shared rounded month/date/select controls and Chinese application copy; apply to dashboard, billing, tenants, expenses and login. Preserve bank-provided values.
6. Verify templates and controls in a real browser, including month navigation, dropdown keyboard/focus behaviour, tenant association batch feedback, and mobile layout. Use isolated test data only.
7. Run `go test ./...`, `go vet ./...` and Trellis quality checks; review migration and reconciliation invariants. Record evidence and outstanding limitations.
8. Update documentation/specs affected by finalized contracts and complete Trellis handoff.

## Risk and rollback checkpoints

- Highest risk: matching.go month selection, matching_service.go allocation/reconciliation and additive schema changes. Review independently from cosmetic changes.
- Read fresh balances during batch processing and lock/check balances atomically during allocation.
- Do not overwrite existing confirmed payments or trigger real bank refresh during verification.
- Reverting UI changes is independent of preserving new financial allocations; do not reverse data by deleting records.
