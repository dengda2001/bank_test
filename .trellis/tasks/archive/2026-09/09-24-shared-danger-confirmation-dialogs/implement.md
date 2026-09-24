# Implementation

- [x] Add shared red dialog CSS/JS to the workspace shell, with declarative submit and imperative confirmation APIs.
- [x] Replace shell and bills/dunning native confirm handlers; add target/action copy to existing marked buttons.
- [x] Add confirmation metadata to payer-relation removal and end-occupancy submissions.
- [x] Verify Escape, cancel, focus return, validation, one-shot submit, external `form=`, and `formaction` semantics.
- [x] Check all runtime workspace sources for remaining native `confirm()`/`alert()` use; leave existing detailed confirmation pages as their own second step.

Validation: focused render/JS checks and real desktop/mobile browser walkthroughs. The shared shell and `rent_collection_pages.go` affect every page, so inspect all diff hunks and rerun `go test ./...` before integration.
