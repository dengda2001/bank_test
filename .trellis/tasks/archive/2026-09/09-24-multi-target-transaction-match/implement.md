# Implementation plan

1. Extend review read model with current effective allocations, machine-readable month balances, and explicit-month lookup while retaining historical evidence.
2. Add an authenticated batch rent confirmation handler and service orchestration around the existing atomic allocation path, with request idempotency and optional single payer relationship.
3. Replace one-target drawer form with month-card add actions, persistent draft list, amount editing, totals, and one final submit. Preserve tenant lookup and payer history pagination while drafts are open.
4. Add responsive styling and update status/action copy for partial allocations.
5. Add focused Go tests for multi-tenant, same-tenant cross-month, partial continuation, over-budget rollback, ownership, and rendered controls. Verify browser interaction at desktop and mobile widths.
6. Run relevant tests and Trellis quality check, then review diff without disturbing unrelated worktree changes.
