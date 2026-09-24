# Frontend Development Guidelines

> Conventions for the server-rendered UI in `cmd/truelayer-demo`.

---

## Overview

There is no JS framework or frontend build step. Pages are server-rendered with
Go `html/template`. Older pages still keep document templates and styles in Go
strings; extracted workspace pages use `web/templates/pages/*.html`, shared
partials under `web/templates/partials/`, and CSS under `web/static/css/`. The
`web_embed.go` embed filesystem and `newEmbeddedWorkspacePageTemplate` keep
those files available in the single deployed binary. "Frontend work" may mean
editing either a Go template or an embedded HTML/CSS file, depending on the page.

That shape has consequences a normal frontend spec would not need to state:
CSS source order is decided by Go string concatenation, comments inside `<style>`
are stripped by `html/template`, and the only assertions available are Go tests
that parse the rendered markup. Those are the things that bite.

---

## Guidelines Index

| Guide | Description | Status |
|-------|-------------|--------|
| [Responsive Conventions](./responsive-conventions.md) | Breakpoints, touch targets, the drawer, frozen columns, CSS ordering | Filled |
| [Status Vocabulary](./status-vocabulary.md) | One word and one control per stored status value; pseudo-filters; known vocabulary splits | Filled |
| [Rent Workspace Navigation](./rent-workspace-navigation.md) | Canonical rent workspace routes, hidden legacy pages, safe redirects | Filled |
| [Tenant Room Assignment](./tenant-room-assignment.md) | Creation-time room selection, safe plan previews, and composite form contract | Filled |
| [Sortable List Headings](./sortable-list-headings.md) | Typed server-side ordering, URL context, and accessible desktop heading links | Filled |
| [Shared Danger Confirmation](./danger-confirmation.md) | Form submitter and imperative dialog contracts for destructive actions | Filled |

---

## Where the UI lives

| File | Holds |
|------|-------|
| `cmd/truelayer-demo/workspace_shell.go` | `workspaceShell`, shared base template, and constructors for embedded or inline pages |
| `cmd/truelayer-demo/web/templates/partials/` | Shared navigation, mobile chrome, and object tabs |
| `cmd/truelayer-demo/web/templates/pages/` | Extracted server-rendered workspace pages, including property/room lists and details, and transaction details |
| `cmd/truelayer-demo/web/static/css/` | Shared and page-scoped stylesheets, including transaction detail styles, served from the embedded filesystem |
| `cmd/truelayer-demo/main.go` | `workspacePageCSS` = `embeddedWebText("web/static/css/workspace.css")` — the shared sheet, carrying both the `max-width: 1100px` and `max-width: 640px` tiers. Legacy templates concatenate it into `<style>`; embedded pages load the same file by `<link>` |
| `cmd/truelayer-demo/mobile_layout_test.go` | Narrow-screen and tier-order invariants, asserted against rendered markup and CSS |
| `cmd/truelayer-demo/desktop_layout_test.go` | Desktop invariants and the CSS tier-order guard |
| `scripts/audit/desktop-widths.mjs` | The browser-side acceptance probe: 11 pages × 1024/1366/1440/1920, fails on document-level horizontal overflow. Run against `scripts/run-audit-local.sh`, never `:8081` |

---

**Language**: All documentation should be written in **English**.
