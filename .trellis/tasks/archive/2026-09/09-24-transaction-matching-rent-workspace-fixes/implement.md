# Integration implementation plan

1. Review parent/child PRDs and designs with the user; resolve remaining product decisions. Do not start implementation before the Trellis review gate.
2. Start and complete the prepayment ledger child first: migration, ledger/service tests, manual credit application, audit-safe revocation. Check its contract before exposing the matching control.
3. Start the matching-review child: detail overlay, error feedback, lookup and month evidence, payer defaults, and integration with explicit prepayment.
4. Complete the independent list/dashboard, invoice-upload, and room-lifecycle children one at a time. For each, load relevant specs, implement, run focused and package tests, then run the Trellis quality check.
5. Integrate: compare all parent acceptance criteria with rendered pages and service behavior. Run `go test ./cmd/truelayer-demo`, `go test ./...`, `go vet ./...`, and `git diff --check`. Run disposable MySQL tests if their DSN becomes available and browser checks at 1440px, 1024px, and 390px.
6. Query IE26090426926842, IE26083165993723, and Arslan Arshad read-only when local MySQL becomes reachable. Record exact findings; any data correction needs a separate reviewed action.
7. Review spec updates, stage only task-owned edits, and follow the Trellis commit approval gate before wrap-up.

Rollback points: each UI child is reversible independently. Preserve additive migrations and all audit data if new actions are disabled. Never roll back by deleting prepayment, allocation, invoice, or rent-history rows.
