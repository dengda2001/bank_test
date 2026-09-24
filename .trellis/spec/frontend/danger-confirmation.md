# Shared danger confirmation

## 1. Scope / Trigger

Use the shared dialog for destructive workspace actions that need a second decision before submitting. The shell partial `web/templates/partials/workspace-nav.html` renders it, `web/static/js/danger-confirm.js` owns its behavior, and `workspace.css` owns its appearance. Detailed revoke and cash-void review pages already serve as the second step and do not need another dialog.

## 2. Signatures

Declarative form submitter:

```html
<button type="submit" data-confirm="true"
        data-confirm-title="删除房间"
        data-confirm-message="删除房间「…」及其关联记录？此操作无法撤销。"
        data-confirm-label="确认删除房间">删除房间</button>
```

Imperative action:

```js
const accepted = await window.RentOpsConfirm.open({
  title: "撤销分配", message: "这笔分配将从责任中移除。",
  confirmLabel: "确认撤销", trigger: button,
});
if (accepted) await revokeAllocation();
```

`open` returns `Promise<boolean>`; its options are plain text. It sets content with `textContent`.

## 3. Contracts

- Only the actual `SubmitEvent.submitter` controls whether the dialog opens. An unmarked button in the same form submits normally.
- The browser validates the form before the `submit` event. A failed constraint check never opens the dialog.
- Confirm calls `form.requestSubmit(originalSubmitter)` once with a one-use bypass. This preserves an external `form=` button, `formaction`, and the submitter's `name`/`value`.
- Cancel and Escape resolve `false` and leave the form untouched. Closing restores focus to the trigger if it remains connected.
- The dialog's confirm label and impact copy name the action, target, and consequence. Dynamic transaction actions use the imperative API and handle their own request after `true`.

## 4. Validation & Error Matrix

| Condition | Result |
|---|---|
| Form invalid | Browser validation; no dialog or submission |
| User cancels or presses Escape | `false`; no submission |
| Dialog already open | A new `open` call resolves `false` |
| Trigger removed before acceptance | Declarative form is not resubmitted |
| User accepts | Original submitter submits once |

## 5. Good / Base / Bad Cases

- Good: a marked external send button with `formaction` shows its own confirmation and submits to its override route.
- Base: an unmarked preview button on that form submits directly.
- Bad: searching the form for any `[data-confirm]` button would also intercept preview; calling `form.submit()` would bypass validation and lose the submitter override.

## 6. Tests Required

Browser checks should assert invalid-form suppression, cancel and Escape behavior, focus return, one accepted submission, and the accepted event's original submitter and `formaction`. Render checks should assert target-specific metadata on each destructive action. Search runtime sources for native `confirm()` and `alert()` when adding strong actions.

## 7. Wrong vs Correct

```js
// Wrong: selects another button in the form and loses submitter semantics.
if (form.querySelector("[data-confirm]")) form.submit();

// Correct: the shared listener reads event.submitter, opens the dialog,
// then calls form.requestSubmit(event.submitter) with a one-use bypass.
```
