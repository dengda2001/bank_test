# Implementation plan: generic expense file attachments

1. Add tests for multiple-file create, one invalid file rolling back the whole create, file count/size limits, and cross-user read/remove rejection.
2. Add the `expense_attachments` migration and owner-scoped storage/service functions; keep legacy invoice reads/downloads intact.
3. Make expense creation multipart with zero or more files and one transaction for the expense plus every attachment.
4. Replace the invoice form with an attachment manager: list, multi-upload, download, and individual soft remove. Update list filters/labels without hiding legacy files.
5. Run focused tests, `go test ./cmd/truelayer-demo`, `go vet ./cmd/truelayer-demo`, and browser multi-upload/remove checks.

Rollback point: the new attachment table is additive and may remain after UI rollback. Legacy invoice routes continue to work; do not delete uploaded files during rollback.
