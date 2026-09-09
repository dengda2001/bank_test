# Database Guidelines

> Database patterns and conventions for this project.

---

## Overview

The demo does not use a database yet. Persistent local demo state is stored in
small JSON or JSONL files configured by environment variables.

## Scenario: Local Ledger JSON Storage

### 1. Scope / Trigger

- Trigger: Any feature that persists demo data outside the TrueLayer bank log or
  token file.
- Current examples: manually entered tenants and expenses.

---

### 2. Signatures

- `RENTOPS_TENANT_FILE`: JSON array file for `[]tenantRecord`; default
  `rentops-tenants.json`.
- `RENTOPS_EXPENSE_FILE`: JSON array file for `[]expenseRecord`; default
  `rentops-expenses.json`.
- `loadTenants() ([]tenantRecord, error)` and `saveTenants([]tenantRecord) error`
  own tenant persistence.
- `loadExpenses() ([]expenseRecord, error)` and `saveExpenses([]expenseRecord) error`
  own expense persistence.
- `readJSONFile(path string, out any) error` treats missing or empty files as an
  empty data set.
- `writeJSONFile(path string, value any) error` writes indented JSON with file
  mode `0600`.

---

### 3. Contracts

Tenant record fields:

- `id`: generated local record id.
- `name`: required, trimmed.
- `monthly_rent`: required positive number.
- `currency`: optional, defaults to `EUR`.
- `room_address`: required, trimmed.
- `created_at`: UTC RFC3339 timestamp.

Expense record fields:

- `id`: generated local record id.
- `description`: required, trimmed.
- `category`: optional, defaults to `General`.
- `amount`: required positive number.
- `currency`: optional, defaults to `EUR`.
- `expense_date`: `YYYY-MM-DD`; invalid or empty input falls back to current UTC
  date.
- `payment_method`: optional, defaults to `Manual`.
- `created_at`: UTC RFC3339 timestamp.

These files are local demo ledgers only. They do not sync to TrueLayer and must
not store bank access tokens or raw OAuth payloads.

---

### 4. Validation & Error Matrix

- Missing tenant file -> render empty tenant list.
- Empty tenant file -> render empty tenant list.
- Missing expense file -> render empty expense list.
- Empty expense file -> render empty expense list.
- Tenant POST missing `name`, `monthly_rent`, or `room_address` -> redirect to
  `/tenants?error=invalid_tenant`.
- Expense POST missing `description` or positive `amount` -> redirect to
  `/expenses?error=invalid_expense`.
- Invalid expense date -> save with current UTC date rather than rejecting the
  whole expense.
- File read/write failure -> return HTTP 500 from the route handler.

---

### 5. Good/Base/Bad Cases

- Good: Form handlers validate once at the HTTP boundary, append a typed record,
  then call the storage owner.
- Base: A fresh checkout with no ledger files can open `/tenants` and
  `/expenses` without creating files until the first POST.
- Bad: Rendering code parses untyped JSON maps directly, or form handlers write
  ad hoc JSON strings.

---

### 6. Tests Required

- Protected ledger routes redirect unauthenticated requests to `/`.
- Tenant POST persists tenant name, monthly rent, and room address to a temp file.
- Expense POST persists description, amount, category, and date to a temp file.
- Tests must use `t.TempDir()` for ledger files; do not create
  `rentops-tenants.json` or `rentops-expenses.json` in the repository root.

---

### 7. Wrong vs Correct

Wrong:

```go
body := fmt.Sprintf(`{"name":%q,"monthly_rent":%q}`, name, rent)
_ = os.WriteFile("rentops-tenants.json", []byte(body), 0o644)
```

Correct:

```go
tenants, err := a.loadTenants()
if err != nil {
    http.Error(w, err.Error(), http.StatusInternalServerError)
    return
}
tenants = append(tenants, tenantRecord{
    Name:        name,
    MonthlyRent: monthlyRent,
    RoomAddress: roomAddress,
})
if err := a.saveTenants(tenants); err != nil {
    http.Error(w, err.Error(), http.StatusInternalServerError)
    return
}
```

---

## Common Mistakes

- Do not use default ledger paths in tests or browser automation that submits
  forms. Use temp files through `RENTOPS_TENANT_FILE` and
  `RENTOPS_EXPENSE_FILE` so verification does not create untracked demo data in
  the repo.
