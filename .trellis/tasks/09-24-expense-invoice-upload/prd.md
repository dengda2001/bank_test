# Simple expense file attachments

## Goal

Let a landlord manage multiple files on each expense without filling invoice metadata.

## Requirements and acceptance

- [ ] The create-expense drawer accepts zero or more files in the same save flow.
- [ ] Existing expenses allow adding, downloading, and removing individual files; number, vendor, date, amount, and note are not required.
- [ ] Multiple active files can coexist on one expense. Existing legacy invoice files remain downloadable. File ownership and expense ownership are checked server-side.
- [ ] Invalid type/size and upload failures explain the issue and do not leave a half-created expense or lose unrelated files.
- [ ] The list's attachment-present/missing filter reflects active files correctly.

## Constraints

- Keep existing historical invoice rows and downloads compatible.
- Accept PDF, common images (JPEG, PNG, WebP), and modern Office documents (DOCX, XLSX, PPTX). Bound file size and count; download every file as an attachment.
- Preserve user isolation and safe download headers.
