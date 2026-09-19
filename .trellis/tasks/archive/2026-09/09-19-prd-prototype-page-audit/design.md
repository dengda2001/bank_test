# Page Prototype Audit and Repair Design

## Evidence Sources

- Use `prdfile/prd.md`, `prdfile/design.md`, and `prdfile/implement.md` as the functional baseline, together with the user's explicit requirements in this task.
- Use `figma/rentops-desktop-suite.html` as the desktop visual/interaction baseline and `mobile/rentops-mobile-suite.html` as the mobile/narrow-layout baseline. Their `DESIGN-HANDOFF.md` files define visual and viewport expectations.
- Treat a locally running app as the implementation evidence. Source routes, templates, styles, and earlier task notes help locate causes but cannot replace browser observation.
- Prototype values and names are illustrative. Compare structure and meaning without asserting that product data must match sample amounts or identities.

## Coverage Matrix

The desktop prototype contains 15 page states: dashboard; properties, rooms, tenants, leases, bills, transactions, dunning, cash, expenses, and bank; plus property, room, tenant, and transaction details. The mobile prototype contains 14 screen states: home, dimension view, bills, transactions, objects, more, leases, dunning, expenses, bank, and four detail screens. Map each prototype state to a current URL or drawer/form state and verify that it is reachable.

Candidate route/source map (verify actual URLs, data, and reachable states in the browser):

| Prototype screen/state | Candidate implementation | Source entry point |
| --- | --- | --- |
| Dashboard / dimension switch | `/rent-dashboard` | `rent-workspace.html`, `rent_workspace_page.go` |
| Property list | `/properties` | `page_data_routes.go` |
| Room list | `/rooms` | `page_data_routes.go` |
| Tenant list | `/tenants` | `main.go` |
| Lease list | `/tenancies` | `page_data_routes.go` |
| Bills | `/bills` (legacy alias `/billing`) | `billing_page.go` |
| Transactions | `/transactions` (legacy alias `/billing`) | `transactions.go`, `billing_page.go` |
| Dunning | `/dunning` | `dunning.go`, `dunning_handlers.go` |
| Cash | `/cash-receipts` | `cash_receipts.go` |
| Expenses | `/expenses` | `main.go` |
| Bank settings | `/bank` | `page_data_routes.go` |
| Property detail/edit | `/properties/{id}`, edit state `?edit=1` | `page_data_routes.go` |
| Room detail/edit | `/rooms/{id}`, edit state `?edit=1` | `rent_workspace_page.go`, `page_data_routes.go` |
| Tenant detail | `/tenants/{id}` | `tenant_detail.go` |
| Transaction detail | Prototype detail state and current transaction detail/allocation interaction | Verify browser route/state |

Priority paths:

1. `/rent-dashboard` opens in the property view and switches among property, room, and tenant views while preserving month and filters.
2. Dashboard property drill-down reaches the room list; a room list item reaches its detail.
3. Property list, edit, and detail; room list, create/edit, and detail.
4. Tenant and transaction details, plus all other desktop/mobile prototype screens and key actions.

The inspected source contains view links, property/room routes, and forms. The implementation is spread across embedded templates and static assets, and the form presentation differs from the prototype's drawer/workspace pattern. Record runtime functionality and visual fidelity as separate dimensions.

## Comparison Method

- Capture the prototype and implementation at matching viewport sizes and interaction states.
- Use 1366×768 and 1440×900 for desktop, and 390×844 for mobile. Add the handoff's other widths for a page when a breakpoint issue appears: 360, 430, 600, 820, 1024, and 1920 pixels.
- Check information hierarchy, content geometry, gutters and whitespace, table/card density, navigation and fixed elements, type, color, control states, empty/error/success states, and drill-down behavior.
- Each finding cites the prototype screen/region, current URL/state, a screenshot or reproducible observation, a category (missing feature, data/state, layout/visual, or behavior), and priority.
- Run the app only in a local isolated audit environment. Verify the current `scripts/run-audit-local.sh` safety checks before use. Never target a live site or shared database.

## Implementation Boundaries

- During the audit, make no product-code changes; save notes and screenshots under this task's `research/` directory.
- After the reviewed plan is active, fix evidence-confirmed page differences in small batches. Check the shared shell/responsive conventions before each batch and recapture affected states. Per the user's latest direction, additive persistence for prototype fields is allowed. Financial changes must go through versioned tenancy records, retain responsibility shares, and honor generated-charge locks.
- The workspace contains user-owned uncommitted `figma/`, `mobile/`, `prdfile/`, `scripts/audit/`, `scripts/run-audit-local.sh`, and fixture files. Use them as inputs; do not overwrite, clean, or reset them.

## Trade-offs

- Matching actual pixels and interactions takes longer than checking routes/components, but catches runtime geometry and responsive issues.
- Use representative viewports as a baseline for every page, expanding a page's viewport matrix only when the screenshots or geometry checks reveal a breakpoint issue.
