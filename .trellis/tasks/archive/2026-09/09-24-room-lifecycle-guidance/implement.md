# Implementation plan: room succession and asset guidance

1. Add regression tests for replacing a room plan from September without changing the prior tenant's July/August facts.
2. Give zero monthly rent a dedicated error code and readable message; retain the existing backend positive-rent validation.
3. Add property/room soft-delete schema and owner-scoped filtering; guard it against still-effective room plans and guide end-occupancy first. Preserve historical lookups and child-room visibility rules.
4. Clarify succession, vacancy, soft-delete confirmation, and zero-rent text.
5. Run focused room-plan and soft-deletion tests, `go test ./cmd/truelayer-demo`, and mobile drawer review.
6. If local MySQL is reachable, inspect Arslan's room plan and report the live September state; do not mutate records without a separate reviewed correction.

Rollback point: do not hard-delete assets or financial rows. The additive deletion timestamp may remain if the new UI is disabled; preserve a reader for already soft-deleted assets.
