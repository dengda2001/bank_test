package main

import "html/template"

var billingTemplate = template.Must(template.New("billing").Parse(`<!doctype html>
<html lang="zh-CN">
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
    .month-choice-form { display: grid; gap: 7px; margin-top: 8px; min-width: 290px; }
    .month-choice-form select { min-height: 34px; font-size: 12px; }
    .month-choice-form .btn { min-height: 32px; padding: 0 9px; font-size: 12px; }
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
        <a href="/rent-dashboard"><span class="glyph">总</span><span>月度总览</span><span class="nav-count">{{.TenantCount}}</span></a>
        <a href="/billing" class="active"><span class="glyph">流</span><span>银行流水</span><span class="nav-count">{{.IncomeCount}}</span></a>
        <a href="/tenants"><span class="glyph">租</span><span>租客管理</span><span class="nav-count">{{.TenantCount}}</span></a>
        <a href="/expenses"><span class="glyph">支</span><span>支出记录</span><span class="nav-count">{{.ExpenseCount}}</span></a>
      </nav>
      <div class="side-foot">当前用户：{{.Username}}<br>银行流水与租金关联</div>
    </aside>
    <main class="content">
      <header class="topbar">
        <div><div class="brand-title">银行流水</div><h1>收款与支出</h1></div>
        <div class="actions">
          {{if .Connected}}<a class="btn primary" href="/refresh">刷新银行数据</a>{{else}}<a class="btn primary" href="/login">连接银行账户</a>{{end}}
          <form method="post" action="/import-legacy"><button class="btn" type="submit">导入旧数据</button></form>
          <form method="post" action="/logout"><button class="btn danger" type="submit">退出登录</button></form>
        </div>
      </header>
      {{if .NeedsReconnect}}<div class="notice error">银行授权已失效，请重新连接。</div>{{end}}
      {{if eq .Error "data_fetch_failed"}}<div class="notice error">银行数据刷新失败，已有流水已保留。</div>{{end}}
      {{if eq .Error "confirmation_failed"}}<div class="notice error">这笔租金无法确认，请检查租客、金额和月份。</div>{{end}}
      {{if eq .Error "invalid_confirmation"}}<div class="notice error">关联请求无效。</div>{{end}}
      {{if eq .Error "invalid_filter"}}<div class="notice error">筛选条件无效。</div>{{end}}
      {{if eq .Message "bank_connected"}}<div class="notice ok">银行账户已连接，流水已导入。</div>{{end}}
      {{if eq .Message "refreshed"}}<div class="notice ok">银行数据已刷新。</div>{{end}}
      {{if eq .Message "rent_confirmed"}}<div class="notice ok">租金已确认，符合条件的同名流水也已关联。</div>{{end}}
      {{if eq .Message "legacy_imported"}}<div class="notice ok">旧版 JSON 和 JSONL 数据已导入。</div>{{end}}
      {{if eq .Error "legacy_import_failed"}}<div class="notice error">旧数据导入失败，请检查源文件。</div>{{end}}
      <form class="filterbar" method="get" action="/billing" aria-label="Transaction filters">
        <label for="period">月份<input id="period" name="period" type="month" value="{{.PeriodFilter}}"></label>
        <label for="direction">类型<select id="direction" name="direction"><option value="">全部</option><option value="income" {{if eq .DirectionFilter "income"}}selected{{end}}>收入</option><option value="expense" {{if eq .DirectionFilter "expense"}}selected{{end}}>支出</option></select></label>
        <label for="match_status">关联状态<select id="match_status" name="match_status"><option value="">全部状态</option><option value="matched" {{if eq .MatchStatusFilter "matched"}}selected{{end}}>已关联</option><option value="candidate" {{if eq .MatchStatusFilter "candidate"}}selected{{end}}>待确认</option><option value="unmatched" {{if eq .MatchStatusFilter "unmatched"}}selected{{end}}>未关联</option><option value="needs_review" {{if eq .MatchStatusFilter "needs_review"}}selected{{end}}>需处理</option></select></label>
        <div class="filter-actions"><button class="btn" type="submit">应用筛选</button><a class="btn subtle" href="/billing">清除筛选</a></div>
      </form>
      <section class="panel surface" aria-labelledby="statement-title">
        <div class="panel-head"><h2 id="statement-title">流水明细</h2><span class="tiny">{{.LastSync}}</span></div>
        {{if .TransactionRows}}
        <div class="table-wrap"><table class="transaction-table">
          <thead><tr><th>类型</th><th>付款人／来源</th><th>金额</th><th>日期</th><th>描述</th><th>账户</th><th>关联</th></tr></thead>
          <tbody>{{range .TransactionRows}}
            <tr>
              <td><span class="direction {{.Direction}}">{{.DirectionLabel}}</span></td>
              <td><div class="match-metadata"><strong>{{.PayerName}}</strong><span class="mono">ID: {{.PayerID}}</span></div></td>
              <td><span class="{{if eq .Direction "expense"}}amount expense{{else}}amount{{end}}">{{.AmountDisplay}}</span></td>
              <td class="mono">{{.DateDisplay}}</td>
              <td><div class="description">{{.Description}}</div><div class="mono">REF: {{.Reference}}</div></td>
              <td>{{.AccountName}}<br><span class="mono">{{.AccountID}}</span></td>
              <td><a class="status status-link {{.MatchStatus}}" href="/billing?match_status={{.MatchStatus}}" title="筛选：{{.MatchStatusLabel}}">{{.MatchStatusLabel}}</a>{{if .NeedsMonthChoice}}<div class="tiny">已关联：请确认租金月份</div><form class="month-choice-form" method="post" action="/billing/confirm"><input type="hidden" name="transaction_id" value="{{.ID}}"><input type="hidden" name="tenant_id" value="{{.TenantID}}"><select name="period" aria-label="选择租金月份" required><option value="">选择月份...</option>{{range .MonthOptions}}<option value="{{.Period}}">{{.Label}} · 应收 {{.Expected}} · 已收 {{.Paid}} · 未收 {{.Remaining}}</option>{{end}}</select><button class="btn" type="submit">确认月份</button></form>{{else if .CanConfirm}}<div class="tiny">候选租客：{{.CandidateTenantName}}</div><form class="confirm-form" method="post" action="/billing/confirm"><input type="hidden" name="transaction_id" value="{{.ID}}"><input type="hidden" name="tenant_id" value="{{.CandidateTenantID}}"><button class="btn" type="submit">关联并记住</button></form>{{else if and (eq .Direction "income") (ne .MatchStatus "matched")}}{{if $.TenantOptions}}<form class="bind-form" method="post" action="/billing/confirm"><input type="hidden" name="transaction_id" value="{{.ID}}"><select name="tenant_id" aria-label="关联流水到租客" required><option value="">选择租客...</option>{{range $.TenantOptions}}<option value="{{.ID}}">{{.Name}}</option>{{end}}</select><button class="btn" type="submit">关联并记住</button></form>{{else}}<div class="tiny bind-empty">请先添加租客，再关联收入流水。</div>{{end}}{{end}}</td>
            </tr>
          {{end}}</tbody>
        </table></div>
        {{else}}<div class="empty">没有符合当前筛选条件的流水。</div>{{end}}
      </section>
    </main>
  </div>
</body>
</html>
`))
