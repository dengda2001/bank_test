# 暂不处理表单解析与提示修复

## Goal

Make “暂不处理” save from the matching drawer and explain recoverable failures in plain language.

## Requirements

- Accept the matching drawer's submitted form fields, including the bank transaction ID, for the defer action.
- Preserve ordinary form submissions and the existing ownership and pending-state checks.
- When a request is genuinely incomplete, tell the user that the change was not saved and what to do next. Do not present it as an unexplained system error.
- Keep the other uncommitted Trellis task and its page/test edits outside this change.

## Acceptance Criteria

- [x] A multipart defer request with a valid transaction ID reaches the defer service instead of returning `invalid_transaction_action` during form parsing.
- [x] A missing or malformed transaction ID still fails safely with a clear user message.
- [x] The browser displays the improved error in the floating notice, and normal URL-encoded form posts remain supported.
- [x] Relevant Go and JavaScript checks pass.

## Confirmed cause

The review drawer submits `new FormData(deferForm)` as `multipart/form-data`, while `handleTransactionAction` calls `r.ParseForm()` only. Go leaves multipart fields unread in that path, so `transaction_id` is empty and the handler redirects with `invalid_transaction_action` before it reaches the defer service.
