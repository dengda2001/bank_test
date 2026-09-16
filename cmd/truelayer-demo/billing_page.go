package main

import "html/template"

var billingTemplate = template.Must(template.New("billing").Parse(`<!doctype html>
<html lang="zh-CN">
<head>
  <meta charset="utf-8">
  <meta name="viewport" content="width=device-width, initial-scale=1">
  <title>RentOps Transactions</title>
  <style>` + workspacePageCSS + workspaceCalendarCSS + `
    .filterbar { display: grid; grid-template-columns: repeat(4, minmax(130px, 1fr)); align-items: end; gap: 10px; margin-bottom: 18px; }
    .filterbar label { margin: 0; }
    .filterbar input[type="month"] { min-height: 40px; border-radius: 10px; }
    .filter-actions { display: flex; align-items: end; gap: 8px; }
    .filterbar .btn { min-height: 40px; margin-top: 0; }
    .filterbar .btn.subtle { color: var(--foreground-muted); background: transparent; }
    .filterbar .btn.subtle:hover { color: var(--foreground); background: var(--surface-muted); }
    .actions { display: flex; align-items: center; justify-content: flex-end; flex-wrap: wrap; gap: 8px; }
    .actions form { margin: 0; }
    .actions .btn.primary { margin-top: 0; }
    .status { display: inline-block; border-radius: 999px; padding: 5px 9px; font-size: 12px; white-space: nowrap; }
    .status.matched { color: #065f46; background: #d1fae5; }
    .status.candidate { color: #92400e; background: #fef3c7; }
    .status.unmatched { color: #1e40af; background: #dbeafe; }
    .status.needs_review { color: #991b1b; background: #fee2e2; }
    .status.partial { color: #92400e; background: #fef3c7; }
    .status.ignored { color: var(--foreground-muted); background: #e5e7eb; }
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
    .action-form { display: grid; gap: 6px; margin-top: 8px; min-width: 190px; }
    .action-form input { min-height: 30px; padding: 5px 8px; font-size: 12px; }
    .action-form .btn { min-height: 30px; padding: 0 9px; font-size: 12px; }
    .allocation-form { display: grid; gap: 6px; margin-top: 8px; min-width: 230px; }
    .allocation-form select, .allocation-form input { min-height: 30px; padding: 5px 8px; font-size: 12px; }
    .allocation-form .btn { min-height: 30px; padding: 0 9px; font-size: 12px; }
    .allocation-line { display: grid; gap: 6px; padding-bottom: 6px; border-bottom: 1px solid var(--border); }
    .allocation-add { color: var(--accent); background: transparent; }
    .allocation-details { margin-top: 8px; }
    .allocation-details summary { color: var(--accent); cursor: pointer; font-size: 12px; }
    .transaction-table tr.expense .allocation-details { display: none; }
    .pagination { display: flex; justify-content: flex-end; align-items: center; gap: 10px; margin-top: 12px; }
    .pagination a { color: var(--accent); text-decoration: none; }
    @media (max-width: 820px) { .filterbar { grid-template-columns: repeat(2, minmax(0, 1fr)); } }
    @media (max-width: 480px) { .filterbar { grid-template-columns: 1fr; } }
  </style>
  <script>` + workspaceCalendarScript + `</script>
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
          <form method="post" action="/billing/payer/preview"><button class="btn" type="submit">历史付款人预览</button></form>
          <form method="post" action="/import-legacy"><button class="btn" type="submit">导入旧数据</button></form>
          <form method="post" action="/logout"><button class="btn danger" type="submit">退出登录</button></form>
        </div>
      </header>
      {{if .NeedsReconnect}}<div class="notice error">银行授权已失效，请重新连接。</div>{{end}}
      {{if eq .Error "data_fetch_failed"}}<div class="notice error">银行数据刷新失败，已有流水已保留。</div>{{end}}
      {{if eq .Error "confirmation_failed"}}<div class="notice error">这笔租金无法确认，请检查租客、金额和月份。</div>{{end}}
      {{if eq .Error "invalid_confirmation"}}<div class="notice error">关联请求无效。</div>{{end}}
      {{if eq .Error "invalid_allocation"}}<div class="notice error">归类请求无效。</div>{{end}}
      {{if eq .Error "allocation_failed"}}<div class="notice error">归类失败，请检查余额、币种、租客和租金月份。</div>{{end}}
      {{if eq .Error "invalid_transaction_action"}}<div class="notice error">流水操作请求无效。</div>{{end}}
      {{if eq .Error "transaction_action_failed"}}<div class="notice error">流水操作失败，请检查当前状态和操作原因。</div>{{end}}
      {{if eq .Error "invalid_payer_confirmation"}}<div class="notice error">付款人确认请求无效。</div>{{end}}
      {{if eq .Error "payer_confirmation_failed"}}<div class="notice error">付款人确认失败，请重新检查月份和账单余额。</div>{{end}}
      {{if eq .Error "invalid_filter"}}<div class="notice error">筛选条件无效。</div>{{end}}
      {{if eq .Message "bank_connected"}}<div class="notice ok">银行账户已连接，流水已导入。</div>{{end}}
      {{if eq .Message "refreshed"}}<div class="notice ok">银行数据已刷新。</div>{{end}}
      {{if eq .Message "rent_confirmed"}}<div class="notice ok">租金已确认，符合条件的同名流水也已关联。</div>{{end}}
      {{if eq .Message "allocation_saved"}}<div class="notice ok">流水归类已保存。</div>{{end}}
      {{if eq .Message "transaction_action_saved"}}<div class="notice ok">流水操作已保存。</div>{{end}}
      {{if eq .Message "payer_confirmed"}}<div class="notice ok">历史流水已逐笔确认。</div>{{end}}
      {{if eq .Message "legacy_imported"}}<div class="notice ok">旧版 JSON 和 JSONL 数据已导入。</div>{{end}}
      {{if eq .Error "legacy_import_failed"}}<div class="notice error">旧数据导入失败，请检查源文件。</div>{{end}}
      <form class="filterbar" method="get" action="/billing" aria-label="Transaction filters">
        <label for="arrival_from">到账起<input id="arrival_from" name="arrival_from" type="date" value="{{.ArrivalFromFilter}}"></label>
        <label for="arrival_to">到账止<input id="arrival_to" name="arrival_to" type="date" value="{{.ArrivalToFilter}}"></label>
        <label for="payer">付款人<input id="payer" name="payer" type="search" value="{{.PayerFilter}}" placeholder="姓名或付款人 ID"></label>
        <label for="tenant_id">租客<select id="tenant_id" name="tenant_id"><option value="">全部租客</option>{{range .TenantOptions}}<option value="{{.ID}}" {{if eq $.TenantFilter .ID}}selected{{end}}>{{.Name}}</option>{{end}}</select></label>
        <label for="period">到账月<input id="period" name="period" type="month" value="{{.PeriodFilter}}" onchange="this.form.submit()"></label>
        <label for="rent_period">租金所属月<input id="rent_period" name="rent_period" type="month" value="{{.RentPeriodFilter}}"></label>
        <label for="allocation">入账用途<select id="allocation" name="allocation"><option value="">全部用途</option><option value="rent" {{if eq .AllocationFilter "rent"}}selected{{end}}>房租</option><option value="deposit" {{if eq .AllocationFilter "deposit"}}selected{{end}}>押金</option><option value="other_income" {{if eq .AllocationFilter "other_income"}}selected{{end}}>其他收入</option></select></label>
        <label for="direction">收支<select id="direction" name="direction"><option value="">全部</option><option value="income" {{if eq .DirectionFilter "income"}}selected{{end}}>收入</option><option value="expense" {{if eq .DirectionFilter "expense"}}selected{{end}}>支出</option></select></label>
        <label for="match_status">匹配状态<select id="match_status" name="match_status"><option value="">全部状态</option><option value="matched" {{if eq .MatchStatusFilter "matched"}}selected{{end}}>已关联</option><option value="partial" {{if eq .MatchStatusFilter "partial"}}selected{{end}}>部分关联</option><option value="candidate" {{if eq .MatchStatusFilter "candidate"}}selected{{end}}>待确认</option><option value="unmatched" {{if eq .MatchStatusFilter "unmatched"}}selected{{end}}>未关联</option><option value="needs_review" {{if eq .MatchStatusFilter "needs_review"}}selected{{end}}>需处理</option><option value="ignored" {{if eq .MatchStatusFilter "ignored"}}selected{{end}}>已忽略</option></select></label>
        <label for="sort">排序<select id="sort" name="sort"><option value="arrival_desc" {{if or (eq .SortFilter "") (eq .SortFilter "arrival_desc")}}selected{{end}}>到账日期新到旧</option><option value="arrival_asc" {{if eq .SortFilter "arrival_asc"}}selected{{end}}>到账日期旧到新</option><option value="amount_desc" {{if eq .SortFilter "amount_desc"}}selected{{end}}>金额从高到低</option><option value="amount_asc" {{if eq .SortFilter "amount_asc"}}selected{{end}}>金额从低到高</option><option value="payer_asc" {{if eq .SortFilter "payer_asc"}}selected{{end}}>付款人 A-Z</option></select></label>
        <label for="pending">待处理<input id="pending" name="pending" type="checkbox" value="1" {{if .PendingFilter}}checked{{end}}></label>
        <label for="page_size">每页<select id="page_size" name="page_size"><option value="25" {{if eq .PageSize 25}}selected{{end}}>25</option><option value="50" {{if eq .PageSize 50}}selected{{end}}>50</option><option value="100" {{if eq .PageSize 100}}selected{{end}}>100</option></select></label>
        <div class="filter-actions"><button class="btn" type="submit">应用筛选</button><a class="btn subtle" href="/billing">清除筛选</a></div>
      </form>
      <section class="panel surface" aria-labelledby="statement-title">
        <div class="panel-head"><h2 id="statement-title">流水明细</h2><span class="tiny">{{.LastSync}}</span></div>
        {{if .TransactionRows}}
        <div class="table-wrap"><table class="transaction-table">
          <thead><tr><th>类型</th><th>付款人／流水号</th><th>金额／余额</th><th>到账／租金月</th><th>描述／参考号</th><th>账户</th><th>用途／处理</th></tr></thead>
          <tbody>{{range .TransactionRows}}
            <tr class="{{.Direction}}">
              <td><span class="direction {{.Direction}}">{{.DirectionLabel}}</span></td>
              <td><div class="match-metadata"><strong>{{.PayerName}}</strong><span class="mono">付款人 ID: {{.PayerID}}</span><span class="mono">内部 ID: #{{.InternalID}}</span><span class="mono">银行流水号: {{.ProviderTransactionID}}</span></div></td>
              <td><span class="{{if eq .Direction "expense"}}amount expense{{else}}amount{{end}}">{{.AmountDisplay}}</span><div class="tiny">已分配 {{.AllocatedAmountDisplay}}</div><div class="tiny">余款 {{.RemainingAmountDisplay}}</div></td>
              <td class="mono">{{.DateDisplay}}<div class="tiny">解析租金月：{{if .ParsedPeriodDisplay}}{{.ParsedPeriodDisplay}}{{else}}未识别{{end}}</div><div class="tiny">确认租金月：{{if .FinalPeriodDisplay}}{{.FinalPeriodDisplay}}{{else}}未确认{{end}}</div></td>
              <td><div class="description">{{.Description}}</div><div class="mono">REF: {{.Reference}}</div></td>
              <td>{{.AccountName}}<br><span class="mono">{{.AccountID}}</span></td>
              <td><div class="tiny">{{.AllocationUseDisplay}}</div><a class="status status-link {{.MatchStatus}}" href="/billing?match_status={{.MatchStatus}}" title="筛选：{{.MatchStatusLabel}}">{{.MatchStatusLabel}}</a>{{if .MatchReason}}<div class="tiny">原因：{{.MatchReason}}</div>{{end}}{{if .NeedsMonthChoice}}<div class="tiny">已关联租客，请确认租金月份</div><form class="month-choice-form" method="post" action="/billing/confirm"><input type="hidden" name="transaction_id" value="{{.ID}}"><input type="hidden" name="tenant_id" value="{{.TenantID}}"><select name="period" aria-label="选择租金月份" required><option value="">选择月份...</option>{{range .MonthOptions}}<option value="{{.Period}}">{{.Label}} · 应收 {{.Expected}} · 已收 {{.Paid}} · 未收 {{.Remaining}}</option>{{end}}</select><label class="tiny"><input type="checkbox" name="remember_payer" value="1" checked> 记住此付款人</label><button class="btn" type="submit">确认月份</button></form>{{else if .CanConfirm}}<div class="tiny">候选租客：{{.CandidateTenantName}}</div><form class="confirm-form" method="post" action="/billing/confirm"><input type="hidden" name="transaction_id" value="{{.ID}}"><input type="checkbox" name="remember_payer" value="1" checked aria-label="记住此付款人"><button class="btn" type="submit">关联并记住</button></form>{{else if and (eq .Direction "income") (ne .MatchStatus "matched") (ne .MatchStatus "ignored")}}{{if $.TenantOptions}}<form class="bind-form" method="post" action="/billing/confirm"><input type="hidden" name="transaction_id" value="{{.ID}}"><select name="tenant_id" aria-label="关联流水到租客" required><option value="">选择租客...</option>{{range $.TenantOptions}}<option value="{{.ID}}">{{.Name}}</option>{{end}}</select><label class="tiny"><input type="checkbox" name="remember_payer" value="1" checked> 记住付款人</label><button class="btn" type="submit">关联并记住</button></form>{{else}}<div class="tiny bind-empty">请先添加租客，再关联收入流水。</div>{{end}}{{end}}{{if and (eq .Direction "income") (ne .MatchStatus "matched") (ne .MatchStatus "ignored")}}<details class="allocation-details"><summary>归类／拆分</summary><form class="allocation-form" method="post" action="/billing/allocate"><input type="hidden" name="transaction_id" value="{{.ID}}"><select name="allocation_kind" aria-label="入账用途"><option value="rent">房租</option><option value="deposit">押金</option><option value="other_income">其他收入</option></select><select name="tenant_id" aria-label="归类租客"><option value="">不指定租客</option>{{range $.TenantOptions}}<option value="{{.ID}}">{{.Name}}</option>{{end}}</select><select name="period" aria-label="归类租金月份"><option value="">选择租金月份...</option>{{range .MonthOptions}}<option value="{{.Period}}">{{.Label}} · 余款 {{.Remaining}}</option>{{end}}</select><input name="amount" inputmode="decimal" value="{{.RemainingAmountInput}}" aria-label="归类金额" required><input name="note" placeholder="其他收入备注（其他收入必填）" aria-label="归类备注"><button class="btn" type="submit">保存归类</button></form></details>{{end}}{{if eq .MatchStatus "ignored"}}<form class="action-form" method="post" action="/billing/restore"><input type="hidden" name="transaction_id" value="{{.ID}}"><input name="reason" aria-label="恢复原因" placeholder="恢复原因" required><button class="btn" type="submit">恢复处理</button></form>{{else if or (eq .MatchStatus "matched") (eq .MatchStatus "partial")}}<form class="action-form" method="post" action="/billing/revoke"><input type="hidden" name="transaction_id" value="{{.ID}}"><input name="reason" aria-label="撤销原因" placeholder="撤销原因" required><button class="btn danger" type="submit">撤销匹配</button></form>{{else if eq .Direction "income"}}<form class="action-form" method="post" action="/billing/ignore"><input type="hidden" name="transaction_id" value="{{.ID}}"><input name="reason" aria-label="忽略原因" placeholder="忽略原因" required><button class="btn" type="submit">无需匹配</button></form>{{end}}</td>
            </tr>
          {{end}}</tbody>
        </table></div>
        {{else}}<div class="empty">没有符合当前筛选条件的流水。</div>{{end}}
        {{if gt .TotalTransactions 0}}<div class="pagination"><span class="tiny">第 {{.Page}} / {{.TotalPages}} 页，共 {{.TotalTransactions}} 笔</span>{{if .PreviousPageURL}}<a href="{{.PreviousPageURL}}">上一页</a>{{end}}{{if .NextPageURL}}<a href="{{.NextPageURL}}">下一页</a>{{end}}</div>{{end}}
      </section>
    </main>
  </div>
<script>(function(){document.querySelectorAll('form[action="/billing/allocate"]').forEach(function(form){var names=['allocation_kind','tenant_id','period','amount','note'];var controls=Array.from(form.children).filter(function(el){return el.name && names.indexOf(el.name)>=0;});if(!controls.length)return;var line=document.createElement('div');line.className='allocation-line';controls.forEach(function(el){el.name=el.name+'[]';line.appendChild(el);});var submit=form.querySelector('button[type="submit"]');var add=document.createElement('button');add.type='button';add.className='btn allocation-add';add.textContent='添加拆分项';add.addEventListener('click',function(){var clone=line.cloneNode(true);clone.querySelectorAll('input,select').forEach(function(el){if(el.name==='allocation_kind[]')el.value='rent';else if(el.tagName==='SELECT')el.value='';else el.value='';});form.insertBefore(clone,add);});form.insertBefore(line,submit);form.insertBefore(add,submit);});document.querySelectorAll('form[action="/billing/revoke"]').forEach(function(form){form.method='get';var reason=form.querySelector('input[name="reason"]');if(reason)reason.remove();});})();</script>
</body>
</html>
`))
