# Audit and Repair Plan

## Sequence

1. [x] Recheck the reviewed planning documents and activate the task before auditing or changing product code.
2. [x] Finalize the prototype-state inventory and map desktop/mobile screens to routes/states, including switchers, drawers, forms, and drill-down states.
3. [x] Read responsive conventions and verify the audit runner uses loopback plus a temporary `rentops_audit_*` database.
4. [x] Browse prototype routes at representative desktop/mobile sizes and save URLs, screenshots, and geometry.
5. [x] Compare every prototype state; classify missing pages/actions, data/state differences, behavior differences, and layout/visual differences.
6. [x] Produce the complete screen map and issue list; fix the highest-priority confirmed gaps within scope.
7. [x] Apply shared-shell and page-specific fixes in small batches; recapture and check interactions without changing domain/API contracts.
8. [x] Recheck full-page, empty, and narrow-screen layouts; run package checks and browser verification without overwriting user-owned files.
9. [ ] Close the remaining prototype-field gaps in room editing (monthly room rent and bill day), preserve list/detail navigation state, and document any unresolved business behavior before treating the page as aligned.

## Verification and Deliverables

- [x] Desktop: 1366×768 and 1440×900. Mobile: 390×844.
- [x] Check page width/scrolling, navigation/content geometry, heading hierarchy, table/card layout, edit/detail presentation, and view/month/filter/back context.
- [x] Save `research/page-audit.md`, page screenshots, and recheck notes with prototype and implementation mappings.
- [x] Run Go package checks and browser checks for changed paths; treat prototype amounts as layout fixtures only.

## Risks

- The current workspace contains uncommitted user assets and audit scripts. Read/use them without cleanup or reset.
- Prototype controls may imply an initialization flow while the PRD excludes a setup wizard. Follow the user's explicit request for edit/list/detail pages and distinguish editing existing records from initial migration/setup.
- The audit README has stale instructions; verify the current local runner before starting it. Never connect to a shared or production instance.
- Screenshot differences can come from viewport, font, or content length. Compare identical viewports and states, and confirm geometry/interaction evidence instead of guessing from scaled screenshots.

## Before Starting

- [x] User reviewed and approved the PRD, design, and implementation plan.
- [x] Verify local audit environment and ensure it cannot reach shared/production data.
- [x] Load `trellis-before-dev` and the frontend responsive conventions before product-code changes.
