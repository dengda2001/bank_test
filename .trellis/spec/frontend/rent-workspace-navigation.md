# Rent Workspace Navigation and Legacy Routes

> Public page entry points and retained aliases for the room rent-plan
> workflow.

## Scenario: Navigating, configuring, or settling monthly rent

### 1. Scope / Trigger

- Trigger: changing rent workspace navigation, tenant/room configuration,
  monthly responsibility actions, or a retained `/bills` compatibility route.
- Applies to server-rendered templates in `cmd/truelayer-demo/web/`, route
  handlers in `main.go` and `page_data_routes.go`, and manual balance redirects.

### 2. Signatures

- `GET /rent-dashboard?period=YYYY-MM&view=rooms|tenants` is the canonical
  monthly rent workspace.
- `POST /rent-dashboard/settle` is the canonical manual-balance action.
- `GET /bills?...` is a legacy redirect to `/rent-dashboard` with
  `view=tenants`; `/bills/settle` remains a legacy action alias.
- `/tenancies` is retired and has no registered GET or POST handler; both
  methods return 404.
- `/billing` remains the bank-transaction matching alias. It is not a rent-bill
  route and must not be removed with `/bills` navigation.

### 3. Contracts

- Do not expose a standalone bills or leases/tenancies item in desktop sidebar,
  mobile navigation, or the More page. Rent collection lives under the
  workspace tenant view; occupancy, rent, due day, and responsibilities are
  edited only in the room rent-plan form. Tenant profile forms contain identity
  and payer details, not rent-plan fields.
- Keep system accounting facts behind the user-facing workflow. Templates must
  submit stable tenant/obligation identifiers and must not reconstruct money
  from display strings.
- The `/bills` redirect preserves valid rent filters, month, and whitelisted
  notice/error codes while selecting `view=tenants`. Do not redirect or add a
  compatibility handler for `/tenancies`.
- Workspace actions preserve the `period` and `view` from a same-site
  `return_to=/rent-dashboard?...`. A return target with an absolute URL,
  foreign host, userinfo, fragment, or a different path is rejected; fallback
  redirects derive from the action's own validated context. Legacy `/bills`
  action requests default to the tenant view.
- Continue using server-rendered `html/template`, shared workspace navigation,
  and `html/template` escaping. Compatibility aliases must call the same domain
  action, never create a parallel accounting implementation.

### 4. Validation & Error Matrix

| Input / request | Result |
|---|---|
| Unauthenticated legacy GET | Auth flow; do not reveal account data |
| Valid `GET /bills` | `302` to tenant view with supported context preserved |
| Invalid bill period/filter | Safe current/default workspace filters; no arbitrary SQL filter |
| `GET /tenancies` or `POST /tenancies` | `404`; route is retired |
| External or malformed `return_to` | Ignore it and use a same-site workspace fallback |
| `POST /rent-dashboard/settle` or legacy alias | Same validation and domain mutation; redirect into workspace |

### 5. Good / Base / Bad Cases

- Good: a tenant responsibility action from the workspace posts to the
  canonical route with a same-site return URL, then returns to the same month
  and view.
- Base: an old `/bills?period=2026-09` bookmark opens the tenant view for that
  month without a standalone bill page.
- Bad: adding a bills link back to primary navigation, deleting `/billing`,
  redirecting an action to an external `return_to`, or keeping a template-only
  source of rent amounts.

### 6. Tests Required

- Route tests assert `/bills` redirect context, `/tenancies` GET/POST 404,
  retained action aliases, and `/billing` transaction behavior.
- Redirect tests reject external, malformed, and non-workspace return paths and
  retain the allowed month/view for safe same-site paths.
- Template tests assert no standalone bill or tenancy navigation item, that
  the room rent-plan form shows the saved schedule, and that physical property,
  room, and tenant forms contain no validity or rent-plan fields.
- Run the focused Go route/template tests, `go test ./... -run '^$' -count=1`,
  `go vet ./...`, and `git diff --check`; run browser checks when the local
  browser acceptance environment is available.

### 7. Wrong vs Correct

Wrong:

```html
<a href="/bills">应收账单</a>
<form action="/bills/settle" method="post">...</form>
```

Correct:

```html
<a href="/rent-dashboard?view=tenants">本月应收</a>
<form action="/rent-dashboard/settle" method="post">
  <input type="hidden" name="return_to" value="/rent-dashboard?period=2026-09&amp;view=tenants">
  ...
</form>
```

Only explicitly retained aliases such as `/bills` stay as compatibility entry
points. `/tenancies` is retired, while the product uses one canonical workspace
and one accounting implementation.
