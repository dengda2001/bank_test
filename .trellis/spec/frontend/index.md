# Frontend Development Guidelines

> Conventions for the server-rendered UI in `cmd/truelayer-demo`.

---

## Overview

There is no JS framework and no build step. Every page is a Go `html/template`
string that renders a full document: inline `<style>`, inline `<script>`, plain
HTML. "Frontend work" here means editing template strings and the shared CSS
constants they concatenate.

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
| `cmd/truelayer-demo/workspace_shell.go` | `workspaceNav` (the shared sidebar + drawer markup), `workspaceShell` struct, `newWorkspacePageTemplate` |
| `cmd/truelayer-demo/main.go` | `workspacePageCSS` (the shared stylesheet, including the shared `@media (max-width: 640px)` block) |
| `cmd/truelayer-demo/<page>.go` | One page per file: its data struct, handler, template, and page-local `<style>` |
| `cmd/truelayer-demo/mobile_layout_test.go` | The narrow-screen invariants, asserted against rendered markup |

---

**Language**: All documentation should be written in **English**.
