# Implementation

- [x] Reuse the shared tenant drawer with room-bound data.
- [x] Add room page query and POST-return handling, including cancel/success/error behavior.
- [x] Update new-room handoff and room “新增租客” link.
- [x] Make conflict notices selected-only in room create and rent-plan editor.
- [x] Test room ownership/redirect validation and selected-only warning; verify desktop/mobile form flow.

Validation: focused room/tenant Go tests, `go test ./...`, browser walkthrough. Review edits in `main.go`, `page_data_routes.go`, and tenant drawer template against unrelated worktree changes.
