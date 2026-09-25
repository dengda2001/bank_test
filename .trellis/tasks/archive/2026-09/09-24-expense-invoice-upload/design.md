# Design: generic expense file attachments

## Boundaries and flow

The expense create form becomes `multipart/form-data` with a multiple-file input. `createExpense` reads a bounded multipart request, validates the expense and each optional file, then writes the expense and all selected attachments in one database transaction. If any file fails validation or storage, none of the rows commit.

Add an `expense_attachments` table with owner, expense, safe file name, detected content type, size, digest, binary content, created timestamp, and optional removal timestamp. An expense may have multiple active attachments. The existing invoice table and its download route remain readable for legacy files; no new invoice metadata is written. The expense file drawer lists all active attachments with download and individual remove actions and accepts multiple new files.

File validation accepts PDF, JPEG, PNG, WebP, DOCX, XLSX, and PPTX, with safe basenames, an 8 MB per-file limit, a 32 MB request limit, and at most 10 files per upload. Check document containers rather than trusting extension alone. Owner-scoped downloads use `Content-Disposition: attachment` and `nosniff`. The expense form preserves its return path and gives a specific upload error code. The list changes “invoice linked/missing” wording to “has/no attachment” and includes active attachments plus legacy invoice files; existing external invoice URLs remain visible as legacy links.

## Compatibility and risk

The additive attachment migration leaves historical invoice rows untouched. The main risks are a half-saved expense when one upload fails and accidental cross-user file access. Keep the expense and all attachments in one transaction, owner-scope every read/remove, bound the whole multipart body, and clean temporary multipart files. Soft removal preserves an audit trail and does not affect other files.
