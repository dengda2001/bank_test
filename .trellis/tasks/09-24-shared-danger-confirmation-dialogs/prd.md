# Shared danger confirmation dialogs

## Goal

Replace browser-owned strong prompts with one consistent, red, in-app second-confirmation experience across the workspace.

## Requirements

- Use one shared dialog for existing high-impact submissions: property, room, and tenant deletion; debt settlement; batch reminder send. Remove every workspace `window.confirm`/`alert` prompt used for these actions.
- Add the same second-confirmation interaction to committed payer-relation removal and end-occupancy actions, which currently submit immediately.
- The current task's allocation-share revoke and deferral confirmations use the same dialog component.
- Explain the action's target and consequence, with a neutral cancel and an explicit red confirm action. A failed or canceled confirmation must not submit.
- Preserve dedicated full-source revoke and cash-receipt void review pages that already require a separate, detailed confirmation step; do not add a third prompt. Removing an unsaved form row needs no confirmation.
- Preserve form validation, exact submitter attributes, accessible keyboard/focus behavior, and desktop/mobile layout.

## Acceptance Criteria

- [ ] No workspace strong-confirmation action opens a browser-native `confirm()` or `alert()`.
- [ ] Desktop and mobile deletion/settlement/send actions show the same red in-app dialog, and cancel leaves data unchanged.
- [ ] Confirm submits exactly once using the intended form and submit button, including an external `form=` button and a button with `formaction`.
- [ ] Escape cancels, focus returns to the trigger, and text is accessible without relying on red color alone.
- [ ] Payer-relation removal and end-occupancy show the same second-confirmation UI.

## Dependency

None. The transaction review/search child depends on this component for its revoke and defer dialogs.
