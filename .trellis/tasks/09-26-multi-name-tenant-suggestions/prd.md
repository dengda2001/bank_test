# 补全多人名流水的租客匹配建议

## Goal

Show Durga Nagendra as a manual tenant suggestion for bank payer `DURGA NAGENDRA GONUGUNTA` on transaction `IE26090426926842`.

## Requirements

- Include plausible tenant names when a bank payer name contains additional given or family names, even when another candidate already passes the full-name similarity threshold.
- Keep suggestions as navigation aids. Name similarity alone must never select a tenant, create a payer relationship, or allocate rent.
- Keep generic banking words from creating suggestions and preserve the existing behavior for payer names whose source is not confirmed.

## Acceptance Criteria

- [x] `DURGA NAGENDRA GONUGUNTA` suggests both Durga Nagendra and Nagendra Gonugunta; Durga is visible as a selectable suggestion.
- [x] Existing close-name and distinctive-word suggestion cases continue to work, without duplicate suggestions.
- [x] Targeted regression tests and the Go package tests pass.

## Notes

- Screenshot shows a confirmed bank payer name, an empty tenant selection, and only Nagendra Gonugunta in the suggestion area.
- The bundled Rosewood data has Durga Nagendra on the September rent plan and Nagendra Gonugunta on the May–June plan for room 4. Live database state was not inspected.
