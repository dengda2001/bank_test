# Full Page Prototype Fidelity Audit and Repair

## Goal

Compare every user-facing page and important state with the product requirements and prototypes. Record missing pages, actions, states, and visual differences with evidence, then fix confirmed page and layout/UI gaps in priority order.

## Confirmed Facts

- Functional requirements are in `prdfile/prd.md`; page and interaction design are in `prdfile/design.md` and `prdfile/implement.md`.
- The current prototype files are `figma/rentops-desktop-suite.html` and `mobile/rentops-mobile-suite.html`. Their handoff documents call them the implementation and responsive-validation baselines.
- The desktop prototype has 15 page states: dashboard; properties, rooms, tenants, leases, bills, transactions, dunning, cash, expenses, and bank; plus property, room, tenant, and transaction details.
- The user called out the dashboard's property/room/tenant switch, property and room lists, editing, and detail pages as priority areas. Layout and UI should match the prototype as closely as possible.
- The server-rendered app has dashboard and room-detail templates plus routes for properties, rooms, tenants, and other pages. Routes and templates alone do not prove the running experience matches the prototype.
- The `rent-workspace.html` template already contains property, room, and tenant view links. Their runtime rendering, default view, navigation state, data, and visual treatment still need verification.
- Earlier Figma implementation tasks exist. This audit must use the current running app and current prototype files as evidence rather than treating past acceptance notes as proof.

## Requirements

- Enumerate each prototype screen and key state, and map it to the running route, handler, template, or drawer/form interaction. Check lists, create/edit forms, and details separately.
- For every page, compare feature and interaction coverage, displayed data and states, and visual fidelity. Distinguish a missing feature from an implemented feature that differs from the design.
- Prioritize the three dashboard views, property and room lists, property/room editing and details, and the list-to-detail paths.
- Compare desktop and mobile prototypes at representative viewport sizes. Record structure, navigation, content width and whitespace, grids/tables, typography, color, spacing, borders/radii, control states, and responsive changes.
- Each finding must identify the page/route, category, severity, prototype evidence, current implementation evidence, and a proposed fix. Do not infer runtime behavior from templates alone.
- Match the prototype's layout and visual hierarchy as closely as possible. Treat prototype data as illustrative and do not copy static sample names or amounts into product behavior.
- Save the page matrix, findings, and key screenshots/evidence in this task directory.
- Implement confirmed UI/page differences, then recheck the same page states and viewports. Prioritize the prototype's layout, hierarchy, spacing, typography, and control treatment.

## Acceptance Criteria

- [ ] Every prototype screen appears in the page matrix with a verified implementation status or a clear reason it could not be checked.
- [ ] Missing pages/actions, data/state issues, interaction issues, and visual/layout differences are listed separately with locatable evidence and priority.
- [ ] The dashboard's three views and the property/room list, edit, and detail paths each have a specific audit result.
- [ ] Desktop and mobile visual differences are recorded page by page; key layout issues include screenshots or reproducible evidence.
- [ ] A prioritized audit report and proposed fix order are delivered.
- [ ] Confirmed in-scope differences have before/after evidence; representative pages and viewports are rechecked against the prototype, and any remaining gap has a documented reason.

## Out of Scope

- Rewriting existing accounting history or changing a financial rule that the prototype does not define. If a screen requires a new rule for allocating room rent among tenants, preserve the existing responsibility shares and generated-bill locks unless the user specifies otherwise.
- Schema and persistence changes needed to make prototype fields editable are in scope. The user has explicitly directed that the prototype is the source of truth; new financial values must use the existing versioned tenancy model and must never silently rewrite generated bills.
- Overwriting, cleaning, or resetting existing uncommitted files. The current workspace has user changes that must be preserved.

## Notes

- Keep this PRD focused on requirements, constraints, and acceptance criteria.
- Complex tasks need `design.md` and `implement.md` before `task.py start`.
