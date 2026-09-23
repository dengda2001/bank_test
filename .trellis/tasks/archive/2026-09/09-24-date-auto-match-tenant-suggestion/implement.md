# Implementation

1. Add narrow unit tests for date boundaries, explicit-month priority, exact/remembered identity, conflicting identities, currency/amount, non-rent descriptions, and fuzzy suggestion ranking.
2. Extend strict matching and reconciliation while preserving allocated/deferred transaction guards and current/past fact materialization.
3. Derive the allocation method label; render it on desktop/mobile list, transaction detail, and review drawer.
4. Add manual-only tenant suggestions to the review data and template, keeping full tenant search.
5. Run `gofmt`, focused tests, `go test ./... -count=1`, `go vet ./...`, and `git diff --check`. Run disposable MySQL integration coverage if a test DSN is available. Inspect the final diff for unrelated dirty work before staging.

Review gate: user explicitly asked to implement the described automatic matching and manual suggestion behavior. The only previously open decision is resolved by preserving explicit-month priority and existing explicit-month matching behavior.
