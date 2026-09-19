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

---

## Where the UI lives

| File | Holds |
|------|-------|
| `cmd/truelayer-demo/workspace_shell.go` | `workspaceShell`, shared base template, and constructors for embedded or inline pages |
| `cmd/truelayer-demo/web/templates/partials/` | Shared navigation, mobile chrome, and object tabs |
| `cmd/truelayer-demo/web/templates/pages/` | Extracted server-rendered workspace pages, including property/room lists and details, and transaction details |
| `cmd/truelayer-demo/web/static/css/` | Shared and page-scoped stylesheets, including transaction detail styles, served from the embedded filesystem |
| `cmd/truelayer-demo/main.go` | `workspacePageCSS` (shared shell styles, including the `max-width: 640px` block) |
| `cmd/truelayer-demo/mobile_layout_test.go` | Narrow-screen invariants, asserted against rendered markup and CSS |

---

**Language**: All documentation should be written in **English**.
