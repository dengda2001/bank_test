package main

var billingTemplate = newWorkspacePageTemplate("billing", nil, `<!doctype html>
<html lang="zh-CN">
<head>
  <meta charset="utf-8">
  <meta name="viewport" content="width=device-width, initial-scale=1">
  <title>RentOps Transactions</title>
  <style>`+workspacePageCSS+workspaceCalendarCSS+`
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
    /* 主金额在窄屏被搬进冻结的操作列，宽屏不渲染这一份（金额列本身就在）。 */
    .txn-amount-mobile { display: none; }
    .direction { color: var(--foreground-muted); font: 700 11px var(--mono); text-transform: uppercase; letter-spacing: 0.08em; }
    .direction.income { color: var(--positive); }
    .direction.expense { color: var(--danger); }
    .match-metadata { display: grid; gap: 6px; }
    .transaction-detail-link { display: inline-flex; width: max-content; align-items: center; min-height: 24px; color: var(--accent); font-size: 11px; font-weight: 700; text-decoration: underline; text-underline-offset: 3px; }
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
    /* 已关联的行原本常驻两个 100% 宽的下拉（选择租客 / 选择租金月份），状态早就定了，
       控件却占满整列，把「撤销匹配」一起挤下去。收进 details：默认只剩「修改匹配」
       这一颗按钮，点开才出现选择。窄屏的 44px 命中区由基础样式的 .btn 规则兜住。 */
    .rematch-details { margin-top: 8px; }
    .rematch-details > summary { list-style: none; }
    .rematch-details > summary::-webkit-details-marker { display: none; }
    .rematch-details[open] > summary { margin-bottom: 6px; }
    th .sort-link { display: inline-flex; align-items: center; gap: 6px; color: inherit; font: inherit; letter-spacing: inherit; text-decoration: none; white-space: nowrap; }
    th .sort-link:hover { color: var(--accent); }
    th .sort-link.active { color: var(--accent); font-weight: 800; }
    th .sort-link .sort-arrow { font-size: 10px; line-height: 1; }
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
    .transaction-route-page .billing-table-wrap { display: none; }
    .transaction-route-quickfilter { display: flex; align-items: center; gap: 8px; margin: 0 0 10px; padding: 0; border: 0; background: transparent; }
    .transaction-route-quickfilter input, .transaction-route-quickfilter select { min-height: 36px; }
    .transaction-route-quickfilter input[type="search"] { flex: 1 1 320px; }
    .transaction-route-quickfilter select[name="match_status"] { flex: 0 0 190px; width: 190px; }
    .transaction-route-quickfilter input[type="month"] { flex: 0 0 180px; width: 180px; }
    .transaction-route-quickfilter .btn { min-height: 36px; }
    .transaction-route-page > .panel.surface > .panel-head { display: none; }
    .transaction-route-desktop-table { min-width: 960px; }
    .transaction-route-desktop-table th, .transaction-route-desktop-table td { padding: 9px 12px; }
    .transaction-route-desktop-table td { vertical-align: middle; }
    .transaction-route-desktop-table .route-txn-payer strong { display: block; }
    .transaction-route-desktop-table .route-txn-context { min-width: 150px; color: var(--foreground-muted); font-size: 12px; }
    .transaction-route-desktop-table .route-txn-amount { white-space: nowrap; font: 700 13px var(--mono); }
    .transaction-route-desktop-table .route-txn-action { white-space: nowrap; }
    .transaction-route-desktop-table .route-txn-action .btn { min-height: 34px; padding: 0 10px; }
    @media (max-width: 820px) { .filterbar { grid-template-columns: repeat(2, minmax(0, 1fr)); } }
    @media (max-width: 480px) { .filterbar { grid-template-columns: 1fr; } }
    @media (min-width: 641px) { .filterbar .calendar-popover { left: auto; right: 0; transform-origin: top right; } }
    /* 用途／处理列实测 320px，钉不住。窄屏把它换成约 88px 的「状态 + 处理」列，
       点开就地展开；账户与描述／参考号是次要信息，先让出宽度。 */
    @media (min-width: 641px) {
      /* 桌面端：summary 隐藏、内容常显，与改动前的直排内容一致。渲染时 details
         总是带 open，所以这里只是宽屏下的零差异声明；::details-content 是手机
         横屏（>640）后 JS 已移除 open 时的兜底，保证内容不会消失。 */
      .transaction-table .txn-col-action > .txn-action > summary { display: none; }
      .transaction-table .txn-col-action > .txn-action::details-content { content-visibility: visible !important; }
    }
    @media (max-width: 640px) {
      .transaction-detail-link { min-height: 44px; }
      /* 归类控件原本只有 30–34px 高，是这页的核心操作。 */
      .confirm-form .btn,
      .bind-form select, .bind-form .btn,
      .month-choice-form select, .month-choice-form .btn,
      .action-form input, .action-form .btn,
      .allocation-form select, .allocation-form input, .allocation-form .btn { min-height: 44px; }

      /* 上面这批之外剩的两个小控件：顶部筛选按钮 40px、每页条数下拉 36px。 */
      .filterbar .btn { min-height: 44px; }
      .pagination .page-size-field select { min-height: 44px; }

      /* 「记住此付款人」的勾选框实测 13x13。被 <label> 包住的那两处，命中区就是
         整个 label，把 label 撑到 44px 即可；确认表单里是裸 input，没有 label
         可撑，就用 padding 把元素盒撑到 44x44 —— border-box 下内容盒仍是 14x14，
         画出来的勾还是原来的大小。 */
      .bind-form label.tiny, .month-choice-form label.tiny { display: inline-flex; align-items: center; gap: 8px; min-height: 44px; }
      .confirm-form input[name="remember_payer"] { width: 44px; height: 44px; padding: 15px; }

      /* 收起两列后原来的 1040px 地板不再需要，但不能撤成 0：CJK 可以逐字断行，
         地板一撤「收入」「支出」就会被竖排成单字一列（P1-3 的成因）。 */
      .transaction-table { min-width: 680px; }
      .transaction-table .txn-col-desc,
      .transaction-table .txn-col-account { display: none; }
      /* 主金额整列搬进冻结的操作列（见下面 .txn-amount-mobile），单独那一列在窄屏
         不再渲染。三行都不换行的金额列实测 136–146px，和 88px 的操作列一起占掉
         327px 可视窗的 72%，又被 sticky 左推后直接盖在付款人列上（实测金额列
         127..263 压在付款人列 73..291 上，重叠 136px）——付款人姓名是唯一能认出
         「正在操作哪一行」的东西，却被压在底下。搬进操作列后冻结组收到 ~118px。 */
      .transaction-table .txn-col-amount { display: none; }
      .transaction-table .txn-col-action { width: 118px; padding: 10px 8px; }
      .transaction-table .txn-action > summary { list-style: none; }
      .transaction-table .txn-action > summary::-webkit-details-marker { display: none; }
      .transaction-table .txn-col-action > .txn-action > summary { display: grid; gap: 6px; justify-items: start; cursor: pointer; }
      .transaction-table .txn-col-action > .txn-action > summary::after { content: "▾ 处理"; color: var(--accent); font-size: 12px; }
      .transaction-table .txn-col-action > .txn-action[open] > summary::after { content: "▴ 收起"; }
      .transaction-table .txn-col-action > .txn-action[open] > .txn-body { margin-top: 8px; }
      /* 展开后这一列被表单撑到 266px（占 327px 可视窗的 81%），付款人列只剩 14px。
         试过在展开时解除冻结：付款人回来了，但表单本身被推到 438..704，正好落在
         可视窗右侧之外 —— 点「处理」之后屏幕上什么都不出现，得先右滑才找得到表单，
         更糟。保持冻结：填表时看的是表单和 summary 里的状态＋金额，付款人这一行
         的身份由金额和状态承担。 */
      /* 搬进来的主金额：数字不换行（被切掉末位后仍读成合法数字），支出色与金额列
         一致。已分配／余款不跟着搬，它们留在这个 details 展开后的 txn-body 里。 */
      .txn-amount-mobile { display: block; font-weight: 700; font-variant-numeric: tabular-nums; white-space: nowrap; }
      .txn-amount-mobile.expense { color: var(--danger); }

      /* 把流水行重排成手机卡片：保留状态、付款人、金额、到账日期和处理入口，
         避免用户横向寻找冻结列。桌面表格结构和操作表单继续复用。 */
      .billing-table-wrap { overflow: visible; }
      .billing-table-wrap .transaction-table { display: block; width: 100%; min-width: 0; }
      .billing-table-wrap .transaction-table thead { display: none; }
      .billing-table-wrap .transaction-table tbody { display: grid; gap: 10px; }
      .billing-table-wrap .transaction-table tbody tr { display: grid; grid-template-columns: minmax(0, 1fr) auto; gap: 8px 12px; padding: 14px; border: 1px solid var(--border); border-radius: 12px; background: var(--surface); }
      .billing-table-wrap .transaction-table tbody td { display: block; min-width: 0; padding: 0; border: 0; }
      .billing-table-wrap .transaction-table tbody td.txn-col-type { grid-column: 1; }
      .billing-table-wrap .transaction-table tbody td.txn-col-payer { grid-column: 1; }
      .billing-table-wrap .transaction-table tbody td.txn-col-date { grid-column: 2; grid-row: 1 / span 2; text-align: right; }
      .billing-table-wrap .transaction-table tbody td.txn-col-desc { display: block; grid-column: 1 / -1; padding-top: 4px; }
      .billing-table-wrap .transaction-table tbody td.txn-col-action { grid-column: 1 / -1; position: static !important; width: auto !important; padding: 0; background: transparent !important; box-shadow: none !important; }
      .billing-table-wrap .transaction-table tbody td.txn-col-action > .txn-action { display: block; }
      .billing-table-wrap .transaction-table tbody td.txn-col-action > .txn-action > summary { display: flex; align-items: center; justify-content: space-between; gap: 10px; min-height: 44px; }
      .billing-table-wrap .transaction-table tbody td.txn-col-action > .txn-action > .txn-body { width: 100%; }

      .transaction-route-heading { align-items: center; margin: 0 0 14px; gap: 10px; }
      .transaction-route-heading h1 { font-size: 27px; }
      .transaction-route-heading .sub { max-width: 240px; font-size: 12px; }
      .transaction-route-heading > .btn { min-height: 40px; padding: 0 12px; font-size: 12px; }
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
      .transaction-route-page .billing-table-wrap { display: none; }
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
      .transaction-review-actions select { min-width: 0; min-height: 44px; padding: 0 8px; border: 1px solid var(--border); border-radius: 8px; background: var(--surface); font-size: 11px; }
    }
  </style>
  <script>`+workspaceCalendarScript+`</script>
</head>
<body>
  <div class="app">
    {{template "workspace-nav" .}}
    <main class="content{{if eq .PageKey "transactions"}} transaction-route-page{{end}}">
      {{if eq .PageKey "transactions"}}
      <header class="transaction-route-heading">
        <div><p class="eyebrow"><span class="transaction-desktop-copy">RentOps · 流水匹配</span><span class="transaction-mobile-copy">银行与收款</span></p><h1><span class="transaction-desktop-copy">流水匹配</span><span class="transaction-mobile-copy">流水处理</span></h1><p class="sub"><span class="transaction-desktop-copy">核对付款人与租金责任，确认一笔收款的最终分配。</span><span class="transaction-mobile-copy">只展示需要决定的关键信息。</span></p></div>
        {{if .Connected}}<a class="btn primary" href="/bank/sync">同步</a>{{else}}<a class="btn primary" href="/login">连接银行</a>{{end}}
      </header>
      <nav class="transaction-route-tabs" aria-label="流水状态">
        <a{{if eq .TransactionScope "pending"}} class="active"{{end}} href="{{.CanonicalPath}}?match_status=pending{{if .PeriodFilter}}&amp;period={{.PeriodFilter}}{{end}}">待处理 <span>{{.PendingCount}}</span></a>
        <a{{if eq .TransactionScope "matched"}} class="active"{{end}} href="{{.CanonicalPath}}?match_status=matched{{if .PeriodFilter}}&amp;period={{.PeriodFilter}}{{end}}">已匹配</a>
        <a{{if eq .TransactionScope "all"}} class="active"{{end}} href="{{.CanonicalPath}}?scope=all{{if .PeriodFilter}}&amp;period={{.PeriodFilter}}{{end}}">全部</a>
      </nav>
      {{else}}
      <header class="topbar">
        <div><div class="brand-title">银行流水</div><h1>收款与支出</h1></div>
        <div class="actions">
          {{if .Connected}}<a class="btn primary" href="/refresh">刷新银行数据</a>{{else}}<a class="btn primary" href="/login">连接银行账户</a>{{end}}
          <form method="post" action="/billing/payer/preview"><button class="btn" type="submit">历史付款人预览</button></form>
          <form method="post" action="/import-legacy"><button class="btn" type="submit">导入旧数据</button></form>
          <form method="post" action="/logout"><button class="btn danger" type="submit">退出登录</button></form>
        </div>
      </header>
      {{end}}
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
      {{if eq .PageKey "transactions"}}
      <form class="transaction-route-quickfilter" method="get" action="/transactions" aria-label="快速筛选流水">
        <input type="hidden" name="scope" value="{{.TransactionScope}}">
        <input type="search" name="payer" value="{{.PayerFilter}}" placeholder="搜索当前列表" aria-label="搜索付款人">
        <select name="match_status" aria-label="流水状态"><option value="pending"{{if eq .MatchStatusSelection "pending"}} selected{{end}}>待处理</option><option value="matched"{{if eq .MatchStatusSelection "matched"}} selected{{end}}>已匹配</option><option value=""{{if and (eq .TransactionScope "all") (eq .MatchStatusSelection "")}} selected{{end}}>全部状态</option><option value="partial"{{if eq .MatchStatusSelection "partial"}} selected{{end}}>部分匹配</option><option value="candidate"{{if eq .MatchStatusSelection "candidate"}} selected{{end}}>待确认</option><option value="unmatched"{{if eq .MatchStatusSelection "unmatched"}} selected{{end}}>未匹配</option><option value="needs_review"{{if eq .MatchStatusSelection "needs_review"}} selected{{end}}>需处理</option></select>
        <input type="month" name="period" value="{{.PeriodFilter}}" aria-label="到账月份">
        <button class="btn" type="submit">筛选</button>
      </form>
      <details class="transaction-route-filters"><summary>更多筛选</summary>{{end}}
      <form class="filterbar" method="get" action="{{.CanonicalPath}}" aria-label="Transaction filters">
        <label for="payer">付款人<input id="payer" name="payer" type="search" value="{{.PayerFilter}}" placeholder="付款人姓名"></label>
        <label for="tenant_id">租客<select id="tenant_id" name="tenant_id"><option value="">全部租客</option>{{range .TenantOptions}}<option value="{{.ID}}" {{if eq $.TenantFilter .ID}}selected{{end}}>{{.Name}}</option>{{end}}</select></label>
        <label for="period">流水到账月<input id="period" name="period" type="month" value="{{.PeriodFilter}}" onchange="this.form.submit()"></label>
        <label for="rent_period">租金所属月<input id="rent_period" name="rent_period" type="month" value="{{.RentPeriodFilter}}"></label>
        <label for="allocation">入账用途<select id="allocation" name="allocation"><option value="">全部用途</option><option value="rent" {{if eq .AllocationFilter "rent"}}selected{{end}}>房租</option><option value="deposit" {{if eq .AllocationFilter "deposit"}}selected{{end}}>押金</option><option value="other_income" {{if eq .AllocationFilter "other_income"}}selected{{end}}>其他收入</option></select></label>
        <label for="direction">收支<select id="direction" name="direction"><option value="">全部</option><option value="income" {{if eq .DirectionFilter "income"}}selected{{end}}>收入</option><option value="expense" {{if eq .DirectionFilter "expense"}}selected{{end}}>支出</option></select></label>
        <label for="match_status">匹配状态<select id="match_status" name="match_status"><option value="">全部状态</option><option value="pending" {{if eq .MatchStatusSelection "pending"}}selected{{end}}>待处理（需关联或确认）</option><option value="matched" {{if eq .MatchStatusSelection "matched"}}selected{{end}}>已关联</option><option value="partial" {{if eq .MatchStatusSelection "partial"}}selected{{end}}>部分关联</option><option value="candidate" {{if eq .MatchStatusSelection "candidate"}}selected{{end}}>待确认</option><option value="unmatched" {{if eq .MatchStatusSelection "unmatched"}}selected{{end}}>未关联</option><option value="needs_review" {{if eq .MatchStatusSelection "needs_review"}}selected{{end}}>需处理</option><option value="ignored" {{if eq .MatchStatusSelection "ignored"}}selected{{end}}>已忽略</option></select></label>
        {{if .ArrivalFromFilter}}<input type="hidden" name="arrival_from" value="{{.ArrivalFromFilter}}">{{end}}
        {{if .ArrivalToFilter}}<input type="hidden" name="arrival_to" value="{{.ArrivalToFilter}}">{{end}}
        {{if .SortFilter}}<input type="hidden" name="sort" value="{{.SortFilter}}">{{end}}
        <div class="filter-actions"><button class="btn" type="submit">应用筛选</button><a class="btn subtle" href="{{.CanonicalPath}}">清除筛选</a></div>
      </form>
      {{if eq .PageKey "transactions"}}</details>{{end}}
      <section class="panel surface" aria-labelledby="statement-title">
        <div class="panel-head"><h2 id="statement-title">流水明细</h2><span class="tiny">{{.LastSync}}</span></div>
        {{if .TransactionRows}}
        {{if eq .PageKey "transactions"}}<div class="transaction-route-desktop-list table-wrap"><table class="transaction-route-desktop-table"><thead><tr><th>日期</th><th>付款记录</th><th>房产 / 房间</th><th>金额</th><th>建议分配</th><th>匹配依据</th><th>状态 / 操作</th></tr></thead><tbody>{{range .TransactionRows}}<tr><td class="mono">{{if .DateShort}}{{.DateShort}}{{else}}{{.DateDisplay}}{{end}}</td><td class="route-txn-payer"><strong>{{.PayerName}}</strong></td><td class="route-txn-context">{{if .ObjectLabel}}{{.ObjectLabel}}{{else}}{{.AccountName}}{{end}}</td><td class="route-txn-amount">{{.AmountDisplay}}</td><td>{{if .AllocationUseDisplay}}{{.AllocationUseDisplay}}{{else}}{{.MatchStatusLabel}}{{end}}</td><td class="route-txn-context">{{if .MatchReason}}{{.MatchReason}}{{else}}—{{end}}</td><td class="route-txn-action"><span class="status {{.MatchStatus}}">{{.MatchStatusLabel}}</span> <a class="btn subtle" href="{{.DetailURL}}">处理</a></td></tr>{{end}}</tbody></table></div>{{end}}
        <div class="billing-table-wrap table-wrap"><table class="transaction-table">
          <thead><tr><th class="txn-col-type">类型</th><th class="txn-col-payer"><a class="sort-link{{if .PayerSort.Active}} active{{end}}" href="{{.PayerSort.URL}}">付款人{{if .PayerSort.Arrow}}<span class="sort-arrow">{{.PayerSort.Arrow}}</span>{{end}}</a></th><th class="txn-col-amount"><a class="sort-link{{if .AmountSort.Active}} active{{end}}" href="{{.AmountSort.URL}}">金额／余额{{if .AmountSort.Arrow}}<span class="sort-arrow">{{.AmountSort.Arrow}}</span>{{end}}</a></th><th class="txn-col-date"><a class="sort-link{{if .ArrivalSort.Active}} active{{end}}" href="{{.ArrivalSort.URL}}">到账／租金月{{if .ArrivalSort.Arrow}}<span class="sort-arrow">{{.ArrivalSort.Arrow}}</span>{{end}}</a></th><th class="txn-col-desc">描述</th><th class="txn-col-account">账户</th><th class="txn-col-action">用途／处理</th></tr></thead>
          <tbody>{{range .TransactionRows}}
            <tr id="transaction-row-{{.DetailKey}}" class="{{.Direction}}">
              <td class="txn-col-type"><span class="direction {{.Direction}}">{{.DirectionLabel}}</span></td>
              <td class="txn-col-payer"><div class="match-metadata"><strong>{{.PayerName}}</strong><a class="transaction-detail-link" href="{{.DetailURL}}">查看流水详情</a></div></td>
              <td class="txn-col-amount"><span class="{{if eq .Direction "expense"}}amount expense{{else}}amount{{end}}">{{.AmountDisplay}}</span><div class="tiny">已分配 {{.AllocatedAmountDisplay}}</div><div class="tiny">余款 {{.RemainingAmountDisplay}}</div></td>
              <td class="mono txn-col-date">{{.DateDisplay}}<div class="tiny">租金月：{{if .FinalPeriodDisplay}}{{.FinalPeriodDisplay}}{{else}}未确认{{end}}</div></td>
              <td class="txn-col-desc"><div class="description">{{.Description}}</div></td>
              <td class="txn-col-account">{{.AccountName}}</td>
<td class="txn-col-action"><details class="txn-action" open><summary class="txn-summary"><span class="status {{.MatchStatus}}">{{.MatchStatusLabel}}</span><span class="txn-amount-mobile{{if eq .Direction "expense"}} expense{{end}}">{{.AmountDisplay}}</span></summary><div class="txn-body"><div class="tiny">{{.AllocationUseDisplay}}</div><a class="status status-link {{.MatchStatus}}" href="{{$.CanonicalPath}}?match_status={{.MatchStatus}}" title="筛选：{{.MatchStatusLabel}}">{{.MatchStatusLabel}}</a>{{if .MatchReason}}<div class="tiny">原因：{{.MatchReason}}</div>{{end}}{{if .CanConfirm}}<div class="tiny">建议：{{.CandidateTenantName}} · 租金月 {{.CandidatePeriod}}</div><form class="confirm-form" method="post" action="{{$.CanonicalPath}}/confirm"><input type="hidden" name="transaction_id" value="{{.ID}}"><input type="hidden" name="rent_obligation_id" value="{{.CandidateRentObligationID}}"><label class="tiny"><input type="checkbox" name="remember_payer" value="1" checked> 记住此付款人</label><button class="btn" type="submit">一键匹配</button></form>{{else if .NeedsMonthChoice}}<div class="tiny">已识别租客，请确认租金月份</div><form class="month-choice-form" method="post" action="{{$.CanonicalPath}}/confirm"><input type="hidden" name="transaction_id" value="{{.ID}}"><input type="hidden" name="tenant_id" value="{{.TenantID}}"><select name="period" aria-label="选择租金月份" required><option value="">选择月份...</option>{{range .MonthOptions}}<option value="{{.Period}}">{{.Label}} · 应收 {{.Expected}} · 已收 {{.Paid}} · 未收 {{.Remaining}}</option>{{end}}</select><label class="tiny"><input type="checkbox" name="remember_payer" value="1" checked> 记住此付款人</label><button class="btn" type="submit">确认匹配</button></form>{{else if and (eq .Direction "income") (ne .MatchStatus "matched") (ne .MatchStatus "ignored")}}{{if .ManualMatchOptions}}<form class="bind-form" method="post" action="{{$.CanonicalPath}}/confirm"><input type="hidden" name="transaction_id" value="{{.ID}}"><select name="rent_obligation_id" aria-label="选择匹配租客和租金月份" required><option value="">选择租客和租金月...</option>{{range .ManualMatchOptions}}<option value="{{.RentObligationID}}">{{.Label}}</option>{{end}}</select><label class="tiny"><input type="checkbox" name="remember_payer" value="1" checked> 记住付款人</label><button class="btn" type="submit">确认匹配</button></form>{{else}}<div class="tiny bind-empty">没有可直接匹配的租金月，可使用归类／拆分处理。</div>{{end}}{{end}}{{if and (eq .Direction "income") (ne .MatchStatus "matched") (ne .MatchStatus "ignored")}}<details class="allocation-details"><summary>归类／拆分</summary><form class="allocation-form" method="post" action="{{$.CanonicalPath}}/allocate"><input type="hidden" name="transaction_id" value="{{.ID}}"><select name="allocation_kind" aria-label="入账用途"><option value="rent">房租</option><option value="deposit">押金</option><option value="other_income">其他收入</option></select><select name="tenant_id" aria-label="归类租客"><option value="">不指定租客</option>{{range $.TenantOptions}}<option value="{{.ID}}">{{.Name}}</option>{{end}}</select><select name="period" aria-label="归类租金月份"><option value="">选择租金月份...</option>{{range .MonthOptions}}<option value="{{.Period}}">{{.Label}} · 余款 {{.Remaining}}</option>{{end}}</select><input name="amount" inputmode="decimal" value="{{.RemainingAmountInput}}" aria-label="归类金额" required><input name="note" placeholder="其他收入备注（其他收入必填）" aria-label="归类备注"><button class="btn" type="submit">保存归类</button></form></details>{{end}}{{if eq .MatchStatus "ignored"}}<form class="action-form" method="post" action="{{$.CanonicalPath}}/restore"><input type="hidden" name="transaction_id" value="{{.ID}}"><input name="reason" aria-label="恢复原因" placeholder="恢复原因" required><button class="btn" type="submit">恢复处理</button></form>{{else if or (eq .MatchStatus "matched") (eq .MatchStatus "partial")}}{{if .CanEditRentMatch}}<details class="rematch-details"><summary class="btn">修改匹配</summary><form class="rematch-form" method="post" action="{{$.CanonicalPath}}/rematch"><input type="hidden" name="transaction_id" value="{{.ID}}"><select name="tenant_id" aria-label="修改匹配租客" required><option value="">选择租客...</option>{{range .RematchTenantOptions}}<option value="{{.ID}}">{{.Name}}</option>{{end}}</select><select name="period" aria-label="修改租金月份" required><option value="">选择租金月份...</option>{{range .RematchMonthOptions}}<option value="{{.Period}}">{{.Label}} · 未收 {{.Remaining}}</option>{{end}}</select><button class="btn" type="submit">确认修改</button></form></details>{{else if .CanRematch}}<div class="tiny">当前没有可替换的租金目标；如需调整，请撤销后重新归类。</div>{{else}}<div class="tiny">该流水已拆分或含其他用途；请撤销后重新归类。</div>{{end}}<form class="action-form" method="post" action="{{$.CanonicalPath}}/revoke"><input type="hidden" name="transaction_id" value="{{.ID}}"><input name="reason" aria-label="撤销原因" placeholder="撤销原因" required><button class="btn danger" type="submit">撤销匹配</button></form>{{else if eq .Direction "income"}}<form class="action-form" method="post" action="{{$.CanonicalPath}}/ignore"><input type="hidden" name="transaction_id" value="{{.ID}}"><input name="reason" aria-label="忽略原因" placeholder="忽略原因" required><button class="btn" type="submit">无需匹配</button></form>{{end}}</div></details></td>
            </tr>
          {{end}}</tbody>
        </table></div>
        {{if eq .PageKey "transactions"}}
        <div class="transaction-route-mobile-list">
          {{range .TransactionRows}}
          <article class="transaction-review-card" id="mobile-transaction-{{.DetailKey}}">
            <div class="transaction-review-head">
              <div><h3>{{.PayerName}}</h3><p>{{if .DateShort}}{{.DateShort}}{{else}}{{.DateDisplay}}{{end}} · {{if .ObjectLabel}}{{.ObjectLabel}}{{else}}{{.AccountName}}{{end}}</p><p class="transaction-review-description">{{.Description}}</p></div>
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
              {{if .CanConfirm}}
              <form method="post" action="{{$.CanonicalPath}}/confirm"><input type="hidden" name="transaction_id" value="{{.ID}}"><input type="hidden" name="rent_obligation_id" value="{{.CandidateRentObligationID}}"><input type="hidden" name="remember_payer" value="0"><input type="hidden" name="return_to" value="/transactions"><button class="btn primary" type="submit">确认</button></form>
              {{else if .NeedsMonthChoice}}
              <form method="post" action="{{$.CanonicalPath}}/confirm"><input type="hidden" name="transaction_id" value="{{.ID}}"><input type="hidden" name="tenant_id" value="{{.TenantID}}"><input type="hidden" name="remember_payer" value="0"><input type="hidden" name="return_to" value="/transactions"><select name="period" aria-label="选择租金月份" required><option value="">选择月份</option>{{range .MonthOptions}}<option value="{{.Period}}">{{.Label}}</option>{{end}}</select><button class="btn primary" type="submit">确认匹配</button></form>
              {{else}}
              <a class="btn primary" href="{{.DetailURL}}">处理流水</a>
              {{end}}
            </div>
          </article>
          {{end}}
        </div>
        {{end}}
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
            <label for="page_size">每页<select id="page_size" name="page_size" onchange="this.form.submit()"><option value="25" {{if eq .PageSize 25}}selected{{end}}>25</option><option value="50" {{if eq .PageSize 50}}selected{{end}}>50</option><option value="100" {{if eq .PageSize 100}}selected{{end}}>100</option></select></label>
          </form>
          <div class="pagination-nav"><span class="tiny">第 {{.Page}} / {{.TotalPages}} 页，共 {{.TotalTransactions}} 笔</span>{{if .PreviousPageURL}}<a href="{{.PreviousPageURL}}">上一页</a>{{end}}{{if .NextPageURL}}<a href="{{.NextPageURL}}">下一页</a>{{end}}</div>
        </div>{{end}}
      </section>
    </main>
</div>
<script>(function(){document.querySelectorAll('form[action$="/allocate"]').forEach(function(form){var names=['allocation_kind','tenant_id','period','amount','note'];var controls=Array.from(form.children).filter(function(el){return el.name && names.indexOf(el.name)>=0;});if(!controls.length)return;var line=document.createElement('div');line.className='allocation-line';controls.forEach(function(el){el.name=el.name+'[]';line.appendChild(el);});var submit=form.querySelector('button[type="submit"]');var add=document.createElement('button');add.type='button';add.className='btn allocation-add';add.textContent='添加拆分项';add.addEventListener('click',function(){var clone=line.cloneNode(true);clone.querySelectorAll('input,select').forEach(function(el){if(el.name==='allocation_kind[]')el.value='rent';else if(el.tagName==='SELECT')el.value='';else el.value='';});form.insertBefore(clone,add);});form.insertBefore(line,submit);form.insertBefore(add,submit);});document.querySelectorAll('form[action^="/transactions/"],form[action^="/billing/"]').forEach(function(form){if(form.method.toLowerCase()!=='post'||form.querySelector('input[name="return_to"]'))return;var target=document.createElement('input');target.type='hidden';target.name='return_to';target.value=window.location.pathname+window.location.search;form.appendChild(target);});document.querySelectorAll('form[action$="/revoke"]').forEach(function(form){form.method='get';var reason=form.querySelector('input[name="reason"]');if(reason)reason.remove();});if(window.matchMedia('(max-width: 640px)').matches){document.querySelectorAll('details.txn-action[open]').forEach(function(node){node.removeAttribute('open');});document.querySelector('.transaction-route-filters[open]')?.removeAttribute('open');}})();</script>
</body>
</html>
`)
