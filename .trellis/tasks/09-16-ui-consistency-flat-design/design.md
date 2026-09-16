# UI consistency and Flat Design technical design

## Current system

- The application is a single Go binary under `cmd/truelayer-demo`.
- User-facing pages are rendered with `html/template`; there is no frontend package, bundler, Tailwind setup, or component library.
- The shared workspace shell is embedded in templates and reuses `workspacePageCSS` from `cmd/truelayer-demo/main.go`.
- The dashboard adds `workspaceCalendarCSS` and an inline interaction script. Billing, tenant list/detail, expenses, cash receipt pages, dunning drawer, and transaction tables are server-rendered HTML forms and links.
- Login, revoke preview, and payer preview have their own standalone styles. They must be brought into the same token language without changing routes or form names.

## Page and state inventory

| Route / template | Important states and interactions |
| --- | --- |
| `GET /` (`loginTemplate`) | login, invalid credentials, autofill/focus |
| `GET /rent-dashboard` (`rentDashboardTemplate`) | monthly metrics, filters, pagination, expandable rows, dunning drawer, notices, empty states |
| `GET /billing` (`billingTemplate`) | connection state, sync/error notices, filters, pagination, allocation controls, action forms, empty state |
| `GET /tenants` (`tenantTemplate`) | list, add/edit form, expandable billing history, status badges, empty state |
| `GET /tenants/{id}` (`tenantDetailTemplate`) | profile, payer relation form, history filters/table, cash entry, error/notices |
| `GET /expenses` (`expenseTemplate`) | expense list/form, empty state |
| `GET /cash-receipts/new` and preview (`cashReceiptTemplate`) | form, validation errors, preview confirmation, correction link |
| `GET /cash-receipts/void` (`cashReceiptVoidTemplate`) | receipt facts, already-voided state, reason form |
| `GET /billing/revoke` (`revokePreviewTemplate`) | allocation review, reason form, cancel link |
| `GET /billing/payer/preview` (`payerPreviewTemplate`) | table, per-row confirmation form, no-result state |

Actual route registration remains the source of truth; no route, form action, field name, or business text is changed by this task.

## Design system

Use a single light, flat token set across all templates:

- Canvas `#ffffff`; app background `#f3f4f6`; foreground `#111827`.
- Primary blue `#2563eb` / hover `#1d4ed8`; emerald `#059669`; amber `#d97706`; danger `#dc2626`.
- Surfaces are solid color blocks (`#ffffff`, `#eff6ff`, `#ecfdf5`, `#fffbeb`) with no box shadows, blur, textures, or decorative gradients.
- Borders are reserved for form controls and high-value structure; use `2px` controls and moderate `6px`/`8px` radii.
- Use system sans fallback with a strong geometric hierarchy; retain monospace only for IDs, dates, and machine values.
- Focus uses a visible solid blue outline/ring; status uses text plus color, never color alone.

## Boundaries and compatibility

- Keep business data types, handlers, routes, HTTP methods, form field names, and JavaScript data attributes unchanged.
- Keep the existing shared CSS injection pattern so the templates continue to compile at package initialization.
- Add only shared CSS selectors where possible; page-specific CSS should express layout differences, not redefine buttons, inputs, typography, or status colors.
- Use CSS-only responsive behavior plus the existing small scripts. No new runtime dependency is needed.
- Dense tables remain inside a labelled `.table-wrap` with intentional horizontal scrolling at narrow widths; the page itself must never horizontally overflow. Expandable rows and drawer controls remain keyboard reachable.

## Execution and rollback

1. Stage 1 updates shared primitives and page-specific overrides for consistent sizing, focus, spacing, and responsive behavior while preserving the existing visual theme. Commit independently.
2. Stage 2 replaces the shared and standalone visual tokens with the flat system above, preserving markup and interactions. Commit independently.
3. Stage 3 verifies and fixes the 320/360/375/390/412 mobile layouts and all interactive states. Commit independently.

Each stage can be reverted by its own commit. Before each commit run Go tests and inspect rendered pages in a real browser at desktop and mobile widths.
