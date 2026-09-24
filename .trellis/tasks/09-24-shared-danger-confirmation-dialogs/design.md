# Design

Provide a global confirmation component through the shared workspace shell. The component creates an HTML `<dialog>` in the document top layer and styles it with the existing danger, border, and surface tokens. It accepts declarative `data-confirm` form submitters and an imperative API for dynamic transaction-review details. All dynamic text enters via `textContent`, never HTML. The panel contains a danger icon/heading, target-specific impact copy, cancel, and red confirm. Use dialog focus management, Escape/cancel, and mobile sizing.

A delegated `submit` listener checks `event.submitter`, not any button in the form. It prevents the initial submission, opens the dialog, then calls `form.requestSubmit(originalSubmitter)` once after confirmation with a bypass guard. This preserves validation, `formaction`, button names/values, and external `form=` submitters. Remove page-local native confirmation listeners in `rent_collection_pages.go` and the shell's old `window.confirm` handler. Mark payer-relation removal and end-occupancy forms for confirmation. Dedicated cash-void and whole-source revoke review pages already provide their own confirmation step and remain separate.

The transaction review uses the imperative dialog API so its evidence share details and defer explanation match the same visual component. It owns the subsequent fetch/redirect and drawer refresh.
