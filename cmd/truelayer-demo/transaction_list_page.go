package main

var transactionListTemplate = newWorkspacePageTemplate("transactions", nil, `<!doctype html>
<html lang="zh-CN">
<head>
  <meta charset="utf-8">
  <meta name="viewport" content="width=device-width, initial-scale=1">
  <title>RentOps Transactions</title>
  <style>`+workspacePageCSS+`
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
    /* The page size sits with the pager rather than in the filter grid. */
    .pagination { display: flex; justify-content: space-between; align-items: center; flex-wrap: wrap; gap: 10px; margin-top: 12px; }
    .pagination .page-size-field { display: flex; align-items: center; gap: 8px; margin: 0; }
    .pagination .page-size-field select { min-height: 36px; }
    .pagination .pagination-nav { display: flex; align-items: center; gap: 10px; margin-left: auto; }
    .pagination a { color: var(--accent); text-decoration: none; }
    .transaction-route-heading { display: flex; align-items: flex-end; justify-content: space-between; gap: 20px; margin: 8px 0 12px; }
    .transaction-route-heading .eyebrow { margin: 0 0 5px; color: var(--accent-bright); font: 700 11px var(--mono); letter-spacing: .08em; }
    .transaction-route-heading h1 { margin: 0; font-size: 28px; }
    .transaction-route-heading .sub { margin-top: 5px; }
    .transaction-route-tools { display: flex; align-items: center; justify-content: flex-end; flex-wrap: wrap; gap: 7px; }
    .transaction-route-tools .btn { min-height: 38px; padding: 0 11px; white-space: nowrap; }
    .transaction-mobile-copy { display: none; }
    .transaction-route-page .transaction-route-tabs { display: none; }
    .transaction-route-tabs { display: flex; width: max-content; max-width: 100%; gap: 3px; margin: 0 0 14px; padding: 3px; border: 1px solid var(--border); border-radius: 8px; background: var(--surface); }
    .transaction-route-tabs a { display: inline-flex; min-height: 34px; align-items: center; justify-content: center; gap: 6px; padding: 0 14px; border-radius: 6px; color: var(--foreground-muted); font-size: 13px; font-weight: 650; text-decoration: none; white-space: nowrap; }
    .transaction-route-tabs a.active { background: var(--foreground); color: var(--surface); }
    .transaction-route-tabs a span { display: grid; min-width: 19px; height: 19px; place-items: center; border-radius: 999px; background: color-mix(in oklch, var(--danger) 12%, var(--surface)); color: var(--danger); font: 700 10px var(--mono); }
    .transaction-route-tabs a.active span { background: color-mix(in oklch, var(--surface) 18%, transparent); color: var(--surface); }
    .transaction-route-filters { margin: 0 0 14px; }
    .transaction-route-filters > summary { display: flex; min-height: 36px; align-items: center; cursor: pointer; color: var(--foreground-muted); font-size: 12px; font-weight: 650; }
    .transaction-route-mobile-list { display: none; }
    .transaction-route-desktop-list { display: block; }
    .transaction-route-quickfilter { display: flex; align-items: center; gap: 8px; margin: 0 0 10px; padding: 0; border: 0; background: transparent; }
    .transaction-route-quickfilter input, .transaction-route-quickfilter select { min-height: 36px; }
    .transaction-route-quickfilter input[type="search"] { flex: 1 1 320px; }
    .transaction-route-quickfilter input[type="month"] { flex: 0 0 180px; width: 180px; }
    .transaction-route-quickfilter .btn { min-height: 36px; }
    .transaction-route-page > .panel.surface > .panel-head { display: none; }
    .transaction-route-desktop-table { min-width: 1120px; }
    /* 左右 12 → 10px：给「操作」列让位。已关联的行比别的行多两个按钮
       （修改匹配 / 撤销匹配），加上租客姓名那列要放下"Bríd Ní Bhraonáin"这种
       带变音符的全名，十列加起来刚好顶出容器几个像素、多一条横向滚动条。
       十个格子各收 2px 就有 40px 余量，肉眼看不出，也扛得住以后更长的姓名。 */
    .transaction-route-desktop-table th, .transaction-route-desktop-table td { padding: 9px 10px; }
    /* 这一列短、且都是不该断开的原子值（日期、月份、用途、状态）。不锁住的话，
       浏览器在列被挤窄时会挑软柿子——"09-01"从连字符处断成两行、"同住代付"四个
       汉字断成两行。锁住之后被压缩的就只剩「匹配依据」那句本来就该折行的说明。 */
    .transaction-route-desktop-table th, .transaction-route-desktop-table .route-txn-fixed { white-space: nowrap; }
    .transaction-route-desktop-table td { vertical-align: middle; }
    .transaction-route-desktop-table .route-txn-payer strong { display: block; }
    .transaction-route-desktop-table .route-txn-context { min-width: 150px; color: var(--foreground-muted); font-size: 12px; }
    .transaction-route-desktop-table .route-txn-amount { white-space: nowrap; font: 700 13px var(--mono); }
    /* 「匹配依据」是一句话，折行不心疼，所以宽度从这里匀给付款人和房产/房间——
       那两列是姓名和地址，断在中间比这句话多折一行难读得多。
       190 → 172 是给这一行的长内容让的：已关联的行比其他行多两个按钮（修改/撤销），
       操作列因此宽了 4px，整张表就顶出容器 4px、多一条横向滚动条。这点宽度从
       这句话里扣，比让按钮换行或让日期断成两行都划算。 */
    .transaction-route-desktop-table .route-txn-reason { max-width: 172px; }
    .transaction-route-desktop-table .route-txn-period { white-space: nowrap; font: 600 12px var(--mono); }
    /* 没解析出月份时留一行灰字占位，不要留空。空单元格和"这笔不用管月份"长得
       一样，房东分不清是没识别出来还是压根不需要识别。 */
    .transaction-route-desktop-table .route-txn-period-none { color: var(--foreground-muted); font-weight: 400; }
    .transaction-route-desktop-table .route-txn-action { white-space: nowrap; }
    .transaction-route-desktop-table .route-txn-action .btn { min-height: 34px; padding: 0 10px; }
    .transaction-list-match > summary { width: max-content; list-style: none; cursor: pointer; }
    .transaction-list-match > summary::-webkit-details-marker { display: none; }
    .transaction-list-match form { display: grid; gap: 7px; min-width: 220px; margin-top: 8px; padding: 10px; border: 1px solid var(--border); border-radius: 9px; background: var(--surface-muted); white-space: normal; }
    .transaction-list-match form > label { display: grid; gap: 4px; color: var(--foreground-muted); font-size: 11px; }
    .transaction-list-match form select, .transaction-list-match form input { min-height: 36px; font-size: 12px; }
    .transaction-list-match-submit { display: flex; align-items: center; justify-content: space-between; gap: 8px; }
    .transaction-list-match-submit > label { display: inline-flex; align-items: center; gap: 6px; color: var(--foreground-muted); font-size: 11px; white-space: nowrap; }
    .transaction-list-match-submit > label input { margin: 0; }
    @media (max-width: 820px) { .filterbar { grid-template-columns: repeat(2, minmax(0, 1fr)); } }
    @media (max-width: 480px) { .filterbar { grid-template-columns: 1fr; } }
    @media (min-width: 641px) { .filterbar .calendar-popover { left: auto; right: 0; transform-origin: top right; } }
    @media (max-width: 640px) {
      .transaction-list-match-submit { flex-wrap: wrap; }
      .transaction-list-match-submit > label { min-height: 44px; }
      .transaction-list-match-submit .btn { min-height: 44px; }

      /* 上面这批之外剩的两个小控件：顶部筛选按钮 40px、每页条数下拉 36px。 */
      .filterbar .btn { min-height: 44px; }
      .pagination .page-size-field select { min-height: 44px; }


      .transaction-route-heading { align-items: stretch; flex-direction: column; margin: 0 0 14px; gap: 10px; }
      .transaction-route-heading h1 { font-size: 27px; }
      .transaction-route-heading .sub { max-width: 240px; font-size: 12px; }
      /* 四颗按钮，两列排成 2×2；三列会剩一颗单吊一行。 */
      .transaction-route-tools { display: grid; grid-template-columns: repeat(2,minmax(0,1fr)); gap: 6px; }
      .transaction-route-tools form { display: contents; }
      .transaction-route-tools .btn { min-height: 44px; padding: 0 7px; font-size: 11px; }
      .transaction-desktop-copy { display: none; }
      .transaction-mobile-copy { display: inline; }
      .transaction-route-tabs { display: grid; width: 100%; grid-template-columns: repeat(3,minmax(0,1fr)); margin-bottom: 12px; }
      .transaction-route-tabs a { min-width: 0; min-height: 40px; padding: 0 7px; font-size: 12px; }
      .transaction-route-tabs a span { min-width: 18px; height: 18px; }
      .transaction-route-filters { margin: 0 0 12px; padding: 0 12px; border: 1px solid var(--border); border-radius: 10px; background: var(--surface); }
      .transaction-route-filters > summary { display: flex; min-height: 42px; align-items: center; cursor: pointer; color: var(--foreground); font-size: 12px; font-weight: 700; }
      .transaction-route-filters[open] > summary { border-bottom: 1px solid var(--border); }
      .transaction-route-page .transaction-route-filters > summary { display: none; }
      .transaction-route-page .transaction-route-filters[open] > summary { display: flex; }
      .transaction-route-filters .filterbar { grid-template-columns: 1fr; margin: 0; padding: 12px 0; }
      .transaction-route-filters .filter-actions { display: grid; grid-template-columns: 1fr 1fr; }
      .transaction-route-quickfilter { display: none; }
      .transaction-route-page .transaction-route-tabs { display: grid; }
      .transaction-route-page .panel-head { display: none; }
      .transaction-route-desktop-list { display: none; }
      .transaction-route-mobile-list { display: grid; gap: 9px; }
      .transaction-route-page .pagination { display: none; }
      .transaction-review-card { min-width: 0; padding: 14px; border: 1px solid var(--border); border-radius: 12px; background: var(--surface); }
      .transaction-review-head { display: grid; grid-template-columns: minmax(0,1fr) auto; gap: 12px; align-items: start; }
      .transaction-review-head h3 { margin: 0; overflow-wrap: anywhere; font-size: 14px; }
      .transaction-review-head p { margin: 3px 0 0; color: var(--foreground-muted); font-size: 11px; line-height: 1.45; }
      .transaction-review-head .transaction-review-description { display: none; }
      .transaction-review-amount { white-space: nowrap; font: 700 14px var(--mono); font-variant-numeric: tabular-nums; }
      .transaction-review-amount.expense { color: var(--danger); }
      .transaction-review-facts { display: grid; grid-template-columns: 1fr 1fr; gap: 7px; margin-top: 10px; }
      .transaction-review-facts > div { display: grid; min-width: 0; gap: 3px; padding: 8px; border-radius: 7px; background: var(--surface-muted); }
      .transaction-review-facts span { color: var(--foreground-muted); font-size: 9px; }
      .transaction-review-facts strong { overflow-wrap: anywhere; font-size: 11px; line-height: 1.35; }
      .transaction-review-note { margin: 10px 0 0; padding: 10px; border-left: 2px solid var(--warning); border-radius: 7px; background: color-mix(in oklch,var(--warning) 7%,var(--surface)); color: var(--foreground-muted); font-size: 11px; line-height: 1.5; }
      .transaction-review-actions { display: grid; grid-template-columns: 1fr 1fr; gap: 8px; margin-top: 10px; }
      .transaction-review-actions .btn, .transaction-review-actions form { width: 100%; min-width: 0; }
      .transaction-review-actions .btn { min-height: 44px; padding: 0 8px; font-size: 12px; }
      .transaction-review-actions form { display: grid; gap: 7px; }
      .transaction-review-actions form[data-tenant-period-match] { grid-column: 1/-1; grid-template-columns: repeat(2,minmax(0,1fr)); }
      .transaction-review-actions form[data-tenant-period-match] select { min-width: 0; min-height: 44px; padding: 0 8px; border: 1px solid var(--border); border-radius: 8px; background: var(--surface); font-size: 11px; }
      .transaction-review-actions form[data-tenant-period-match] .btn { grid-column: 1/-1; }
      .transaction-review-actions select { min-width: 0; min-height: 44px; padding: 0 8px; border: 1px solid var(--border); border-radius: 8px; background: var(--surface); font-size: 11px; }
      /* 已关联的卡片比别的卡片多两颗 details 按钮（修改匹配 / 撤销匹配）。默认那套
         两列网格会把三样东西排成"查看 | 修改匹配"再单吊一颗撤销，留一个空格；
         更糟的是网格项默认拉伸对齐，修改匹配一展开，"查看"就被抻成一根竖条。
         带 details 的卡片改成单列堆叠，summary 撑满整行，展开的表单自然占满宽度。 */
      .transaction-review-actions:has(details) { grid-template-columns: 1fr; align-items: start; }
      .transaction-review-actions:has(details) > details > summary { width: 100%; justify-content: center; }
    }
  </style>
</head>
<body>
  <div class="app">
    {{template "workspace-nav" .}}
    <main class="content transaction-route-page">
      <header class="transaction-route-heading">
        <div><p class="eyebrow"><span class="transaction-desktop-copy">RentOps · 流水匹配</span><span class="transaction-mobile-copy">银行与收款</span></p><h1><span class="transaction-desktop-copy">流水匹配</span><span class="transaction-mobile-copy">流水处理</span></h1><p class="sub"><span class="transaction-desktop-copy">核对付款人与租金责任，确认一笔收款的最终分配。</span><span class="transaction-mobile-copy">只展示需要决定的关键信息。</span></p></div>
        {{/* 历史付款人预览原先只挂在老页面的顶栏上，老顶栏删掉后它就成了有路由没入口的
           功能，所以搬到这里。POST 是 handler 的要求（handlePayerPreview 只收 POST）。 */}}
        <div class="transaction-route-tools"><a class="btn subtle" href="{{.CashReceiptOpenURL}}">现金补录</a><a class="btn subtle" href="{{.ExpenseOpenURL}}">新增支出</a><form method="post" action="{{.CanonicalPath}}/payer/preview"><button class="btn subtle" type="submit">历史付款人预览</button></form>{{if .Connected}}<a class="btn primary" href="/bank/sync">同步</a>{{else}}<a class="btn primary" href="/login">连接银行</a>{{end}}</div>
      </header>
      <nav class="transaction-route-tabs" aria-label="流水状态">
        <a{{if eq .TransactionScope "pending"}} class="active"{{end}} href="{{.CanonicalPath}}?match_status=pending{{if .PeriodFilter}}&amp;period={{.PeriodFilter}}{{end}}">待处理 <span>{{.PendingCount}}</span></a>
        <a{{if eq .TransactionScope "matched"}} class="active"{{end}} href="{{.CanonicalPath}}?match_status=matched{{if .PeriodFilter}}&amp;period={{.PeriodFilter}}{{end}}">已关联</a>
        <a{{if eq .TransactionScope "all"}} class="active"{{end}} href="{{.CanonicalPath}}?scope=all{{if .PeriodFilter}}&amp;period={{.PeriodFilter}}{{end}}">全部</a>
      </nav>
      {{if .NeedsReconnect}}<div class="notice error">银行授权已失效，请重新连接。</div>{{end}}
      {{if eq .Error "data_fetch_failed"}}<div class="notice error">银行数据刷新失败，已有流水已保留。</div>{{end}}
      {{if eq .Error "confirmation_failed"}}<div class="notice error">这笔租金无法确认，请检查租客、金额和月份。</div>{{end}}
      {{if eq .Error "invalid_confirmation"}}<div class="notice error">关联请求无效。</div>{{end}}
      {{if eq .Error "invalid_allocation"}}<div class="notice error">归类请求无效。</div>{{end}}
      {{if eq .Error "allocation_failed"}}<div class="notice error">归类失败，请检查余额、币种、租客和租金月份。</div>{{end}}
      {{if eq .Error "rent_facts_conflict"}}<div class="notice error">该租金月份的计划刚刚更新，请刷新后重新分配流水。</div>{{end}}
      {{if eq .Error "rematch_failed"}}<div class="notice error">匹配调整失败，请刷新账期后重试。</div>{{end}}
      {{if eq .Error "invalid_transaction_action"}}<div class="notice error">流水操作请求无效。</div>{{end}}
      {{if eq .Error "transaction_action_failed"}}<div class="notice error">流水操作失败，请检查当前状态和操作原因。</div>{{end}}
      {{if eq .Error "invalid_payer_confirmation"}}<div class="notice error">付款人确认请求无效。</div>{{end}}
      {{if eq .Error "payer_confirmation_failed"}}<div class="notice error">付款人确认失败，请重新检查月份和账单余额。</div>{{end}}
      {{if eq .Error "invalid_filter"}}<div class="notice error">筛选条件无效。</div>{{end}}
      {{if eq .Message "bank_connected"}}<div class="notice ok" data-toast>银行账户已连接，流水已导入。</div>{{end}}
      {{if eq .Message "refreshed"}}<div class="notice ok" data-toast>银行数据已刷新。</div>{{end}}
      {{if eq .Message "rent_confirmed"}}<div class="notice ok" data-toast>租金已确认，符合条件的同名流水也已关联。</div>{{end}}
      {{if eq .Message "allocation_saved"}}<div class="notice ok" data-toast>流水归类已保存。</div>{{end}}
      {{if eq .Message "transaction_action_saved"}}<div class="notice ok" data-toast>流水操作已保存。</div>{{end}}
      {{if eq .Message "payer_confirmed"}}<div class="notice ok" data-toast>历史流水已逐笔确认。</div>{{end}}
      {{if eq .Message "cash_receipt_saved"}}<div class="notice ok" data-toast>现金收款已登记。</div>{{end}}
      {{if eq .Message "expense_added"}}<div class="notice ok" data-toast>支出记录已保存。</div>{{end}}
      {{if .CashReceiptDrawer}}{{template "cash-receipt-drawer" .CashReceiptDrawer}}{{end}}
      {{if .ExpenseDrawer}}{{template "expense-form-drawer" .ExpenseDrawer}}{{end}}
      <form class="transaction-route-quickfilter" method="get" action="/transactions" aria-label="搜索流水">
        <input type="hidden" name="scope" value="{{.TransactionScope}}">
        <input type="search" name="payer" value="{{.PayerFilter}}" placeholder="搜索当前列表" aria-label="搜索付款人">
        <input type="month" name="period" value="{{.PeriodFilter}}" aria-label="到账月份">
        <button class="btn" type="submit">搜索</button>
      </form>
      <details class="transaction-route-filters"><summary>搜索</summary>
      <form class="filterbar" method="get" action="{{.CanonicalPath}}" aria-label="Transaction filters">
        <label for="payer">付款人<input id="payer" name="payer" type="search" value="{{.PayerFilter}}" placeholder="付款人姓名"></label>
        <label for="tenant_id">租客<select id="tenant_id" name="tenant_id" data-searchable><option value="">全部租客</option>{{range .TenantOptions}}<option value="{{.ID}}" {{if eq $.TenantFilter .ID}}selected{{end}}>{{.Name}}</option>{{end}}</select></label>
        <label for="period">流水到账月<input id="period" name="period" type="month" value="{{.PeriodFilter}}" onchange="this.form.submit()"></label>
        <label for="rent_period">租金所属月<input id="rent_period" name="rent_period" type="month" value="{{.RentPeriodFilter}}"></label>
        <label for="allocation">入账用途<select id="allocation" name="allocation"><option value="">全部用途</option><option value="rent" {{if eq .AllocationFilter "rent"}}selected{{end}}>房租</option><option value="deposit" {{if eq .AllocationFilter "deposit"}}selected{{end}}>押金</option><option value="other_income" {{if eq .AllocationFilter "other_income"}}selected{{end}}>其他收入</option></select></label>
        <label for="direction">收支<select id="direction" name="direction"><option value="">全部</option><option value="income" {{if eq .DirectionFilter "income"}}selected{{end}}>收入</option><option value="expense" {{if eq .DirectionFilter "expense"}}selected{{end}}>支出</option></select></label>
        <label for="match_status">关联状态<select id="match_status" name="match_status" onchange="this.form.requestSubmit()"><option value="">全部状态</option><option value="pending" {{if eq .MatchStatusSelection "pending"}}selected{{end}}>待处理（需关联或确认）</option><option value="matched" {{if eq .MatchStatusSelection "matched"}}selected{{end}}>已关联</option><option value="partial" {{if eq .MatchStatusSelection "partial"}}selected{{end}}>部分关联</option><option value="candidate" {{if eq .MatchStatusSelection "candidate"}}selected{{end}}>待确认</option><option value="unmatched" {{if eq .MatchStatusSelection "unmatched"}}selected{{end}}>未关联</option><option value="needs_review" {{if eq .MatchStatusSelection "needs_review"}}selected{{end}}>需处理</option><option value="ignored" {{if eq .MatchStatusSelection "ignored"}}selected{{end}}>已忽略</option></select></label>
        {{if .ArrivalFromFilter}}<input type="hidden" name="arrival_from" value="{{.ArrivalFromFilter}}">{{end}}
        {{if .ArrivalToFilter}}<input type="hidden" name="arrival_to" value="{{.ArrivalToFilter}}">{{end}}
        {{if .SortFilter}}<input type="hidden" name="sort" value="{{.SortFilter}}">{{end}}
        <div class="filter-actions"><button class="btn" type="submit">搜索</button><a class="btn subtle" href="{{.CanonicalPath}}">清除筛选</a></div>
      </form>
      </details>
      <section class="panel surface" aria-labelledby="statement-title">
        <div class="panel-head"><h2 id="statement-title">流水明细</h2><span class="tiny">{{.LastSync}}</span></div>
        {{if .TransactionRows}}
<div class="transaction-route-desktop-list table-wrap"><table class="transaction-route-desktop-table"><thead><tr><th>日期</th><th>付款人</th><th>租客姓名</th><th>房产 / 房间</th><th>金额</th><th>识别租金月份</th><th>建议分配</th><th>匹配依据</th><th>状态</th><th>操作</th></tr></thead><tbody>{{range .TransactionRows}}{{$row := .}}<tr><td class="mono route-txn-fixed">{{if .DateShort}}{{.DateShort}}{{else}}{{.DateDisplay}}{{end}}</td><td class="route-txn-payer"><strong>{{.PayerName}}</strong></td><td>{{if .MatchedTenantName}}{{.MatchedTenantName}}{{else}}—{{end}}</td><td class="route-txn-context">{{if .ObjectLabel}}{{.ObjectLabel}}{{else}}{{.AccountName}}{{end}}</td><td class="route-txn-amount">{{.AmountDisplay}}</td><td class="route-txn-period">{{if .ParsedPeriodDisplay}}{{.ParsedPeriodDisplay}}{{else}}<span class="route-txn-period-none">未识别</span>{{end}}</td><td class="route-txn-fixed">{{if .AllocationUseDisplay}}{{.AllocationUseDisplay}}{{else}}{{.MatchStatusLabel}}{{end}}</td><td class="route-txn-context route-txn-reason">{{if .MatchReason}}{{.MatchReason}}{{else}}—{{end}}</td><td class="route-txn-fixed"><span class="status {{.MatchStatus}}">{{.MatchStatusLabel}}</span></td><td class="route-txn-action"><a class="btn subtle" href="{{.DetailURL}}">查看详情</a>{{if .ManualMatchOptions}}<details class="transaction-list-match"><summary class="btn primary">匹配流水</summary><form method="post" action="{{$.CanonicalPath}}/confirm" data-tenant-period-match><input type="hidden" name="transaction_id" value="{{.ID}}"><input type="hidden" name="remember_payer" value="0"><input type="hidden" name="return_to" value="{{.ReturnURL}}"><label>租客<select name="tenant_id" aria-label="选择匹配租客" data-searchable required><option value="">选择租客</option>{{range .ManualMatchTenantOptions}}<option value="{{.ID}}"{{if eq .ID $row.CandidateTenantID}} selected{{end}}>{{.Name}}</option>{{end}}</select></label>{{template "tenant-period-calendar" (tenantPeriodMatchCalendar .ManualMatchOptions .CandidateTenantID .CandidatePeriod)}}<div class="transaction-list-match-submit"><label class="tiny"><input type="checkbox" name="remember_payer" value="1" checked> 记住付款人</label><button class="btn primary" type="submit">确认匹配</button></div></form></details>{{else if or (eq .MatchStatus "matched") (eq .MatchStatus "partial")}}<details class="transaction-list-match"><summary class="btn">修改匹配</summary>{{if .CanEditRentMatch}}<form method="post" action="{{$.CanonicalPath}}/rematch"><input type="hidden" name="transaction_id" value="{{.ID}}"><input type="hidden" name="return_to" value="{{.ReturnURL}}"><label>租客<select name="tenant_id" aria-label="修改匹配租客" data-searchable required><option value="">选择租客</option>{{range .RematchTenantOptions}}<option value="{{.ID}}">{{.Name}}</option>{{end}}</select></label><label>租金月份<select name="period" aria-label="修改租金月份" required><option value="">选择租金月份</option>{{range .RematchMonthOptions}}<option value="{{.Period}}">{{.Label}} · 未收 {{.Remaining}}</option>{{end}}</select></label><button class="btn primary" type="submit">确认修改</button></form>{{else if .CanRematch}}<p class="tiny">当前没有可替换的租金目标；如需调整，请先撤销匹配。</p>{{else}}<p class="tiny">该流水已拆分或含其他用途；请先撤销匹配，再重新归类。</p>{{end}}</details>{{/* 撤销是两步：这里只发一个 GET，跳到确认页把该流水的全部分配摊开给人看，
    填完原因才真的 POST。所以这个表单没有原因输入框，method 也是 get —— 老表格里
    那一份写的是 post + 原因，靠页尾脚本改成 get 再删掉输入框，这里不重复那个把戏。 */}}<details class="transaction-list-match"><summary class="btn danger">撤销匹配</summary><form method="get" action="{{$.CanonicalPath}}/revoke"><input type="hidden" name="transaction_id" value="{{.ID}}"><input type="hidden" name="return_to" value="{{.ReturnURL}}"><p class="tiny">下一步会列出这笔流水的全部分配，确认后才作废。</p><button class="btn danger" type="submit">继续撤销</button></form></details>{{end}}</td></tr>{{end}}</tbody></table></div>
        <div class="transaction-route-mobile-list">
          {{range .TransactionRows}}
          {{$row := .}}
          <article class="transaction-review-card" id="mobile-transaction-{{.DetailKey}}">
            <div class="transaction-review-head">
              <div><h3>{{.PayerName}}</h3><p>{{if .DateShort}}{{.DateShort}}{{else}}{{.DateDisplay}}{{end}} · {{if .ObjectLabel}}{{.ObjectLabel}}{{else}}{{.AccountName}}{{end}}</p><p class="transaction-review-description">{{.Description}}</p><p class="tiny">识别租金月份：{{if .ParsedPeriodDisplay}}{{.ParsedPeriodDisplay}}{{else}}未识别{{end}}</p>{{if .MatchedTenantName}}<p class="tiny">租客姓名：{{.MatchedTenantName}}</p>{{end}}</div>
              <strong class="transaction-review-amount{{if eq .Direction "expense"}} expense{{end}}">{{if eq .Direction "income"}}+{{end}}{{.AmountDisplay}}</strong>
            </div>
            <div class="transaction-review-facts">
              {{if .CanConfirm}}
              <div><span>建议用途</span><strong>{{if .AllocationUseDisplay}}{{.AllocationUseDisplay}}{{else}}{{.MatchStatusLabel}}{{end}}</strong></div>
              <div><span>未分配</span><strong>{{.RemainingAmountDisplay}}</strong></div>
              {{else if .NeedsMonthChoice}}
              <div><span>建议对象</span><strong>{{if .CandidateTenantName}}{{.CandidateTenantName}}{{else}}{{.PayerName}}{{end}}{{if .RoomOnlyLabel}} · {{.RoomOnlyLabel}}{{end}}</strong></div>
              <div><span>匹配依据</span><strong>{{if .MatchReason}}{{.MatchReason}}{{else}}已识别租客{{end}}</strong></div>
              {{else}}
              <div><span>状态</span><strong>{{.MatchStatusLabel}}</strong></div>
              <div><span>未分配</span><strong>{{.RemainingAmountDisplay}}</strong></div>
              {{end}}
            </div>
            {{if and .MatchReason (not .NeedsMonthChoice)}}<p class="transaction-review-note">{{.MatchReason}}</p>{{end}}
            <div class="transaction-review-actions">
              <a class="btn" href="{{.DetailURL}}">{{if .CanConfirm}}查看分配{{else}}查看{{end}}</a>
              {{if .ManualMatchOptions}}
              <form method="post" action="{{$.CanonicalPath}}/confirm" data-tenant-period-match><input type="hidden" name="transaction_id" value="{{.ID}}"><input type="hidden" name="remember_payer" value="0"><input type="hidden" name="return_to" value="{{$row.ReturnURL}}"><select name="tenant_id" aria-label="选择匹配租客" data-searchable required><option value="">选择租客</option>{{range .ManualMatchTenantOptions}}<option value="{{.ID}}"{{if eq .ID $row.CandidateTenantID}} selected{{end}}>{{.Name}}</option>{{end}}</select>{{template "tenant-period-calendar" (tenantPeriodMatchCalendar .ManualMatchOptions .CandidateTenantID .CandidatePeriod)}}<label class="tiny"><input type="checkbox" name="remember_payer" value="1" checked> 记住付款人</label><button class="btn primary" type="submit">匹配流水</button></form>
              {{else if or (eq .MatchStatus "matched") (eq .MatchStatus "partial")}}
              {{/* 窄屏的「改错出口」。桌面表格 ≤640px 是藏起来的，手机上只剩这张卡片；
                 只给一个"处理流水"的话，点进详情页也只有修改匹配——那里还写着
                 "请先在流水列表撤销匹配"，而手机上根本没有那张列表。两条路都堵死。 */}}
              <details class="transaction-list-match"><summary class="btn">修改匹配</summary>{{if .CanEditRentMatch}}<form method="post" action="{{$.CanonicalPath}}/rematch"><input type="hidden" name="transaction_id" value="{{.ID}}"><input type="hidden" name="return_to" value="{{$row.ReturnURL}}"><label>租客<select name="tenant_id" aria-label="修改匹配租客" data-searchable required><option value="">选择租客</option>{{range .RematchTenantOptions}}<option value="{{.ID}}">{{.Name}}</option>{{end}}</select></label><label>租金月份<select name="period" aria-label="修改租金月份" required><option value="">选择租金月份</option>{{range .RematchMonthOptions}}<option value="{{.Period}}">{{.Label}} · 未收 {{.Remaining}}</option>{{end}}</select></label><button class="btn primary" type="submit">确认修改</button></form>{{else if .CanRematch}}<p class="tiny">当前没有可替换的租金目标；如需调整，请先撤销匹配。</p>{{else}}<p class="tiny">该流水已拆分或含其他用途；请先撤销匹配，再重新归类。</p>{{end}}</details>
              <details class="transaction-list-match"><summary class="btn danger">撤销匹配</summary><form method="get" action="{{$.CanonicalPath}}/revoke"><input type="hidden" name="transaction_id" value="{{.ID}}"><input type="hidden" name="return_to" value="{{$row.ReturnURL}}"><p class="tiny">下一步会列出这笔流水的全部分配，确认后才作废。</p><button class="btn danger" type="submit">继续撤销</button></form></details>
              {{else}}
              <a class="btn primary" href="{{.DetailURL}}">处理流水</a>
              {{end}}
            </div>
          </article>
          {{end}}
        </div>
        {{else}}<div class="empty">没有符合当前筛选条件的流水。</div>{{end}}
        {{if gt .TotalTransactions 0}}<div class="pagination">
          <form class="page-size-field" method="get" action="{{.CanonicalPath}}">
            {{if .ArrivalFromFilter}}<input type="hidden" name="arrival_from" value="{{.ArrivalFromFilter}}">{{end}}
            {{if .ArrivalToFilter}}<input type="hidden" name="arrival_to" value="{{.ArrivalToFilter}}">{{end}}
            {{if .PayerFilter}}<input type="hidden" name="payer" value="{{.PayerFilter}}">{{end}}
            {{if .TenantFilter}}<input type="hidden" name="tenant_id" value="{{.TenantFilter}}">{{end}}
            {{if .PeriodFilter}}<input type="hidden" name="period" value="{{.PeriodFilter}}">{{end}}
            {{if .RentPeriodFilter}}<input type="hidden" name="rent_period" value="{{.RentPeriodFilter}}">{{end}}
            {{if .AllocationFilter}}<input type="hidden" name="allocation" value="{{.AllocationFilter}}">{{end}}
            {{if .DirectionFilter}}<input type="hidden" name="direction" value="{{.DirectionFilter}}">{{end}}
            {{if .MatchStatusSelection}}<input type="hidden" name="match_status" value="{{.MatchStatusSelection}}">{{end}}
            {{if .SortFilter}}<input type="hidden" name="sort" value="{{.SortFilter}}">{{end}}
            <label for="page_size">每页<select id="page_size" name="page_size" onchange="this.form.submit()"><option value="10" {{if eq .PageSize 10}}selected{{end}}>10</option><option value="25" {{if eq .PageSize 25}}selected{{end}}>25</option><option value="50" {{if eq .PageSize 50}}selected{{end}}>50</option><option value="100" {{if eq .PageSize 100}}selected{{end}}>100</option></select></label>
          </form>
          <div class="pagination-nav"><span class="tiny">第 {{.Page}} / {{.TotalPages}} 页，共 {{.TotalTransactions}} 笔</span>{{if .PreviousPageURL}}<a href="{{.PreviousPageURL}}">上一页</a>{{end}}<span class="pagination-numbers" aria-label="选择页码">{{range .Pagination}}{{if .Ellipsis}}<span class="pagination-ellipsis" aria-hidden="true">…</span>{{else if .Current}}<span class="pagination-number is-current" aria-current="page">{{.Number}}</span>{{else}}<a class="pagination-number" href="{{.URL}}">{{.Number}}</a>{{end}}{{end}}</span>{{if .NextPageURL}}<a href="{{.NextPageURL}}">下一页</a>{{end}}</div>
        </div>{{end}}
      </section>
    </main>
</div>
{{/* 页尾原来还有一段脚本：≤640px 时摘掉「搜索」折叠面板的 open。它从来没生效过——
   <details class="transaction-route-filters"> 本身就不带 open，没有东西可摘。 */}}
</body>
</html>
`)
