package main

import (
	"html/template"
	"net/http"
	"time"
)

func (a *app) handleRentDashboard(w http.ResponseWriter, r *http.Request) {
	if !a.requireAuth(w, r) {
		return
	}
	period := r.URL.Query().Get("period")
	periodMonth, err := parsePeriodMonth(period)
	if err != nil {
		periodMonth = monthStart(time.Now().UTC())
	}
	data := rentDashboardPageData{
		Username:    a.cfg.AdminUsername,
		Environment: a.cfg.Environment,
		ActivePage:  "rent-dashboard",
		Period:      periodMonth.Format("2006-01"),
		Message:     r.URL.Query().Get("message"),
		Error:       r.URL.Query().Get("error"),
	}
	if period != "" {
		if _, err := parsePeriodMonth(period); err != nil {
			data.Error = "invalid_period"
		}
	}

	if userID, ok := a.currentUserID(r); ok && a.db != nil {
		service := newTransactionService(a.db)
		if err := service.reconcileTransactions(r.Context(), userID); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		summary, err := newObligationService(a.db).summarizeRentDashboard(r.Context(), userID, periodMonth)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		data.Rows = summary.Rows
		currency := firstNonEmpty(summary.Currency, "EUR")
		data.ExpectedTotal = formatMoney(centsToMoney(summary.ExpectedCents), currency, 2)
		data.PaidTotal = formatMoney(centsToMoney(summary.PaidCents), currency, 2)
		data.BalanceTotal = formatMoney(centsToMoney(summary.BalanceCents), currency, 2)
		data.ExpenseTotal = formatMoney(centsToMoney(summary.ExpenseCents), currency, 2)
		data.OpenCount = summary.OpenCount
		data.PartialCount = summary.PartialCount
		data.PaidCount = summary.PaidCount
		data.ReviewCount = summary.ReviewCount
		data.TenantCount = summary.TenantCount
		data.IncomeCount = summary.IncomeCount
		data.ExpenseCount = summary.ExpenseCount
	} else {
		tenants, _ := a.loadTenants()
		expenses, _ := a.loadExpenses()
		data.TenantCount = len(tenants)
		data.ExpenseCount = len(expenses)
		data.ExpenseTotal = formatMoney(sumExpenses(expenses), "EUR", 2)
		data.ExpectedTotal = formatMoney(sumTenantRent(tenants), "EUR", 2)
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := rentDashboardTemplate.Execute(w, data); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
}

var rentDashboardTemplate = template.Must(template.New("rent-dashboard").Parse(`<!doctype html>
<html lang="en">
<head>
  <meta charset="utf-8">
  <meta name="viewport" content="width=device-width, initial-scale=1">
  <title>RentOps Dashboard</title>
  <style>` + workspacePageCSS + `
    .dashboard-summary { display: grid; grid-template-columns: repeat(4, minmax(0, 1fr)); gap: 12px; }
    .dashboard-toolbar { display: flex; align-items: end; justify-content: space-between; gap: 16px; margin-bottom: 18px; }
    .dashboard-toolbar form { display: flex; align-items: end; gap: 10px; }
    .dashboard-toolbar label { margin: 0; min-width: 150px; }
    .dashboard-toolbar input[type="month"] { min-height: 42px; border-radius: 14px; padding: 9px 12px; background: rgba(255,255,255,0.07); box-shadow: inset 0 1px 0 rgba(255,255,255,0.10), 0 0 0 1px rgba(255,255,255,0.03); }
    .dashboard-toolbar input[type="month"]:hover { border-color: rgba(255,255,255,0.22); background: rgba(255,255,255,0.09); }
    .dashboard-toolbar input[type="month"]:focus { border-color: rgba(104,114,217,0.85); background: rgba(255,255,255,0.08); }
    .status.open, .status.overdue, .status.partial, .status.paid, .status.needs_review { border-radius: 999px; padding: 5px 9px; display: inline-block; font-size: 12px; }
    .status.open { color: #d8dcff; background: rgba(104,114,217,0.12); }
    .status.overdue, .status.needs_review { color: #ffd0ce; background: rgba(255,139,134,0.10); }
    .review-link { color: #ffd0ce; text-decoration: none; border-bottom: 1px dashed currentColor; }
    .review-link:hover { color: #fff; }
    .status.partial { color: #ffe0a7; background: rgba(233,184,114,0.10); }
    .status.paid { color: #cbffe1; background: rgba(125,211,168,0.10); }
    .rent-row { cursor: pointer; }
    .rent-row:hover, .rent-row:focus { background: rgba(255,255,255,0.035); outline: none; }
    .rent-row td:first-child::after { content: " +"; margin-left: 6px; color: var(--foreground-muted); font: 700 12px var(--mono); }
    .rent-row[aria-expanded="true"] td:first-child::after { content: " -"; }
    .rent-details td { padding: 0; background: rgba(255,255,255,0.025); }
    .payment-list { padding: 14px 18px 16px 32px; border-top: 1px solid rgba(255,255,255,0.045); }
    .payment-list h3 { margin: 0 0 10px; font-size: 12px; color: var(--foreground); }
    .payment-item { display: grid; grid-template-columns: 140px 170px minmax(180px, 1fr) minmax(160px, 1fr) 100px; gap: 12px; padding: 9px 0; border-bottom: 1px solid rgba(255,255,255,0.04); color: var(--foreground-subtle); font-size: 12px; }
    .payment-item:last-child { border-bottom: 0; }
    .payment-item .amount { font-size: 13px; }
    @media (max-width: 760px) { .payment-item { grid-template-columns: 1fr 1fr; } .payment-item .payment-description { grid-column: 1 / -1; } }
    @media (max-width: 900px) { .dashboard-summary { grid-template-columns: repeat(2, minmax(0, 1fr)); } }
    @media (max-width: 640px) { .dashboard-toolbar { align-items: stretch; flex-direction: column; } .dashboard-toolbar form { align-items: stretch; } .dashboard-toolbar label { flex: 1; } }
  </style>
</head>
<body>
  <div class="app">
    <aside class="sidebar" aria-label="Main navigation">
      <div class="side-brand"><div class="mark">R</div><div><div class="brand-title">RentOps</div><div class="brand-meta">{{.Environment}} workspace</div></div></div>
      <nav class="nav">
        <a href="/rent-dashboard" class="active"><span class="glyph">DB</span><span>Rent Dashboard</span><span class="nav-count">{{.TenantCount}}</span></a>
        <a href="/billing"><span class="glyph">TX</span><span>Transactions</span><span class="nav-count">{{.IncomeCount}}</span></a>
        <a href="/tenants"><span class="glyph">TN</span><span>Tenants</span><span class="nav-count">{{.TenantCount}}</span></a>
        <a href="/expenses"><span class="glyph">EX</span><span>Expenses</span><span class="nav-count">{{.ExpenseCount}}</span></a>
      </nav>
      <div class="side-foot">Signed in as {{.Username}}<br>Monthly rent workspace.</div>
    </aside>
    <main class="content">
      <header class="topbar">
        <div><div class="brand-title">Rent dashboard</div><h1>{{.Period}}</h1></div>
        <form method="post" action="/logout"><button class="btn danger" type="submit">Sign out</button></form>
      </header>
      {{if eq .Error "invalid_period"}}<div class="notice error">The selected month is invalid.</div>{{end}}
      {{if eq .Message "rent_confirmed"}}<div class="notice ok">Rent payment confirmed.</div>{{end}}
      <div class="dashboard-toolbar">
        <div><div class="label">Monthly review</div><div class="tiny">Expected rent, received payments, balances and expenses for one month.</div></div>
        <form method="get" action="/rent-dashboard">
          <label for="period">Month<input id="period" name="period" type="month" value="{{.Period}}" onchange="this.form.submit()"></label>
          <button class="btn primary" type="submit">View month</button>
        </form>
      </div>
      <section class="dashboard-summary" aria-label="Monthly rent summary">
        <div class="panel metric"><div class="label">Expected</div><strong>{{.ExpectedTotal}}</strong><span>active tenant obligations</span></div>
        <div class="panel metric"><div class="label">Paid</div><strong>{{.PaidTotal}}</strong><span>{{.PaidCount}} fully paid</span></div>
        <div class="panel metric"><div class="label">Balance</div><strong>{{.BalanceTotal}}</strong><span>{{.OpenCount}} open, {{.PartialCount}} partial</span></div>
        <div class="panel metric"><div class="label">Expenses</div><strong>{{.ExpenseTotal}}</strong><span>{{.ExpenseCount}} outgoing transactions</span></div>
      </section>
      <section class="panel surface" aria-labelledby="rent-status-title">
        <div class="panel-head"><h2 id="rent-status-title">Tenant rent status</h2><a class="tiny review-link" href="/billing?match_status=needs_review">{{.ReviewCount}} needs review</a></div>
        {{if .Rows}}
        <div class="table-wrap"><table>
          <thead><tr><th>Tenant</th><th>Room</th><th>Due</th><th>Expected</th><th>Paid</th><th>Balance</th><th>Status</th></tr></thead>
          <tbody>{{range .Rows}}
            <tr class="rent-row" tabindex="0" role="button" aria-expanded="false" aria-controls="rent-details-{{.ObligationID}}" data-details-target="rent-details-{{.ObligationID}}"><td><strong>{{.TenantName}}</strong></td><td>{{if .RoomLabel}}{{.RoomLabel}}<br>{{end}}{{.RoomAddress}}</td><td class="mono">{{.DueDate}}</td><td class="amount">{{.ExpectedAmount}}</td><td class="amount">{{.PaidAmount}}</td><td class="amount">{{.BalanceAmount}}</td><td><span class="status {{.Status}}">{{.StatusLabel}}</span></td></tr>
            <tr id="rent-details-{{.ObligationID}}" class="rent-details" hidden><td colspan="7"><div class="payment-list"><h3>Payment details</h3>{{if .Payments}}{{range .Payments}}<div class="payment-item"><span class="amount">{{.AmountDisplay}}</span><span class="mono">{{.DateDisplay}}</span><span class="payment-description">{{.Description}}</span><span class="mono">REF: {{.Reference}}</span><span class="mono">{{.ConfirmationSource}}</span></div>{{end}}{{else}}<div class="tiny">No payment details for this month.</div>{{end}}</div></td></tr>
          {{end}}</tbody>
        </table></div>
        {{else}}<div class="empty">No monthly obligations yet. Add an active tenant with a rent start date to generate this month.</div>{{end}}
      </section>
    </main>
  </div>
  <script>
    for (const row of document.querySelectorAll("[data-details-target]")) {
      const details = document.getElementById(row.dataset.detailsTarget);
      const toggle = () => {
        const expanded = row.getAttribute("aria-expanded") === "true";
        row.setAttribute("aria-expanded", String(!expanded));
        details.hidden = expanded;
      };
      row.addEventListener("click", toggle);
      row.addEventListener("keydown", (event) => {
        if (event.key === "Enter" || event.key === " ") { event.preventDefault(); toggle(); }
      });
    }
  </script>
</body>
</html>
`))
