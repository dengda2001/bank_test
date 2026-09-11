package main

import "html/template"

var billingTemplate = template.Must(template.New("billing").Parse(`<!doctype html>
<html lang="en">
<head>
  <meta charset="utf-8">
  <meta name="viewport" content="width=device-width, initial-scale=1">
  <title>RentOps Transactions</title>
  <style>` + workspacePageCSS + `
    .filterbar { display: grid; grid-template-columns: 150px 170px 180px auto; align-items: end; gap: 10px; margin-bottom: 18px; }
    .filterbar label { margin: 0; }
    .filter-actions { display: flex; align-items: end; gap: 8px; }
    .filterbar .btn { min-height: 40px; margin-top: 0; }
    .filterbar .btn.subtle { color: var(--foreground-muted); background: transparent; box-shadow: none; }
    .filterbar .btn.subtle:hover { color: var(--foreground); background: rgba(255,255,255,0.055); }
    .actions { display: flex; align-items: center; justify-content: flex-end; flex-wrap: wrap; gap: 8px; }
    .actions form { margin: 0; }
    .actions .btn.primary { margin-top: 0; }
    .status { display: inline-block; border-radius: 999px; padding: 5px 9px; font-size: 12px; white-space: nowrap; }
    .status.matched { color: #cbffe1; background: rgba(125,211,168,0.10); }
    .status.candidate { color: #ffe0a7; background: rgba(233,184,114,0.10); }
    .status.unmatched { color: #d8dcff; background: rgba(104,114,217,0.12); }
    .status.needs_review { color: #ffd0ce; background: rgba(255,139,134,0.10); }
    .status-link { text-decoration: none; cursor: pointer; }
    .status-link:hover { border-color: currentColor; }
    .description { max-width: 260px; color: var(--foreground); overflow-wrap: anywhere; }
    .transaction-table { min-width: 1040px; }
    .direction { color: var(--foreground-muted); font: 700 11px var(--mono); text-transform: uppercase; letter-spacing: 0.08em; }
    .direction.income { color: var(--positive); }
    .direction.expense { color: var(--danger); }
    .match-metadata { display: grid; gap: 6px; }
    .confirm-form { margin-top: 8px; }
    .confirm-form .btn { min-height: 30px; padding: 0 9px; font-size: 12px; }
    .bind-form { display: flex; align-items: center; gap: 7px; margin-top: 8px; min-width: 250px; }
    .bind-form select { min-height: 30px; padding: 5px 28px 5px 8px; font-size: 12px; }
    .bind-form .btn { min-height: 30px; padding: 0 9px; font-size: 12px; white-space: nowrap; }
    .bind-empty { margin-top: 8px; }
    @media (max-width: 820px) { .filterbar { grid-template-columns: repeat(2, minmax(0, 1fr)); } }
    @media (max-width: 480px) { .filterbar { grid-template-columns: 1fr; } }
  </style>
</head>
<body>
  <div class="app">
    <aside class="sidebar" aria-label="Main navigation">
      <div class="side-brand"><div class="mark">R</div><div><div class="brand-title">RentOps</div><div class="brand-meta">{{.Environment}} workspace</div></div></div>
      <nav class="nav">
        <a href="/rent-dashboard"><span class="glyph">DB</span><span>Rent Dashboard</span><span class="nav-count">{{.TenantCount}}</span></a>
        <a href="/billing" class="active"><span class="glyph">TX</span><span>Transactions</span><span class="nav-count">{{.IncomeCount}}</span></a>
        <a href="/tenants"><span class="glyph">TN</span><span>Tenants</span><span class="nav-count">{{.TenantCount}}</span></a>
        <a href="/expenses"><span class="glyph">EX</span><span>Expenses</span><span class="nav-count">{{.ExpenseCount}}</span></a>
      </nav>
      <div class="side-foot">Signed in as {{.Username}}<br>Bank statement and rent matching.</div>
    </aside>
    <main class="content">
      <header class="topbar">
        <div><div class="brand-title">Transactions</div><h1>Bank statement</h1></div>
        <div class="actions">
          {{if .Connected}}<a class="btn primary" href="/refresh">Refresh bank data</a>{{else}}<a class="btn primary" href="/login">Bind bank account</a>{{end}}
          <form method="post" action="/import-legacy"><button class="btn" type="submit">Import legacy files</button></form>
          <form method="post" action="/logout"><button class="btn danger" type="submit">Sign out</button></form>
        </div>
      </header>
      {{if .NeedsReconnect}}<div class="notice error">Bank access needs a new authorization.</div>{{end}}
      {{if eq .Error "data_fetch_failed"}}<div class="notice error">Bank data refresh failed. Existing transactions were kept.</div>{{end}}
      {{if eq .Error "confirmation_failed"}}<div class="notice error">This rent payment could not be confirmed. Check the tenant, amount, and open month.</div>{{end}}
      {{if eq .Error "invalid_confirmation"}}<div class="notice error">The confirmation request is invalid.</div>{{end}}
      {{if eq .Error "invalid_filter"}}<div class="notice error">One or more transaction filters are invalid.</div>{{end}}
      {{if eq .Message "bank_connected"}}<div class="notice ok">Bank account connected and transactions imported.</div>{{end}}
      {{if eq .Message "refreshed"}}<div class="notice ok">Bank data refreshed.</div>{{end}}
      {{if eq .Message "rent_confirmed"}}<div class="notice ok">Rent payment confirmed and tenant payer ID updated when available.</div>{{end}}
      {{if eq .Message "legacy_imported"}}<div class="notice ok">Legacy JSON and JSONL files imported.</div>{{end}}
      {{if eq .Error "legacy_import_failed"}}<div class="notice error">Legacy import failed. Check the configured source files.</div>{{end}}
      <form class="filterbar" method="get" action="/billing" aria-label="Transaction filters">
        <label for="period">Month<input id="period" name="period" type="month" value="{{.PeriodFilter}}"></label>
        <label for="direction">Type<select id="direction" name="direction"><option value="">All</option><option value="income" {{if eq .DirectionFilter "income"}}selected{{end}}>Income</option><option value="expense" {{if eq .DirectionFilter "expense"}}selected{{end}}>Expense</option></select></label>
        <label for="match_status">Matching<select id="match_status" name="match_status"><option value="">All statuses</option><option value="matched" {{if eq .MatchStatusFilter "matched"}}selected{{end}}>Matched</option><option value="candidate" {{if eq .MatchStatusFilter "candidate"}}selected{{end}}>Name candidate</option><option value="unmatched" {{if eq .MatchStatusFilter "unmatched"}}selected{{end}}>Unmatched</option><option value="needs_review" {{if eq .MatchStatusFilter "needs_review"}}selected{{end}}>Needs review</option></select></label>
        <div class="filter-actions"><button class="btn" type="submit">Apply filters</button><a class="btn subtle" href="/billing">Clear filters</a></div>
      </form>
      <section class="panel surface" aria-labelledby="statement-title">
        <div class="panel-head"><h2 id="statement-title">Statement entries</h2><span class="tiny">{{.LastSync}}</span></div>
        {{if .TransactionRows}}
        <div class="table-wrap"><table class="transaction-table">
          <thead><tr><th>Type</th><th>Payer / source</th><th>Amount</th><th>Date</th><th>Description</th><th>Account</th><th>Matching</th></tr></thead>
          <tbody>{{range .TransactionRows}}
            <tr>
              <td><span class="direction {{.Direction}}">{{.DirectionLabel}}</span></td>
              <td><div class="match-metadata"><strong>{{.PayerName}}</strong><span class="mono">ID: {{.PayerID}}</span></div></td>
              <td><span class="{{if eq .Direction "expense"}}amount expense{{else}}amount{{end}}">{{.AmountDisplay}}</span></td>
              <td class="mono">{{.DateDisplay}}</td>
              <td><div class="description">{{.Description}}</div><div class="mono">REF: {{.Reference}}</div></td>
              <td>{{.AccountName}}<br><span class="mono">{{.AccountID}}</span></td>
              <td><a class="status status-link {{.MatchStatus}}" href="/billing?match_status={{.MatchStatus}}" title="Filter by {{.MatchStatusLabel}}">{{.MatchStatusLabel}}</a>{{if .CanConfirm}}<div class="tiny">Candidate: {{.CandidateTenantName}}</div><form class="confirm-form" method="post" action="/billing/confirm"><input type="hidden" name="transaction_id" value="{{.ID}}"><input type="hidden" name="tenant_id" value="{{.CandidateTenantID}}"><button class="btn" type="submit">Confirm rent</button></form>{{else if and (eq .Direction "income") (ne .MatchStatus "matched")}}{{if $.TenantOptions}}<form class="bind-form" method="post" action="/billing/confirm"><input type="hidden" name="transaction_id" value="{{.ID}}"><select name="tenant_id" aria-label="Bind transaction to tenant" required><option value="">Bind tenant...</option>{{range $.TenantOptions}}<option value="{{.ID}}">{{.Name}}</option>{{end}}</select><button class="btn" type="submit">Bind</button></form>{{else}}<div class="tiny bind-empty">Add a tenant first to bind this income.</div>{{end}}{{end}}</td>
            </tr>
          {{end}}</tbody>
        </table></div>
        {{else}}<div class="empty">No transactions match the selected filters.</div>{{end}}
      </section>
    </main>
  </div>
</body>
</html>
`))
