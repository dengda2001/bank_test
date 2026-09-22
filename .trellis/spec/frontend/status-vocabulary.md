# Status Vocabulary and Filter Controls

> One stored status value, one word, one control per page.

---

A status has two halves that change at different speeds. The **stored value**
(`matched`, `overdue`, `paid`, …) is a contract: it lives in the database, in the
query parser, and in a validation whitelist. The **display word** is presentation.
Most status bugs in this codebase come from editing one half while believing you
edited the other, or from letting two views invent their own word for the same
value.

## Scenario: A status is rendered, filtered, or reworded

### 1. Scope / Trigger

- Trigger: adding or rewording a status label, adding a status filter control,
  or deduplicating filter controls on a list page.
- Why: the same value is rendered as a row badge, a dropdown option, a tab, and a
  column header. When each writes its own string, they drift — and the drift is
  invisible until someone compares two pages side by side.

### 2. Signatures

- Stored transaction match statuses: `matched`, `candidate`, `unmatched`,
  `needs_review`, `partial`, `ignored`.
- Stored rent statuses: `paid`, `partial`, `open`, `overdue`, `needs_review`,
  `vacant`.
- Label sources: the `statusLabel` map in the transaction row renderer
  (`transactions.go:461-468`, fallback `未关联` at `469-470`) and
  `workspaceStatusLabel` (`rent_workspace.go:930-937`, delegating to
  `rentStatusLabel`).
- Validation whitelist: `transactions.go:318`; rent filter whitelist:
  `rent_workspace.go:110`.
- Query parameters: `match_status` (transactions), `status` (rent workspace).

### 3. Contracts

- **One parameter, one control.** A page renders exactly one control per query
  parameter. Two `<select>`s bound to the same parameter are not redundancy, they
  are a divergence waiting to happen: `/transactions` carried both a compact
  `match_status` selector and the filter bar's, and the compact one never gained
  the `ignored` option. A user entering through it could not reach 已忽略.
- **Words come from the label function, not from the template.** A dropdown
  `<option>` must read exactly what the row badge for that value reads. The rent
  workspace dropdown said 逾期 / 已交满 / 未到期未缴 while its own badges said
  已逾期 / 已缴清 / 未缴.
- **A control's own label counts as a word.** The surviving transactions selector
  was labelled 匹配状态 while all eight of its options said 关联. Renaming options
  without renaming the label leaves the control contradicting itself.
- **The verb 匹配 is not the state 关联.** 匹配流水, 确认匹配, 撤销匹配, 匹配依据
  name the *act* of matching a transaction to a rent month, and they keep the
  verb. Only the *state* a row is in moves to 关联. Do not blanket-rename 匹配.
- **Rewording never moves values.** Deduplicating controls or unifying words must
  not touch a stored value, the validation whitelist, or any derivation function
  (`ledgerObligationStatus` in `ledger.go`, `workspaceWorstStatus` in
  `rent_workspace.go`). Every value must remain reachable from some control.
- **Pseudo-filters are not values.** `pending` (transactions) and `unpaid` (rent
  workspace) are not in the whitelist as states — they expand to a set:
  `pendingMatchStatuses` is `candidate, needs_review, unmatched, partial`
  (`transactions.go:78`), and `unpaid` expands to
  `needs_review + overdue + partial + open` (`rent_workspace.go:944`). They are
  accepted by the parser (`transactions.go:318`) but never round-trip as a row's
  own status, so never derive a badge label from them.
- **Check for a second renderer before deleting a control.** `/billing` and
  `/transactions` used to render different chrome from the same template behind
  a `PageKey` branch, so a control could look like dead markup while the other
  path still used it. That branch is gone, but the habit stands: a control
  guarded by a condition is not the same as a control nothing renders.

### 4. Validation & Error Matrix

| Input | Result |
|---|---|
| Status outside the whitelist | HTTP 400 (transactions), or falls back to `all` (rent filters) |
| Pseudo-filter (`pending`, `unpaid`) | Accepted; expands to its set, filtered income-only (`pending`) |
| Empty status | Renders all rows; the dropdown shows the 全部 option |
| Status reachable from no control | Not valid — every whitelisted value needs a UI path |

### 5. Good / Base / Bad Cases

- Good: the transactions page has one `match_status` selector listing all seven
  reachable values including 已忽略, and its tab for `matched` reads 已关联 —
  the same word the row badge renders.
- Base: `/transactions` renders the filter bar's status selector on wide screens
  and the compact quick-filter form on narrow ones; the `/billing` page it
  replaced had its own eight-option selector, which is why the two used to drift.
  `/billing` now only forwards here, so there is one selector to keep honest.
- Bad: two selects for one parameter. They pass a test that only asserts "a
  selector exists", then drift apart on the next feature.
- Bad: rewording a dropdown option without checking the badge — this is how 逾期
  (dropdown) and 已逾期 (badge) coexisted on one screen.

### 6. Tests Required

- `TestTransactionStatusSelectorExistsExactlyOnceAndReachesEveryStatus`: exactly
  one status selector renders, it reaches every whitelisted value, and no retired
  word (已匹配 / 未匹配 / 部分匹配) survives.
- `TestRentWorkspaceStatusFilterUsesTheBadgeVocabulary`: the option `value`s are
  unchanged **and** each option's text equals `workspaceStatusLabel` for that
  value, so the dropdown and the badge cannot drift again.
- `TestRentWorkspaceConfirmationToastUsesTheBadgeVocabulary`: the `rent_confirmed`
  toast names the state with 已关联, not the retired 已匹配. Prose drifts on its
  own schedule; a state named in a sentence needs the same guard as a badge.
- `TestTransactionRouteUsesPrototypeQueueAndKeepsLocalReturnPath`: the compact
  filter form still carries its payer, period, and scope fields after its
  duplicate selector was removed.
- **Assert on the shape that identifies the control.** `strings.Count(page,
  `name="match_status"`)` counts hidden pager inputs too — a page with one
  selector and one hidden field counts 2. Count `id="match_status"`, or assert
  the absence of `<select name="match_status"`, so the number means what the test
  name claims.

### 7. Wrong vs Correct

#### Wrong — a second control for the same parameter, with its own words

```html
<!-- compact filter form: its own selector, its own words -->
<select name="match_status" aria-label="流水状态" onchange="this.form.requestSubmit()">
  <option value="pending">待处理</option>
  <option value="matched">已匹配</option>   <!-- filter bar says 已关联 -->
  <option value="">全部状态</option>
  <option value="partial">部分匹配</option> <!-- filter bar says 部分关联 -->
  <!-- 已忽略 missing: unreachable from here -->
</select>
```

#### Correct — one control, label and options from the badge's vocabulary

```html
<!-- compact filter form: payer/period/scope only; status lives in the filter bar -->
<label for="match_status">关联状态<select id="match_status" name="match_status" onchange="this.form.requestSubmit()">
  <option value="">全部状态</option>
  <option value="matched" {{if eq .MatchStatusSelection "matched"}}selected{{end}}>已关联</option>
  <option value="partial" {{if eq .MatchStatusSelection "partial"}}selected{{end}}>部分关联</option>
  <option value="ignored" {{if eq .MatchStatusSelection "ignored"}}selected{{end}}>已忽略</option>
  <!-- …all whitelisted values, each word matching its row badge -->
</select></label>
```

---

## Known remaining inconsistencies

Recorded, deliberately not fixed — do not treat these as regressions:

| Where | Split |
|---|---|
| `transactionMatchStatusLabel` (`transaction_previews.go:183`) | A third label map over the same stored values: 已匹配候选 / 部分匹配候选 / 未找到候选. Rendered on the payer-preview page and as the detail page's suggestion status (`transaction_detail.go:311`), so one screen shows 已关联 in the list badge and 已匹配候选 in the detail. The 候选 suffix is meaningful here (it labels a *proposed* rent month, not the transaction's own state), which is why it was left alone. |
| `rent-workspace.html:67`, `:115` | Column headers and card labels say 已交满 / 未交满 while the status badges say 已缴清 / 未缴. Headers count rooms (`已交满 N 间`), so they are not the same question as the status filter. |
| `/bills` | Legacy GET now redirects to `/rent-dashboard?view=tenants`; the retained template vocabulary is not an active product surface. |
| `needs_review` | `rentStatusLabel` (`obligations.go`) says 需处理; `workspaceStatusLabel` overrides it to 待处理 for the workspace. Three call sites, two words. |
| 超额付款 | The prototype has an overpaid filter; `grep '超额\|overpaid'` finds nothing, and `ledger.go`'s `paid >= expected` folds overpayment into `paid`. |
| `transaction_actions.go:199` | The undefer audit reason is written to the database as 已匹配，恢复待处理状态. Stored data, not a rendered string — rewording it would split historical rows from new ones. |
