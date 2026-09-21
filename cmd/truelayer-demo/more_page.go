package main

type morePageData struct {
	workspaceShell
}

var morePageTemplate = newWorkspacePageTemplate("more-page", nil, `<!doctype html>
<html lang="zh-CN">
<head>
  <meta charset="utf-8">
  <meta name="viewport" content="width=device-width, initial-scale=1">
  <title>RentOps 更多功能</title>
  <style>`+workspacePageCSS+`
    .more-page { max-width: 960px; }
    .more-page-head { margin-bottom: 18px; }
    .more-page-eyebrow { margin: 0 0 5px; color: var(--accent); font: 11px var(--mono); letter-spacing: .08em; }
    .more-page-head h1 { margin: 0; }
    .more-page-head p { margin: 5px 0 0; color: var(--foreground-muted); }
    .more-page-grid { display: grid; grid-template-columns: repeat(3, minmax(0, 1fr)); gap: 12px; }
    .more-page-card { min-height: 126px; padding: 15px; border: 1px solid var(--border); border-radius: 12px; background: var(--surface); color: var(--foreground); text-decoration: none; transition: border-color .16s ease, transform .16s ease, background .16s ease; }
    .more-page-card:hover, .more-page-card:focus-visible { border-color: var(--accent); background: var(--surface-accent); outline: none; transform: translateY(-1px); }
    .more-page-icon { display: grid; width: 32px; height: 32px; place-items: center; border-radius: 9px; background: var(--surface-muted); color: var(--accent); font: 700 11px var(--mono); }
    .more-page-card strong { display: block; margin-top: 15px; font-size: 14px; }
    .more-page-card span:last-child { display: block; margin-top: 3px; color: var(--foreground-muted); font-size: 11px; }
    @media (max-width: 640px) {
      .more-page-grid { grid-template-columns: repeat(2, minmax(0, 1fr)); gap: 9px; }
      .more-page-card { min-height: 108px; padding: 14px; }
      .more-page-card strong { margin-top: 13px; }
    }
    @media (max-width: 359px) { .more-page-grid { grid-template-columns: 1fr; } }
  </style>
</head>
<body>
  <div class="app">
    {{template "workspace-nav" .}}
    <main class="content more-page">
      <header class="more-page-head">
        <p class="more-page-eyebrow">全部功能</p>
        <h1>更多</h1>
        <p>低频操作集中在这里。</p>
      </header>
      <nav class="more-page-grid" aria-label="更多功能">
        <a class="more-page-card" href="/dunning"><span class="more-page-icon">D</span><strong>催收任务</strong><span>跟进未结清的租金责任</span></a>
        <a class="more-page-card" href="/cash-receipts"><span class="more-page-icon">C</span><strong>现金收款</strong><span>补录并核对现金租金</span></a>
        <a class="more-page-card" href="/expenses"><span class="more-page-icon">E</span><strong>房屋支出</strong><span>记录房产和房间支出</span></a>
        <a class="more-page-card" href="/bank"><span class="more-page-icon">B</span><strong>银行设置</strong><span>同步与授权状态</span></a>
      </nav>
    </main>
  </div>
</body>
</html>`)
