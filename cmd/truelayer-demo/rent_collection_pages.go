package main

import (
	"html/template"
	"strings"
)

func billsPageURL(period, search, status, sortValue string, page, pageSize int) string {
	return strings.Replace(rentDashboardURL(period, search, status, sortValue, page, pageSize), "/rent-dashboard", "/bills", 1)
}

var rentCollectionPageFuncs = template.FuncMap{
	"billsPageURL": billsPageURL,
	"dunningDeliveryLabel": func(status string) string {
		switch status {
		case dunningDeliveryAccepted:
			return "排队中"
		case dunningDeliverySent:
			return "已发送"
		case dunningDeliveryFailed:
			return "发送失败"
		case dunningDeliverySkipped:
			return "已跳过"
		default:
			return "未发送"
		}
	},
	"dashboardPreviousPage": func(page int) int {
		if page <= 1 {
			return 1
		}
		return page - 1
	},
	"dashboardNextPage": func(page, totalPages int) int {
		if page >= totalPages {
			return totalPages
		}
		return page + 1
	},
}

var billsPageTemplate = newWorkspacePageTemplate("bills-page", rentCollectionPageFuncs, `<!doctype html>
<html lang="zh-CN"><head><meta charset="utf-8"><meta name="viewport" content="width=device-width, initial-scale=1"><title>RentOps 应收账单</title><link rel="stylesheet" href="/static/css/workspace.css"><link rel="stylesheet" href="/static/css/pages/collection-pages.css"></head>
<body><div class="app">{{template "workspace-nav" .}}<main class="content workspace collection-page bills-page">
  <header class="collection-toolbar"><div><div class="eyebrow">{{.PeriodLabel}} · RENT COLLECTION</div><h1>应收账单</h1><p class="tiny">按租客责任核对本月应收、覆盖与未付金额。</p></div><div class="collection-primary-actions"><a class="btn primary bills-desktop-action" href="/bills?period={{.Period}}">生成本月账单</a><a class="btn primary bills-mobile-action" href="/cash-receipts?add=1&amp;period={{.Period}}">补录现金</a></div></header>
  {{if .Message}}<div class="notice ok">{{.Message}}</div>{{end}}{{if .Error}}<div class="notice error">{{.Error}}</div>{{end}}
  <form class="collection-filters bills-filters" method="get" action="/bills"><label class="bills-search"><span class="sr-only">搜索</span><input type="search" name="search" value="{{.SearchFilter}}" placeholder="搜索当前列表" aria-label="搜索当前列表"></label><label><span class="sr-only">状态</span><select name="status" onchange="this.form.submit()"><option value="unpaid"{{if or (eq .StatusFilter "unpaid") (eq .StatusFilter "all")}} selected{{end}}>未结清</option><option value="all">全部账单</option><option value="paid"{{if eq .StatusFilter "paid"}} selected{{end}}>已结清</option><option value="overdue"{{if eq .StatusFilter "overdue"}} selected{{end}}>逾期</option><option value="partial"{{if eq .StatusFilter "partial"}} selected{{end}}>部分缴纳</option><option value="open"{{if eq .StatusFilter "open"}} selected{{end}}>未到期未缴</option></select></label><label><span class="sr-only">账单月份</span><input type="month" name="period" value="{{.Period}}" aria-label="账单月份"></label><input type="hidden" name="sort" value="{{.SortFilter}}"><button class="btn" type="submit">筛选</button><a class="btn subtle" href="/bills?period={{.Period}}">清除</a></form>
  <section class="panel surface collection-list bills-list"><div class="panel-head"><div><h2>账单列表</h2><p class="tiny">{{.FilteredCount}} 条账单</p></div></div>
    {{if .Rows}}<div class="table-wrap collection-table-wrap"><table class="collection-table"><thead><tr><th>账单</th><th>租客 / 房间</th><th>应缴日</th><th>应收</th><th>已覆盖</th><th>未付</th><th>状态</th><th>操作</th></tr></thead><tbody>{{range .Rows}}<tr><td class="mono">{{.Period}}</td><td><a href="/tenants/{{.TenantID}}?from_month={{.Period}}&amp;to_month={{.Period}}"><strong>{{.TenantName}}</strong></a><span class="tiny collection-room-label">{{if .RoomAddress}}{{.RoomAddress}}{{if .RoomLabel}} · {{.RoomLabel}}{{end}}{{else}}{{.RoomLabel}}{{end}}</span></td><td class="mono">{{.DueDate}}</td><td class="amount">{{.ExpectedAmount}}</td><td class="amount">{{.PaidAmount}}</td><td class="amount">{{.BalanceAmount}}</td><td><span class="status {{.Status}}">{{.StatusLabel}}</span></td><td><div class="collection-row-actions">{{if gt .ExpectedCents .PaidCents}}<details class="collection-settle"><summary class="btn primary">一键平账</summary><form method="post" action="/bills/settle" class="collection-balance-form"><input type="hidden" name="obligation_id" value="{{.ObligationID}}"><input type="hidden" name="period" value="{{$.Period}}"><input type="hidden" name="search" value="{{$.SearchFilter}}"><input type="hidden" name="status" value="{{$.StatusFilter}}"><input type="hidden" name="sort" value="{{$.SortFilter}}"><input type="hidden" name="page" value="{{$.Page}}"><input type="hidden" name="page_size" value="{{$.PageSize}}"><label class="sr-only">平账原因<input name="reason" maxlength="512" placeholder="平账原因" aria-label="填写平账原因" required></label><button class="btn" type="submit" data-confirm="true">确认平账</button></form></details>{{else}}<a class="btn subtle" href="/tenants/{{.TenantID}}?from_month={{.Period}}&amp;to_month={{.Period}}">查看</a>{{end}}</div></td></tr>{{end}}</tbody></table></div>
    <div class="collection-mobile-list">{{range .Rows}}<article class="collection-bill-card"><div class="collection-bill-top"><div><h3>{{.TenantName}}</h3><p>{{if .RoomAddress}}{{.RoomAddress}}{{if .RoomLabel}} · {{.RoomLabel}}{{end}}{{else}}{{.RoomLabel}}{{end}} · {{.Period}} 租金</p></div><strong class="collection-bill-amount">{{.ExpectedAmount}}</strong></div><div class="collection-bill-facts"><div><span>已覆盖</span><strong>{{.PaidAmount}}</strong></div><div><span>剩余未付</span><strong>{{.BalanceAmount}}</strong></div></div><div class="collection-bill-actions"><a class="btn" href="/tenants/{{.TenantID}}?from_month={{.Period}}&amp;to_month={{.Period}}">租客详情</a>{{if gt .ExpectedCents .PaidCents}}<details class="collection-settle"><summary class="btn primary">一键平账</summary><form method="post" action="/bills/settle" class="collection-balance-form"><input type="hidden" name="obligation_id" value="{{.ObligationID}}"><input type="hidden" name="period" value="{{$.Period}}"><input type="hidden" name="search" value="{{$.SearchFilter}}"><input type="hidden" name="status" value="{{$.StatusFilter}}"><input type="hidden" name="sort" value="{{$.SortFilter}}"><input type="hidden" name="page" value="{{$.Page}}"><input type="hidden" name="page_size" value="{{$.PageSize}}"><label class="sr-only">平账原因<input name="reason" maxlength="512" placeholder="平账原因" aria-label="填写平账原因" required></label><button class="btn" type="submit" data-confirm="true">确认平账</button></form></details>{{end}}</div></article>{{end}}</div>
    <nav class="collection-pager" aria-label="账单分页"><label>每页<select name="page_size" form="bills-page-size" onchange="this.form.submit()"><option value="12"{{if eq .PageSize 12}} selected{{end}}>12</option><option value="24"{{if eq .PageSize 24}} selected{{end}}>24</option><option value="50"{{if eq .PageSize 50}} selected{{end}}>50</option></select></label><form id="bills-page-size" method="get" action="/bills"><input type="hidden" name="period" value="{{.Period}}"><input type="hidden" name="search" value="{{.SearchFilter}}"><input type="hidden" name="status" value="{{.StatusFilter}}"><input type="hidden" name="sort" value="{{.SortFilter}}"></form><span class="tiny">第 {{.Page}} / {{.TotalPages}} 页</span><div><a class="btn subtle{{if eq .Page 1}} disabled{{end}}" href="{{billsPageURL .Period .SearchFilter .StatusFilter .SortFilter (dashboardPreviousPage .Page) .PageSize}}">上一页</a><a class="btn subtle{{if eq .Page .TotalPages}} disabled{{end}}" href="{{billsPageURL .Period .SearchFilter .StatusFilter .SortFilter (dashboardNextPage .Page .TotalPages) .PageSize}}">下一页</a></div></nav>
    {{else}}<div class="workspace-empty">{{if gt .TotalRows 0}}没有符合当前筛选条件的账单。{{else}}本月暂无应收账单；现有租客账单不会自动转换为房产数据。{{end}}</div>{{end}}
  </section>
</main></div><script>document.addEventListener("submit",event=>{const button=event.submitter;if(button?.dataset.confirm==="true"&&!window.confirm("确认按剩余未付金额平账吗？"))event.preventDefault()});</script></body></html>`)

var dunningPageTemplate = newWorkspacePageTemplate("dunning-page", rentCollectionPageFuncs, `<!doctype html>
<html lang="zh-CN"><head><meta charset="utf-8"><meta name="viewport" content="width=device-width, initial-scale=1"><title>RentOps 催收任务</title><link rel="stylesheet" href="/static/css/workspace.css"><link rel="stylesheet" href="/static/css/pages/collection-pages.css"></head>
<body><div class="app">{{template "workspace-nav" .}}<main class="content workspace collection-page dunning-page">
  <header class="collection-toolbar"><div><div class="eyebrow">{{.PeriodLabel}} · RENT FOLLOW-UP</div><h1>催收任务</h1><p class="tiny">按本月未收金额排序，先核对责任，再发送提醒。</p></div><form class="collection-period" method="get" action="/dunning"><label for="dunning-period">账单月份</label><input id="dunning-period" type="month" name="period" value="{{.Dunning.Period}}"><button class="btn" type="submit">查看</button></form></header>
  {{if .Dunning.Error}}<div class="notice error" role="alert">{{.Dunning.Error}}</div>{{end}}{{if .Dunning.Notice}}<div class="notice ok" role="status">{{.Dunning.Notice}}</div>{{end}}
  <section class="collection-summary dunning-summary"><article class="panel metric metric-primary"><div class="label">未收责任</div><strong>{{.UnpaidCount}}</strong><span>当前账单月份</span></article><article class="panel metric metric-warning"><div class="label">逾期责任</div><strong>{{.OverdueCount}}</strong><span>优先核对</span></article><article class="panel metric"><div class="label">本页候选</div><strong>{{len .Dunning.Candidates}}</strong><span>按金额和状态排序</span></article><article class="panel metric"><div class="label">发件配置</div><strong>{{if .Dunning.SenderConfigured}}已配置{{else}}待配置{{end}}</strong><span>{{.Dunning.Sender.ReplyToEmail}}</span></article></section>
  <div class="dunning-layout"><section class="panel surface"><div class="panel-head"><div><h2>待核对责任</h2><p class="tiny">只会选择存在有效邮箱且当前未结清的责任。</p></div></div>
    <form method="post" action="/dunning/preview" class="dunning-candidate-form"><input type="hidden" name="period" value="{{.Dunning.Period}}"><input type="hidden" name="search" value="{{.Dunning.SearchFilter}}"><input type="hidden" name="status" value="{{.Dunning.StatusFilter}}"><input type="hidden" name="sort" value="{{.Dunning.SortFilter}}"><input type="hidden" name="page" value="{{.Dunning.Page}}"><input type="hidden" name="page_size" value="{{.Dunning.PageSize}}"><input type="hidden" name="request_key" value="{{.Dunning.RequestKey}}">
    {{if .Dunning.Candidates}}{{range .Dunning.Candidates}}<label class="dunning-candidate{{if not .Selectable}} disabled{{end}}"><input type="checkbox" name="obligation_id" value="{{.ObligationID}}"{{if .Selected}} checked{{end}}{{if not .Selectable}} disabled{{end}}><span class="candidate-main"><strong>{{.TenantName}}</strong><span class="tiny">{{if .RoomLabel}}{{.RoomLabel}} · {{end}}{{.RoomAddress}} · 应缴 {{.DueDate}}</span>{{if not .EmailValid}}<span class="tiny candidate-warning">{{.EmailError}}</span>{{else if .SentToday}}<span class="tiny">今天已发送提醒</span>{{end}}</span><span class="candidate-balance"><strong>{{.BalanceAmount}}</strong><span class="status {{.Status}}">{{.StatusLabel}}</span></span></label>{{end}}{{else}}<div class="workspace-empty">当前月份没有待催收的未付责任。</div>{{end}}
    {{if .Dunning.Candidates}}<div class="dunning-actions"><button class="btn subtle" type="submit" formaction="/dunning/preview">预览提醒</button><label class="resend-confirm"><input type="checkbox" name="confirm_resend" value="1">确认同日重发</label><button class="btn primary" type="submit" formaction="/dunning/send" data-confirm="true">发送已选</button></div>{{end}}</form>
    {{if .Dunning.PreviewRows}}<section class="dunning-feedback"><h3>发送前预览</h3>{{range .Dunning.PreviewRows}}<article><strong>{{.Candidate.TenantName}}</strong>{{if .Message}}<p>{{.Message.Subject}} · {{.Message.RecipientEmail}}</p><pre>{{.Message.Body}}</pre>{{else}}<p class="notice error">{{.Error}}</p>{{end}}</article>{{end}}</section>{{end}}
    {{if .Dunning.Results}}<section class="dunning-feedback"><h3>发送结果</h3>{{range .Dunning.Results}}<article><strong>{{.Candidate.TenantName}}</strong><p>{{if .Attempt}}{{dunningDeliveryLabel .Attempt.DeliveryStatus}}{{else}}{{.Error}}{{end}}</p>{{if .Error}}<p class="notice error">{{.Error}}</p>{{end}}</article>{{end}}</section>{{end}}
  </section><aside class="panel surface dunning-config"><div class="panel-head"><div><h2>发件配置</h2><p class="tiny">租客回复地址和显示名称。</p></div></div>{{if .Dunning.ConfigurationError}}<div class="notice error">{{.Dunning.ConfigurationError}}</div>{{end}}<form method="post" action="/dunning/config"><input type="hidden" name="period" value="{{.Dunning.Period}}"><input type="hidden" name="search" value="{{.Dunning.SearchFilter}}"><input type="hidden" name="status" value="{{.Dunning.StatusFilter}}"><input type="hidden" name="sort" value="{{.Dunning.SortFilter}}"><input type="hidden" name="page" value="{{.Dunning.Page}}"><input type="hidden" name="page_size" value="{{.Dunning.PageSize}}"><label>显示名称<input name="display_name" value="{{.Dunning.Sender.DisplayName}}" autocomplete="organization" required></label><label>回复邮箱<input name="reply_to_email" type="email" value="{{.Dunning.Sender.ReplyToEmail}}" autocomplete="email" required></label><button class="btn" type="submit">保存配置</button></form></aside></div>
</main></div><script>document.addEventListener("submit",event=>{const button=event.submitter;if(button?.dataset.confirm==="true"&&!window.confirm("确认发送所选催收提醒吗？"))event.preventDefault()});</script></body></html>`)
