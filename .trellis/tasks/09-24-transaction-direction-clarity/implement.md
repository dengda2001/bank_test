# Implementation plan: transaction direction clarity

1. Move the existing `direction` select into the always-visible quick filter and preserve it as a hidden value in the advanced form. Keep exactly one visible direction control.
2. Render income/expense text and distinct subtle backgrounds in table rows and mobile cards using the existing direction field. Preserve hover, sticky action cell, and mobile touch targets.
3. Update existing render tests for selector placement and direction labels. Run focused transaction page tests, `go test ./cmd/truelayer-demo`, and browser checks at wide and narrow widths.

## Rollback point

This is a presentation-only change. Revert the template and render assertions together if table width or mobile geometry regresses.
