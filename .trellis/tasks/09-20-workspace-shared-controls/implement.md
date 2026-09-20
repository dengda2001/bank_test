# Implementation plan: workspace month and input control consistency

## Dependency

Depends on the approved parent assessment and quick-fixes slice. It does not depend on the contextual drawer or transaction workflow tasks; later drawer work must call the shared enhancement hook after inserting controls.

## Steps

1. Inspect existing calendar initialization and every page's calendar asset loading. Choose one shared include/init point and remove duplicate includes.
2. Update dashboard month navigation to previous / shared month picker / next while carrying all active query state.
3. Prototype the custom select enhancement on representative static and dependent selects; then enable across workspace pages.
4. Add explicit search clear buttons and the shared hit-area/focus behavior.
5. Run static searches for all date/month/select/search controls and stale duplicate calendar initialization.
6. Inspect changes against the frontend responsive conventions and verify narrow-screen geometry in a browser when explicitly authorized.

## Validation

- Static source review and `git diff --check`.
- No tests or browser checks are run in this slice unless the user requests verification.
- Keep the control enhancement progressive so no-script pages continue to submit native controls.
