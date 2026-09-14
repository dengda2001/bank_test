# Quality Guidelines

> Code quality standards for backend development.

---

## Overview

<!--
Document your project's quality standards here.

Questions to answer:
- What patterns are forbidden?
- What linting rules do you enforce?
- What are your testing requirements?
- What code review standards apply?
-->

(To be filled by the team)

---

## Forbidden Patterns

<!-- Patterns that should never be used and why -->

(To be filled by the team)

---

## Required Patterns

<!-- Patterns that must always be used -->

(To be filled by the team)

---

## Testing Requirements

### Scenario: Demo Auth Gate for Bank Data Screens

#### 1. Scope / Trigger

- Trigger: Any route that starts bank authorization, handles OAuth callbacks, refreshes bank data, or renders bank-derived UI.
- This project currently runs as a small Go HTTP demo, but bank tokens and bank transaction payloads are still sensitive.

#### 2. Signatures

- `GET /` renders the local demo login screen when unauthenticated.
- `POST /login-local` validates the configured demo administrator credential and sets the app session cookie.
- `POST /logout` clears the app session cookie.
- `GET /billing` requires the app session and renders the bank income workspace.
- `GET /login` requires the app session and starts the TrueLayer OAuth flow.
- `GET /callback` requires the app session before exchanging an authorization code.
- `GET|POST /refresh` requires the app session before using the stored refresh token.

#### 3. Contracts

- Environment keys:
  - `APP_ADMIN_USERNAME`: demo administrator username.
  - `APP_ADMIN_PASSWORD`: demo administrator password.
  - `APP_SESSION_SECRET`: HMAC signing secret for the demo session cookie.
  - `TL_TOKEN_FILE`: local refresh-token file path.
  - `TL_LOG_FILE`: local JSONL bank payload log path.
- Session cookie:
  - `HttpOnly`.
  - `SameSite=Lax`, so the OAuth redirect back to `/callback` can carry the app session.
  - Must not contain bank tokens or raw bank payloads.
- Stored token file:
  - Stores refresh token only, never access token.
  - Uses file mode `0600`.

#### 4. Validation & Error Matrix

- Missing or invalid app session on protected route -> redirect to `/`.
- Invalid demo login credentials -> redirect to `/?error=invalid_login`.
- Missing stored refresh token on refresh -> redirect to `/billing?reconnect=1&error=no_saved_login`.
- Refresh token rejected by provider -> redirect to `/billing?reconnect=1&error=refresh_failed`.
- Missing or expired OAuth state -> reject callback before token exchange.

#### 5. Good/Base/Bad Cases

- Good: `/callback` checks the app session before consuming OAuth state, exchanging code, saving refresh token, or logging bank data.
- Base: `/billing` can render with no token file and no bank log, showing a bind-bank empty state.
- Bad: Returning raw bank JSON from `/callback` to an unauthenticated browser, or exchanging OAuth code without a valid app session.

#### 6. Tests Required

- Login success sets a session cookie and redirects to `/billing`.
- Protected routes reject missing sessions.
- Callback without app session does not consume OAuth state.
- Stored refresh token detection is covered.
- Income transaction normalization covers `CREDIT`, positive amount fallback, debit filtering, and unknown/inferred payer fields.

#### 7. Wrong vs Correct

Wrong:

```go
func (a *app) handleCallback(w http.ResponseWriter, r *http.Request) {
    token, _ := a.exchangeCode(r.Context(), r.URL.Query().Get("code"))
    _ = a.saveStoredToken(token)
}
```

Correct:

```go
func (a *app) handleCallback(w http.ResponseWriter, r *http.Request) {
    if !a.requireAuth(w, r) {
        return
    }
    if err := verifyState(r); err != nil {
        http.Error(w, err.Error(), http.StatusBadRequest)
        return
    }
    // Exchange code and save refresh token only after both checks pass.
}
```

### Scenario: Transaction Description Month Parsing

#### 1. Scope / Trigger

- Trigger: Any bank transaction matching or billing-history projection that derives a billing month from `Description` or `Reference`.
- Keep date extraction in the shared `parseReferencedPeriod` path so matching and UI projections use the same interpretation.

#### 2. Signatures

- `parseReferencedPeriod(text string, transactionTime time.Time) (time.Time, bool)` returns the first recognized billing period and whether the description contained a usable period.
- `selectObligationForTransaction(tx transactionRow, obligations []obligationRow) (obligationRow, bool)` consumes the parsed period when selecting a billing obligation.

#### 3. Contracts

- Supported explicit formats include numeric month/year (`9/26`, `09/2026`), spaced English month/year (`Sep 2026`), Chinese year/month, and compact English month/year (`JULY26`, `SEP26`, `JULY2026`).
- A two-digit year in a compact English token maps to `2000 + year`.
- An explicit year in the description always takes precedence over `transactionTime`; `transactionTime` is only a fallback for month-only references such as `JULY`.
- The parser returns the first valid period in the combined description/reference text, preserving the existing matching precedence.

#### 4. Validation & Error Matrix

- Valid month and year -> return the first day of that month with `true`.
- Invalid month/year token -> ignore that token and continue with other supported formats; if none match, return the existing month-only or no-period fallback.
- Unknown description text -> return `false` without inventing a year.
- Compact token such as `JULY26` with a transaction dated in 2027 -> return July 2026, never July 2027.

#### 5. Good/Base/Bad Cases

- Good: `REMAIN50 *MOBI RENT JULY26` on a 2027 transaction matches `2026-07-01`.
- Base: `RENT 9/26` matches September 2026; `REMAIN 50 EURO *MOBI RENT 8/26` matches August 2026.
- Bad: Treating `JULY26` as month-only and deriving 2027 from the transaction timestamp.

#### 6. Tests Required

- Unit-test compact English month/year tokens with a transaction timestamp in a different year; assert both `2026-07-01` and `2026-09-01` examples.
- Retain tests for numeric, spaced English, Chinese, month-only, and no-period inputs.
- Run the billing matching and full backend test suites after changing regex precedence or fallback behavior.

#### 7. Wrong vs Correct

Wrong:

```go
// JULY26 is reduced to JULY, then the transaction year is inferred.
month := monthOnlyRegex.FindStringSubmatch(text)
return time.Date(transactionTime.Year(), monthNumber(month), 1, 0, 0, 0, 0, time.UTC), true
```

Correct:

```go
if m := compactEnglishMonthYearRegex.FindStringSubmatch(text); len(m) == 3 {
    year := parseCompactYear(m[2])
    return time.Date(year, monthNames[strings.ToLower(m[1])], 1, 0, 0, 0, 0, time.UTC), true
}
```


---

## Code Review Checklist

<!-- What reviewers should check -->

(To be filled by the team)
