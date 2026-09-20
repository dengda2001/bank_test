# Design: workspace month and input control consistency

This child task implements parent requirements 4, 6, 12 and 14. Search labels and shared topbar cleanup were delivered in the quick-fixes child.

## Shared control contracts

### Calendar

- Use the existing `calendar.js` and `calendar.css` behavior and visual tokens as the reference.
- Load shared assets once through the common workspace shell mechanism so embedded and legacy pages receive the same picker.
- Initialization is idempotent. Month/date inputs created in a drawer after page load are enhanced when inserted/opened.
- Preserve the input's current form name/value, readonly/disabled state and change events.
- Dashboard month links preserve view, property, room, search, status, sort and page-size state.

### Select

- The native `<select>` remains the submitted control and works as fallback when scripting is unavailable.
- When enhanced, expose a button trigger and anchored listbox; synchronize selection into the native element and dispatch `input` and `change`.
- Support single-select controls used in current workspace forms. If implementation research finds a multi-select, leave it native and document why rather than silently changing its semantics.
- Support ArrowUp/ArrowDown, Home/End, Enter/Space, Escape and type-ahead; preserve visible focus and an accessible label.
- Position the popup from the clicked control, flip vertically near viewport edges and reposition on scroll/resize while open.
- Observe option mutations so dependent tenant/room/month selectors stay in sync.

### Search clear action

- Add one explicit labelled button per search input through a shared enhancement.
- The button uses a 44px minimum hit area on narrow screens, clears the input, dispatches `input` and returns focus to the input.
- Do not auto-submit unless the page already submits as a direct result of input events.

## Likely files

- `cmd/truelayer-demo/workspace_shell.go`
- `cmd/truelayer-demo/web/templates/partials/workspace-nav.html`
- `cmd/truelayer-demo/web/static/js/calendar.js`
- `cmd/truelayer-demo/web/static/css/calendar.css`
- New shared select/search control assets under `cmd/truelayer-demo/web/static/`
- `cmd/truelayer-demo/web/templates/pages/rent-workspace.html`
- Existing page heads that duplicate calendar includes

## Risks and compatibility

- Native popup appearance cannot meet the request consistently, so select controls need progressive enhancement with accessible custom popup behavior.
- Workspace pages mix embedded files with Go string templates. The shared loading point must not assume a frontend build step.
- Existing `change` handlers submit forms or rebuild dependent options. Event ordering must avoid duplicate submits and stale labels.
- Popups rendered in drawers may be clipped by drawer overflow; use viewport-anchored positioning rather than inheriting clipping containers.
