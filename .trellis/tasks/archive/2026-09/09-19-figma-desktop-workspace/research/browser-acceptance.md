# Desktop browser acceptance

Date: 2026-09-19

The latest workspace server was exercised in Chrome through the local DevTools
endpoint. The authenticated desktop shell was checked at both target sizes:

- `1366×768`: sidebar `236px`, topbar `64px`, document `scrollWidth=1366`,
  client width `1366`.
- `1440×900`: sidebar `236px`, topbar `64px`, document `scrollWidth=1440`,
  client width `1440`.

The dashboard exposes the four primary Figma metrics (本月应收、已收租金、剩余未收、房产支出)
and keeps 待处理流水 as a secondary linked entry. The room creation page was
opened at 1440px and verified to render the property binding select, room label,
effective month and save action without horizontal overflow. The tenant create
page was verified to render the optional room binding select and structured
tenant fields.

Screenshots captured during this check:

- `dashboard-1366.png`
- `dashboard-1440.png`
- `rooms-create-1440.png`

Automated checks passed for the affected package slices and `go vet`; the full
package still contains two pre-existing fixture failures in
`tenantActiveInMonth` tests because those fixtures omit `MonthlyRentCents`.
