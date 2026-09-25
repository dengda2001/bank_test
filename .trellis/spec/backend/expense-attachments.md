# Expense attachments

## Current contract

Expenses can hold multiple generic files in `expense_attachments`. The `manual_expense_invoices` table and `/expenses/invoices/:id` remain readable for older uploads; new forms do not require invoice number, vendor, date, amount, or note.

- `POST /expenses` accepts `multipart/form-data` with optional `attachments` files. The expense, synthetic bank transaction, and all files commit in one database transaction. Any invalid file aborts the create.
- `POST /expenses/files` adds files to an owner-owned expense. `GET /expenses/files/:id` downloads one owner-owned active file as an attachment. `POST /expenses/files/:id` soft-removes only that file.
- File reads, adds, and removals always scope both expense or attachment ID and `user_id`. Removed files remain in the database for audit and are excluded from lists and downloads.
- Each file is at most 8 MiB; each request at most 32 MiB; each upload at most 10 files. Accept PDF, JPEG, PNG, WebP, DOCX, XLSX, PPTX. Compare file extension with detected content; Office files must contain the expected OpenXML parts and type declaration. Sanitize the basename. Downloads send `Content-Disposition: attachment`, `nosniff`, and private no-store headers.
- Expense list “has attachment” includes active generic files, current legacy invoice files, and historical external invoice URLs. New files do not replace one another. Bank debit attribution uses the same generic multi-file upload in the attribution transaction.

The internal `invoice_linked` / `invoice_missing` filter keys and `InvoiceLinked` view field remain for URL compatibility; user-facing labels are “有附件” / “无附件”.
