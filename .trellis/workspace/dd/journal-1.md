# Journal - dd (Part 1)

> AI development session journal
> Started: 2026-09-08

---



## Session 1: Tenant billing income demo

**Date**: 2026-09-08
**Task**: Tenant billing income demo
**Branch**: `main`

### Summary

Implemented demo login, protected TrueLayer bank routes, redirected successful authorization back to billing, and rendered latest bank income transactions with payer id/name confidence.

### Main Changes

- Added confirmed property, room, and tenant deletion from their detail pages, with transaction-scoped ownership checks and dependency guards that preserve rent, payment, expense, receipt, and dunning history.
- Replaced repeated direct-match month options with the shared calendar and added the selected month's outstanding-rent helper; the remembered-tenant month path uses the same control.
- Enabled shared case-insensitive substring tenant search on every tenant picker, including room rent-plan members and actual cash payers.

### Git Commits

| Hash | Message |
|------|---------|
| `0261c5f` | (see git log) |

### Testing

- [OK] `go test ./... -count=1`
- [OK] `go vet ./...`
- [OK] disposable-MySQL deletion guard regression

### Status

[OK] **Completed**

### Next Steps

- None - task complete


## Session 2: Add tenant and expense ledgers

**Date**: 2026-09-09
**Task**: Add tenant and expense ledgers
**Branch**: `main`

### Summary

Added sidebar navigation plus manual tenant and expense ledger pages backed by local JSON files, with Go tests and README/spec updates.

### Main Changes

(Add details)

### Git Commits

| Hash | Message |
|------|---------|
| `cf15a95` | (see git log) |

### Testing

- [OK] (Add test results)

### Status

[OK] **Completed**

### Next Steps

- None - task complete


## Session 3: MySQL租金匹配工作台

**Date**: 2026-09-10
**Task**: MySQL租金匹配工作台
**Branch**: `main`

### Summary

完成 RentOps 从 JSON 到 GORM/MySQL 的账号隔离改造，新增月度租金工作台、租客付款方匹配和人工确认、流水收入/支出筛选、旧 JSON/JSONL 幂等导入及本地启动配置；测试、race、vet 通过。启动验证因本机 3306 无 MySQL 服务未能运行页面。

### Main Changes

(Add details)

### Git Commits

| Hash | Message |
|------|---------|
| `b21539b` | (see git log) |

### Testing

- [OK] (Add test results)

### Status

[OK] **Completed**

### Next Steps

- None - task complete


## Session 4: Monthly rent dashboard and payer matching UX

**Date**: 2026-09-14
**Task**: Monthly rent dashboard and payer matching UX
**Branch**: `main`

### Summary

Implemented the Chinese monthly rent dashboard with clear expected/paid/balance totals, progress and month navigation; unified rounded controls and mobile layout; remembered exact payer names and batch-associated same-name income transactions; ambiguous months remain tenant-associated for explicit user selection; added migration, regression coverage, and database spec.

### Main Changes

(Add details)

### Git Commits

| Hash | Message |
|------|---------|
| `5dd03a2` | (see git log) |

### Testing

- [OK] (Add test results)

### Status

[OK] **Completed**

### Next Steps

- None - task complete


## Session 5: 账单日历与流水匹配修复

**Date**: 2026-09-15
**Task**: 账单日历与流水匹配修复
**Branch**: `main`

### Summary

统一全局日历为圆角自定义选择器；银行流水选月后自动搜索；首页待处理数量按月份联动并可跳转；租客页新增最近三个月账单与确认流水层级；补充 JULY26、SEP26 等紧凑英文月份解析及回归测试。

### Main Changes

(Add details)

### Git Commits

| Hash | Message |
|------|---------|
| `039d88b` | (see git log) |

### Testing

- [OK] (Add test results)

### Status

[OK] **Completed**

### Next Steps

- None - task complete


## Session 6: EUR 收租账本基础实现与测试

**Date**: 2026-09-16
**Task**: EUR 收租账本基础实现与测试
**Branch**: `main`

### Summary

将收租工作台一期货币范围收敛为 EUR-only 并保留三位币种扩展口子；完成账本基础迁移、有效分配/作废投影、预算与归属校验，补充单元测试和可选真实 MySQL 迁移幂等测试。安全 rebase 远端改动后已推送 origin/main；归档 rent-ledger-foundation 子任务。

### Main Changes

(Add details)

### Git Commits

| Hash | Message |
|------|---------|
| `80c661b` | (see git log) |
| `052bebc` | (see git log) |
| `f3791e8` | (see git log) |
| `4a7fff2` | (see git log) |
| `342ceec` | (see git log) |

### Testing

- [OK] (Add test results)

### Status

[OK] **Completed**

### Next Steps

- None - task complete


## Session 7: 完成租客档案与付款人关系

**Date**: 2026-09-16
**Task**: 完成租客档案与付款人关系
**Branch**: `main`

### Summary

完成租客档案别名与邮箱、付款人关系表及软删除、现有 JSON 仅姓名付款人兼容、租客缴费历史详情与分页、租期生命周期作废规则、银行/现金来源展示；通过 go test ./...、go vet ./... 和任务校验，MySQL 集成测试因未配置 DSN 跳过。

### Main Changes

(Add details)

### Git Commits

| Hash | Message |
|------|---------|
| `280e21a` | (see git log) |
| `e220702` | (see git log) |
| `860664f` | (see git log) |
| `fef32ce` | (see git log) |
| `40e554c` | (see git log) |
| `3347878` | (see git log) |
| `48ccd1a` | (see git log) |

### Testing

- [OK] (Add test results)

### Status

[OK] **Completed**

### Next Steps

- None - task complete


## Session 8: 完成银行入账归类与纠错

**Date**: 2026-09-16
**Task**: 完成银行入账归类与纠错
**Branch**: `main`

### Summary

完成银行同步覆盖、严格付款人匹配、EUR 原子归类拆分、部分分配余款、忽略恢复撤销审计、筛选分页、撤销预览和历史付款人逐笔预览；通过 go test ./...、go vet ./...、git diff --check。临时 MySQL 8.4 初始化崩溃，opt-in 集成测试未执行；浏览器运行时不可用，页面契约由 Go 模板测试覆盖。代码已推送并归档任务。

### Main Changes

(Add details)

### Git Commits

| Hash | Message |
|------|---------|
| `91cac85` | (see git log) |
| `d233e72` | (see git log) |
| `ec51805` | (see git log) |
| `c5a65b5` | (see git log) |
| `8ac6917` | (see git log) |
| `1746fa8` | (see git log) |
| `4810dcd` | (see git log) |

### Testing

- [OK] (Add test results)

### Status

[OK] **Completed**

### Next Steps

- None - task complete


## Session 9: 完成现金租金补录与作废更正

**Date**: 2026-09-16
**Task**: 完成现金租金补录与作废更正
**Branch**: `main`

### Summary

完成独立现金收款账本、EUR integer-cent 校验、预览与原子入账、同月银行+现金投影、幂等与作废纠正、租客历史和月度总览来源展示；增加 MySQL 可选迁移幂等、预览无写入、用户隔离和并发余额竞争测试，并更新 backend database spec。全量 go test、go vet、git diff --check 通过；本机未配置 RENTOPS_MYSQL_TEST_DSN，MySQL 集成测试按约定跳过。

### Main Changes

(Add details)

### Git Commits

| Hash | Message |
|------|---------|
| `c9d18df` | (see git log) |
| `af143ea` | (see git log) |
| `3eea6f3` | (see git log) |
| `8e94cf2` | (see git log) |
| `e4f9dc5` | (see git log) |
| `3cce4e6` | (see git log) |

### Testing

- [OK] (Add test results)

### Status

[OK] **Completed**

### Next Steps

- None - task complete


## Session 10: 完成月度收租 Dashboard

**Date**: 2026-09-16
**Task**: 完成月度收租 Dashboard
**Branch**: `main`

### Summary

完成月度收租 Dashboard：全量月度汇总与状态计数、搜索筛选排序分页、银行到账月待分配与其他收入指标、同步成功/异常/无同步状态、现金与银行明细及租客历史入口；补充模板/HTTP/单元/MySQL 集成测试与 backend database spec，go test/go vet/git diff --check 通过；MySQL 集成因未配置 RENTOPS_MYSQL_TEST_DSN 跳过。

### Main Changes

(Add details)

### Git Commits

| Hash | Message |
|------|---------|
| `997d199` | (see git log) |

### Testing

- [OK] (Add test results)

### Status

[OK] **Completed**

### Next Steps

- None - task complete


## Session 11: 完成单月邮件催缴

**Date**: 2026-09-16
**Task**: 完成单月邮件催缴
**Branch**: `main`

### Summary

完成单月邮件催缴：新增租户隔离的候选读模型与 008 迁移、固定英文提醒/逾期模板、SMTP 边界和配置、预览纯读、逐人发送审计、请求幂等、并发重复点击保护、同日确认重发、失败单项重试；在月度 Dashboard 嵌入响应式抽屉与配置/预览/发送入口。全量 go test、go vet、git diff --check 通过；MySQL/真实浏览器验收因环境未提供 RENTOPS_MYSQL_TEST_DSN/DevTools 留待集成环境。

### Main Changes

(Add details)

### Git Commits

| Hash | Message |
|------|---------|
| `522b706` | (see git log) |
| `4c812db` | (see git log) |
| `b247ed9` | (see git log) |

### Testing

- [OK] (Add test results)

### Status

[OK] **Completed**

### Next Steps

- None - task complete


## Session 12: 完成全站 UI Flat Design 重构

**Date**: 2026-09-16
**Task**: 完成全站 UI Flat Design 重构
**Branch**: `main`

### Summary

按用户授权跳过仍受环境阻塞的 API live gate，完成 UI 任务：建立设计与实现计划，统一工作台交互基础，替换登录/工作台/日历/账单/仪表盘/租客/支出/现金收款/预览页面为浅色 Flat Design，修正移动端仪表盘筛选布局。通过 go test ./...、go vet、git diff --check，并用 Chromium 验证 10 个页面在 320/360/375/390/412/768/1024/1440 CSS px 无 body 横向溢出，交互与控制台检查通过。UI 任务已归档；API 任务保持 in_progress。

### Main Changes

(Add details)

### Git Commits

| Hash | Message |
|------|---------|
| `7eb23af` | (see git log) |
| `8bf1c8a` | (see git log) |
| `0fb4cdb` | (see git log) |
| `9a47273` | (see git log) |
| `ecff7f4` | (see git log) |

### Testing

- [OK] (Add test results)

### Status

[OK] **Completed**

### Next Steps

- None - task complete


## Session 13: 移动端适配：审计、整改、真机前验收

**Date**: 2026-09-16
**Task**: 移动端适配：审计、整改、真机前验收
**Branch**: `main`

### Summary

把 10 个页面在真实浏览器里按 375/390/768 量了一遍，然后整改。审计自己的结论被实测推翻：固定像素列宽并不导致页面级横向溢出（21 条页面×视口记录全部为 0），真正的缺陷是滚动容器内被切掉的列、26-32px 的触控目标、11px 正文。整改内容：抽出共享外壳 workspaceNav（原侧栏有 7 份漂移的拷贝）；≤640 侧栏改为纯 CSS 抽屉；冻结最右操作列；/billing 用就地展开的 <details> 代替无法冻结的 320px 操作列，金额移进冻结单元格；44px 触控下限（含两个绕开共享样式的预览模板）；/tenant-detail 档案标签列改单列。新增 8 个 Go 测试断言渲染结果不变量，Playwright 夹具（29 个脚本）复测每一条结论，桌面端与改动前构建逐页比对（6/8 字节一致，2 处已归因）。自查抓到并修掉 3 个我自己引入的可见回退：44px 姓名链接把虚线下划线推到文字下方 22px、抽屉复选框带 hidden 导致窄屏导航只能点不能按键盘、关闭态侧栏仅 translate 仍留在 Tab 顺序里。未修项（5 条 P1-4 排版、/billing 在 900–1440px 的桌面横向溢出、真机验证）连同理由记在 research/mobile-audit.md。任务按要求压成单个提交 0f92566。

### Main Changes

(Add details)

### Git Commits

| Hash | Message |
|------|---------|
| `0f92566` | (see git log) |

### Testing

- [OK] (Add test results)

### Status

[OK] **Completed**

### Next Steps

- None - task complete


## Session 14: Simplify billing and dashboard summaries

**Date**: 2026-09-16
**Task**: Simplify billing and dashboard summaries
**Branch**: `main`

### Summary

Removed technical transaction identifiers, kept only the confirmed rent month, renamed the arrival-month filter, and hid non-essential Dashboard income cards while keeping data flows intact. Added rendering and E2E regression coverage.

### Main Changes

(Add details)

### Git Commits

| Hash | Message |
|------|---------|
| `ea1a1f5` | (see git log) |

### Testing

- [OK] (Add test results)

### Status

[OK] **Completed**

### Next Steps

- None - task complete


## Session 15: Dashboard manual rent balance

**Date**: 2026-09-16
**Task**: Dashboard manual rent balance
**Branch**: `main`

### Summary

Added a confirmed one-click Dashboard rent settlement that creates an auditable 手动平账 income transaction for the locked outstanding balance, with atomic allocation, concurrency and account guards, confirmation UI, regression coverage, and ledger-spec documentation.

### Main Changes

(Add details)

### Git Commits

| Hash | Message |
|------|---------|
| `ee5a5f6` | (see git log) |

### Testing

- [OK] (Add test results)

### Status

[OK] **Completed**

### Next Steps

- None - task complete


## Session 16: Manual transaction matching and independent rematch selectors

**Date**: 2026-09-16
**Task**: Manual transaction matching and independent rematch selectors
**Branch**: `main`

### Summary

Disabled automatic transaction matching writes, added explicit one-click billing matching and safe rent rematching, and changed rematch UI to independent tenant and rent-month selectors. Tests and vet passed; pushed to origin/main.

### Main Changes

(Add details)

### Git Commits

| Hash | Message |
|------|---------|
| `4e3e6da` | (see git log) |

### Testing

- [OK] (Add test results)

### Status

[OK] **Completed**

### Next Steps

- None - task complete


## Session 17: Complete landlord rent workspace implementation

**Date**: 2026-09-18
**Task**: Complete landlord rent workspace implementation
**Branch**: `main`

### Summary

Completed the landlord multi-property rent workspace: schema and repository layers, rent ledger and dashboard handlers, externalized frontend resources, responsive desktop/mobile views, hidden billing references, and integration verification. Existing unrelated task docs, audit fixtures, scripts, and test data remain uncommitted.

### Main Changes

(Add details)

### Git Commits

| Hash | Message |
|------|---------|
| `a9a7dc1` | (see git log) |
| `1d99375` | (see git log) |
| `3d5fd8f` | (see git log) |
| `c43d10d` | (see git log) |
| `238e7f6` | (see git log) |
| `4eb1e91` | (see git log) |
| `3754c5e` | (see git log) |
| `08b81f6` | (see git log) |
| `1a9aa4a` | (see git log) |
| `68cebc4` | (see git log) |
| `3866af6` | (see git log) |
| `1132e69` | (see git log) |
| `71ad019` | (see git log) |

### Testing

- [OK] (Add test results)

### Status

[OK] **Completed**

### Next Steps

- None - task complete


## Session 18: 完成 Figma 领域任务与 MySQL 清理测试

**Date**: 2026-09-19
**Task**: 完成 Figma 领域任务与 MySQL 清理测试
**Branch**: `main`

### Summary

拆分 Figma 原型还原任务；完成领域与运营能力、迁移、租住安排、平账原因和发票 URL；新增隔离 MySQL 测试脚本，测试前后重建 rentops_test 并验证领域相关测试通过。

### Main Changes

(Add details)

### Git Commits

| Hash | Message |
|------|---------|
| `4ae94fa` | (see git log) |
| `6b0f5fb` | (see git log) |
| `566b040` | (see git log) |

### Testing

- [OK] (Add test results)

### Status

[OK] **Completed**

### Next Steps

- None - task complete


## Session 19: 完成 Figma 页面数据与规范路由

**Date**: 2026-09-19
**Task**: 完成 Figma 页面数据与规范路由
**Branch**: `main`

### Summary

完成用户隔离的 bills、transactions、dunning、properties、rooms、tenancies、cash receipts、bank 页面数据模型与规范路由，保留旧入口；加入平账原因、催收强确认、refresh-token 同步、房产编辑和页面语义字段。定向 Go/vet 与 MySQL 清理隔离测试通过；完整包仍有两个前置租客旧 fixture 测试失败。

### Main Changes

(Add details)

### Git Commits

| Hash | Message |
|------|---------|
| `dd95fa1` | (see git log) |

### Testing

- [OK] (Add test results)

### Status

[OK] **Completed**

### Next Steps

- None - task complete


## Session 20: 完成 Figma 桌面工作台切片

**Date**: 2026-09-19
**Task**: 完成 Figma 桌面工作台切片
**Branch**: `main`

### Summary

完成桌面端 Figma shell、四项总览指标、房产/房间新建编辑、租客绑定房间、停用确认、催收焦点交互；记录 1366/1440 浏览器验收与截图。定向测试、go vet 和 MySQL 隔离测试通过；完整测试仅剩两个既有租客日期 fixture 失败。归档 09-19-figma-desktop-workspace，下一步使用现有 09-19-figma-mobile-workspace。

### Main Changes

(Add details)

### Git Commits

| Hash | Message |
|------|---------|
| `97f8813` | (see git log) |
| `36b1c7a` | (see git log) |
| `2337477` | (see git log) |
| `40b846b` | (see git log) |
| `a54ab23` | (see git log) |
| `5d54b8b` | (see git log) |
| `a26f468` | (see git log) |

### Testing

- [OK] (Add test results)

### Status

[OK] **Completed**

### Next Steps

- None - task complete


## Session 21: Figma mobile workspace cards and navigation

**Date**: 2026-09-19
**Task**: Figma mobile workspace cards and navigation
**Branch**: `main`

### Summary

Completed the existing 09-19 mobile workspace task: five-item bottom navigation and flyouts; dashboard, billing, tenant, expense, and transaction mobile cards; shared object-table cardization; mobile dunning bottom sheet with strong confirmation; 360/390/430/600 browser overflow checks; go vet and MySQL isolation verification. Full module tests still have the two pre-existing tenant-date fixture failures.

### Main Changes

(Add details)

### Git Commits

| Hash | Message |
|------|---------|
| `6a80d2b` | (see git log) |
| `a5ec2b1` | (see git log) |
| `7fcbc0e` | (see git log) |
| `013c8c0` | (see git log) |
| `1e66c40` | (see git log) |
| `1962f9a` | (see git log) |
| `96dd78b` | (see git log) |
| `879d6fd` | (see git log) |
| `e289354` | (see git log) |
| `07a5517` | (see git log) |
| `f23f710` | (see git log) |
| `05d946c` | (see git log) |
| `f383c4e` | (see git log) |

### Testing

- [OK] (Add test results)

### Status

[OK] **Completed**

### Next Steps

- None - task complete


## Session 22: Complete Figma prototype task tree integration

**Date**: 2026-09-19
**Task**: Complete Figma prototype task tree integration
**Branch**: `main`

### Summary

Ran the existing Figma parent integration task after all four child tasks were already archived. Fixed incomplete tenant-date test fixtures, made the mobile dunning sheet opaque above the fixed navigation, verified all canonical desktop/mobile routes and forms with Chrome, captured integration screenshots, and passed go test ./..., go vet ./..., and the cleaned MySQL isolation test. Archived the parent task without touching unrelated run-truelayer-demo.sh or user files.

### Main Changes

(Add details)

### Git Commits

| Hash | Message |
|------|---------|
| `c155988` | (see git log) |
| `a5d50ce` | (see git log) |

### Testing

- [OK] (Add test results)

### Status

[OK] **Completed**

### Next Steps

- None - task complete


## Session 23: Rosewood test data seeding and audit seeder repair

**Date**: 2026-09-19
**Task**: Rosewood test data seeding and audit seeder repair
**Branch**: `main`

### Summary

Seeded the local rentops database from the Rosewood rent ledger (4 properties / 25 rooms / 58 tenants / 40 agreements / 63 parties / 186 transactions / 307 obligations), idempotent on re-run, so every page has data to click through. Established that the EUR 0.00 on /rent-dashboard is a product gap rather than a data gap: that route renders the room-centric workspace, whose amounts come only from rent_charges (rent_workspace.go:1118 requires rent_charge_id IS NOT NULL) and no production code writes rent_charges. Withdrew that acceptance criterion and moved it out of scope for a separate task. Repaired three staleness bugs in scripts/audit/seed.mjs, each hiding the next: B1 a regex requiring class immediately after tr, B2 the ignored demo sitting on the one transaction that can render the auto-match suggestion, and B3 four dashboard assertions parsing rent-row, markup that a DB-backed session can never render because renderRentDashboard selects its template by request path. The local audit seeder now completes. Recorded the findings in the desktop-contract task and in the backend spec.

### Main Changes

(Add details)

### Git Commits

| Hash | Message |
|------|---------|
| `98a692a` | (see git log) |
| `ec60726` | (see git log) |
| `15d49bb` | (see git log) |
| `abb74ac` | (see git log) |
| `8e9dc5c` | (see git log) |

### Testing

- [OK] (Add test results)

### Status

[OK] **Completed**

### Next Steps

- None - task complete


## Session 24: 房间视角金额：改读租客义务，放弃 charge 生产者

**Date**: 2026-09-19
**Task**: 房间视角金额：改读租客义务，放弃 charge 生产者
**Branch**: `main`

### Summary

推翻原方案：不补 rent_charge 生产者，改为房间视角直接汇总惰性义务，归属走 agreementParty → tenancyAgreement.RoomID。去掉 rent_charge_id IS NOT NULL 门控；load 先调 ensureMonthlyObligations。检查发现 design §3 的歧义房间 needs_review 标记未实现（单测因歧义租客是唯一租客而无区分度），已补并用混合房间单测钉死（关掉修复会失败）。实机 2026-09 房间树与 /bills 同为 EUR 24,985.00 / 6,510.00，307 条义务逐行未变。另确认 TestDunningDashboardHTTPWorkflowOnMySQL 为既有失败（pristine HEAD 复现）。

### Main Changes

(Add details)

### Git Commits

| Hash | Message |
|------|---------|
| `77739ff` | (see git log) |

### Testing

- [OK] (Add test results)

### Status

[OK] **Completed**

### Next Steps

- None - task complete


## Session 25: 桌面契约解冻与 1100 断点（子任务 ①）

**Date**: 2026-09-19
**Task**: 桌面契约解冻与 1100 断点（子任务 ①）
**Branch**: `main`

### Summary

完成 09-19-pc-ui-fidelity-alignment 的子任务 ①，并规划删除任务。① ：把「桌面渲染是冻结契约」改写为三条可审的桌面改动条件；新增 @media (max-width:1100px) 档（基准之后、640 之前）；640 档逐字节不变（sha256 证明）；980/981 保留并明确为导航阈值；断言为改指向而非削弱（逐条 diff + 注入反证）；44 张四档截图；desktop-widths.mjs 11 页×4 档 exit 0 无文档级横向溢出。诚实结论已入 spec：共享 1100 档只装一条声明且其唯一渲染者是死模板，档位存在≠档位填满。新增任务 09-19-legacy-dashboard-template-removal（父任务第 6 个子任务）并完成规划：查证 rentDashboardTemplate 是死代码但其宿主函数 renderRentDashboard 是 /bills、/dunning 的活渲染体（差点误删）；/rent-dashboard/settle 与 handleDashboardManualBalance 保留给 ③；催收抽屉只存在于待删模板，登记为原型功能缺口；停机门 1.1（催收链路鉴权）开工前已通过。

### Main Changes

(Add details)

### Git Commits

| Hash | Message |
|------|---------|
| `c944538` | (see git log) |
| `3951263` | (see git log) |

### Testing

- [OK] (Add test results)

### Status

[OK] **Completed**

### Next Steps

- None - task complete


## Session 26: 删除无库降级 dashboard 模板（死代码收敛）

**Date**: 2026-09-19
**Task**: 删除无库降级 dashboard 模板（死代码收敛）
**Branch**: `main`

### Summary

删掉 rentDashboardTemplate 及其唯一的 db==nil 入口，dashboard.go 569→189 行。/bills 与 /dunning 逐字节不变（独立复现）。计划阶段核实推翻了一半前提：renderRentDashboard 有 4 个调用点、3 个是活的，函数与 switch 必须留。复核推翻首轮「全绿」——修了 1 处恒真断言、1 处丢失的别名覆盖、1 个丢失的非法输入测试。两处原型功能缺口（催收抽屉、重试此人按钮）已登记交给 ③。

### Main Changes

(Add details)

### Git Commits

| Hash | Message |
|------|---------|
| `c3c6ecb` | (see git log) |
| `95ba6e7` | (see git log) |

### Testing

- [OK] (Add test results)

### Status

[OK] **Completed**

### Next Steps

- None - task complete


## Session 27: 09-20 移动端筛选开关与重复错误提示修复

**Date**: 2026-09-20
**Task**: 09-20 移动端筛选开关与重复错误提示修复
**Branch**: `main`

### Summary

修四个缺陷：≤640 筛选区打不开（共享外壳脚本早于页面正文）、同一错误码两条措辞、查询参数被原样渲染成绿色提示、/bills 函数中介形态的同类泄漏。复验阶段发现第四处并推翻三处计划期误记。

### Main Changes

## 背景

09-19 PC 端还原度对齐的复验阶段报出四个缺陷，本任务收口。四个都是「用户看到的东西不对」，
没有一个是崩溃或数据错误，所以全部靠实机渲染证据判定，而不是靠读代码。

## 四个缺陷与处置

| # | 现象 | 根因 | 处置 |
|---|---|---|---|
| 1 | ≤640 点「筛选」没反应 | 共享外壳 `<script>` 在 partial 顶部，早于它要绑定的页面正文；顶层查找 `.object-list-filter-toggle` 返回 null | 所有页面正文查找推迟到 `DOMContentLoaded`；同时删掉 ④ 为绕行加的 `onchange="this.form.submit()"`，只留外壳一条提交路径 |
| 2 | 一次失败看到两条不同措辞 | 同一错误码在两处渲染点各有一份文案 | 收敛成一个常量 `cashOverbalanceText` / `cashReceiptFailedText`，只留抽屉一处渲染点 |
| 3 | 查询参数被原样渲染成绿色成功提示 | `{{if .Message}}{{.Message}}{{end}}` 裸回显 | 改白名单 `{{if eq .Message "code"}}`，句子留在模板里 |
| 4 | `/bills?message=任意文本` 同样进绿 toast | 同 3，但形态是 `{{billsNotice .Message}}` —— 函数中介，字面量扫描抓不到 | `billsMessageText` 的 `default` 由 `return code` 改 `return ""`，守卫改 `{{if billsNotice .Message}}` |

缺陷 4 是复验阶段才发现的，也是本任务最有价值的一条：**只匹配字面量 `{{.Message}}` 的
扫描有结构性盲区**。这条已写进 `responsive-conventions.md` §8.1。

## 三处勘误（计划期记录有误，实机推翻）

1. `[data-mobile-search]` 不是同源受害者 —— 它的按钮在 partial 第 5 行，**早于**主脚本，
   所以那次查找本就命中。它是回归项，不是被同一个 bug 打中的第二个受害者。
2. 「页面级横幅被抽屉背板挡住所以用户只看得到一条」—— 实测按档位分：背板是
   `rgba(10,18,24,.22)` 半透明，桌面档横幅压暗但可读，**两条都看得见**；≤640 抽屉变
   底部 sheet 整个盖住，才只剩一条。用 `elementsFromPoint` 读绘制栈定的案，不是读 CSS 猜的。
3. `cash_receipt_failed` 不是「同类问题」，是**同一个缺陷本身**（修前三份措辞，
   与 `cash_overbalance` 逐处对应）。

## 验证

- 主会话独立重写的实机探针 `probe-0920-independent.mjs`：修前 19/29 → 修后 **33/33**。
- 红证（先证伪再证真）：完整退回 HEAD 版外壳 → 红；只把一条查找提到顶层 → 红；
  把旧措辞放回 → 红；把 `bankPageTemplate` 改回裸回显 → 红。
- `probe-integration-review.mjs` 11 页 × 4 档：0 溢出、0 报错；侧栏 800/900/980 档
  `position=static coversBody=false`；开合 `aria false -> true` 在 `/properties` 与 `/rooms` 都成立。
- 三笔修复提交各自的树 `go build` / `go vet` / `go test` 全绿（`git worktree` 逐笔验证）。

## 报了但没修（留给后续任务）

- **`?error=` 同样裸回显**：全仓库 7 处，`/bank`、`/bills` 实测可注入。未修的理由是
  本任务 AC 显式限定在绿色成功提示，而 `billsErrorText` 的原样回退由
  `list_pages_alignment_test.go:58` 钉住；顺手改会同时改契约并把爆炸面扩到 7 个页面。
  风险低于绿 toast（红条不会被误认成「系统确认」），但仍是「受信 UI 显示攻击者文本」。
- **若干页面「任意 `?message=` 都弹固定文案的成功 toast」**（`/properties`、`/rooms`、
  `/property-detail`、`/room-detail`）：文案由模板选定、不来自请求，不是注入，
  但构造链接能让用户看到一条他并未触发的「操作已完成」。
- **`/bank` 正向腿非端到端**：审计账号 `connected=false`，渲染不出同步表单，
  探针读的是重定向落点而非真的 POST。产出方已静态钉死（`main.go:1684` →
  `bankRefreshRedirect` → `/bills` 同款 URL），守卫本身已证明生效。
  残留风险：没有测试把产出方的码字符串与模板的 `{{if eq .Message "refreshed"}}` 绑在一起，
  将来改了产出方的码会静默失去匹配（toast 消失、测试全绿）。


### Git Commits

| Hash | Message |
|------|---------|
| `cb15952` | (see git log) |
| `0ccf7bf` | (see git log) |
| `ce1dfeb` | (see git log) |
| `33758ef` | (see git log) |
| `ef3924d` | (see git log) |

### Testing

- [OK] (Add test results)

### Status

[OK] **Completed**

### Next Steps

- None - task complete


## Session 28: 工作台交互与控件改造

**Date**: 2026-09-20
**Task**: 工作台交互与控件改造
**Branch**: `fix/workspace-quick-fixes`

### Summary

完成工作台筛选、分页、日历与下拉控件统一；补充租客搜索和详情编辑抽屉、房产支出抽屉、流水匹配及首页人工处理队列。构建与 diff 检查通过，未运行测试。

### Main Changes

(Add details)

### Git Commits

| Hash | Message |
|------|---------|
| `24d218f` | (see git log) |

### Testing

- [OK] (Add test results)

### Status

[OK] **Completed**

### Next Steps

- None - task complete


## Session 29: 精简流水与主页的状态控件与用词

**Date**: 2026-09-21
**Task**: 精简流水与主页的状态控件与用词
**Branch**: `main`

### Summary

流水页删掉重复的状态筛选下拉（快捷筛选那份从来没有「已忽略」，漂移成筛不出已忽略的流水），页签「已匹配」改「已关联」，下拉标签「匹配状态」改「关联状态」。主页状态下拉三处文案对齐行内徽章（逾期→已逾期、已交满→已缴清、未到期未缴→未缴），成功提示「流水已匹配」改「流水已关联」。动词「匹配」不动。测试：原断言要求「两个下拉都存在」，正是它默许了漂移，改写为「不再有重复控件」；新增两条防回潮断言，期望文案取自 workspaceStatusLabel 而非抄字面量，并各做反向对照验证。新增 .trellis/spec/frontend/status-vocabulary.md 记录「一个存储值一个词一个控件」与已知残留。验证：失败 9 条与 git stash 后基线逐条相同，0 回归。纠正：先前报的「基线 3 条」是错的，真值 9 条——go test 接管道既截断输出又让退出码恒为 0。未做视觉走查。两处发现未改并记入 Out of scope：transactionMatchStatusLabel 第三套词、transaction_actions.go 把状态散文写进数据库审计留痕。

### Main Changes

(Add details)

### Git Commits

| Hash | Message |
|------|---------|
| `6a9b9a9` | (see git log) |

### Testing

- [OK] (Add test results)

### Status

[OK] **Completed**

### Next Steps

- None - task complete


## Session 30: 房间入住与租金计划交付

**Date**: 2026-09-22
**Task**: 房间入住与租金计划交付
**Branch**: `main`

### Summary

完成房间收租计划切换：迁移至 room rent plan、计划唯一写入口、租客月度唯一房间约束、真实 E2E 与移动端浏览器验收。

### Main Changes

(Add details)

### Git Commits

| Hash | Message |
|------|---------|
| `80d34f0` | (see git log) |

### Testing

- [OK] (Add test results)

### Status

[OK] **Completed**

### Next Steps

- None - task complete


## Session 31: 对象删除、匹配日历与租客搜索修复

**Date**: 2026-09-22
**Task**: 对象删除、匹配日历与租客搜索修复
**Branch**: `main`

### Summary

完成安全删除、匹配月份日历和租客模糊搜索修复；全量 Go、vet 与 MySQL 保护回归通过。

### Main Changes

(Add details)

### Git Commits

| Hash | Message |
|------|---------|
| `e465b9a` | (see git log) |
| `3692312` | (see git log) |

### Testing

- [OK] (Add test results)

### Status

[OK] **Completed**

### Next Steps

- None - task complete


## Session 32: Tenant room rent setup

**Date**: 2026-09-22
**Task**: Tenant room rent setup
**Branch**: `main`

### Summary

Added vacant room rent plans, atomic room and tenant composite writes, tenant room/rent assignment UI, detail buttons, and code-spec coverage. Full Go tests and vet passed; browser bridge was unavailable for interactive verification.

### Main Changes

(Add details)

### Git Commits

| Hash | Message |
|------|---------|
| `b7f2d41` | (see git log) |
| `0ebad35` | (see git log) |
| `6e3f510` | (see git log) |
| `be1dfc7` | (see git log) |
| `5cde6c5` | (see git log) |
| `7320729` | (see git log) |

### Testing

- [OK] (Add test results)

### Status

[OK] **Completed**

### Next Steps

- None - task complete


## Session 33: 完成桌面列表排序与收款识别

**Date**: 2026-09-23
**Task**: 完成桌面列表排序与收款识别
**Branch**: `main`

### Summary

完成桌面列表表头排序、筛选上下文保留、收款人身份识别与收款处理改进；测试与规范已更新。

### Main Changes

(Add details)

### Git Commits

| Hash | Message |
|------|---------|
| `32bd4bc` | (see git log) |

### Testing

- [OK] (Add test results)

### Status

[OK] **Completed**

### Next Steps

- None - task complete


## Session 34: 流水匹配核对抽屉

**Date**: 2026-09-23
**Task**: 流水匹配核对抽屉
**Branch**: `main`

### Summary

实现宽版流水核对抽屉、已缴月份凭据与付款人历史分页；修复租客搜索和列表入账用途文案，完成 Go 与浏览器验证。

### Main Changes

(Add details)

### Git Commits

| Hash | Message |
|------|---------|
| `890761f` | (see git log) |

### Testing

- [OK] (Add test results)

### Status

[OK] **Completed**

### Next Steps

- None - task complete


## Session 35: 流水租金月份来源优先级

**Date**: 2026-09-23
**Task**: 流水租金月份来源优先级
**Branch**: `main`

### Summary

description 作为唯一明确租金月份来源；无月份时仅展示转账月份待确认建议。旧 reference-only 流水不会自动匹配。全量 Go 测试、go vet 和差异检查通过；本地 MySQL 不可连接。

### Main Changes

(Add details)

### Git Commits

| Hash | Message |
|------|---------|
| `8526a9d` | (see git log) |

### Testing

- [OK] (Add test results)

### Status

[OK] **Completed**

### Next Steps

- None - task complete


## Session 36: Bank rent-month evidence and date suggestion

**Date**: 2026-09-24
**Task**: Bank rent-month evidence and date suggestion
**Branch**: `main`

### Summary

Audited 179 income descriptions; ignored bank TxnDate and non-rent months for explicit matching; added Dublin posting-date suggestions with configurable day-15 cutoff and regression tests.

### Main Changes

(Add details)

### Git Commits

| Hash | Message |
|------|---------|
| `6b1c0a2` | (see git log) |

### Testing

- [OK] (Add test results)

### Status

[OK] **Completed**

### Next Steps

- None - task complete


## Session 37: 多租客与跨月流水匹配

**Date**: 2026-09-24
**Task**: 多租客与跨月流水匹配
**Branch**: `main`

### Summary

实现单笔流水多租客与同租客跨月批量匹配；保留租客月份证据和同付款人历史；Go、MySQL 和浏览器验证通过。

### Main Changes

(Add details)

### Git Commits

| Hash | Message |
|------|---------|
| `a3e7196` | (see git log) |
| `9419f60` | (see git log) |

### Testing

- [OK] (Add test results)

### Status

[OK] **Completed**

### Next Steps

- None - task complete


## Session 38: 流水日期自动匹配与租客建议

**Date**: 2026-09-24
**Task**: 流水日期自动匹配与租客建议
**Branch**: `main`

### Summary

实现明确月份优先、5日及25日起按日期自动匹配、可信付款人及精确余额约束；列表详情标记自动/手动来源，人工抽屉展示姓名近似租客建议。全套 Go 测试与 go vet 通过；隔离 MySQL 测试 DSN 未配置。

### Main Changes

(Add details)

### Git Commits

| Hash | Message |
|------|---------|
| `680635e` | (see git log) |

### Testing

- [OK] (Add test results)

### Status

[OK] **Completed**

### Next Steps

- None - task complete


## Session 39: 月底预付匹配补充

**Date**: 2026-09-24
**Task**: 月底预付匹配补充
**Branch**: `main`

### Summary

补齐25日后指向未来月份、但账单尚未生成的情况：先核对唯一有效租金计划、租客、金额和币种，再生成下月责任并重新严格匹配；新增边界测试。完整 Go 测试和 go vet 通过。

### Main Changes

(Add details)

### Git Commits

| Hash | Message |
|------|---------|
| `c89b388` | (see git log) |

### Testing

- [OK] (Add test results)

### Status

[OK] **Completed**

### Next Steps

- None - task complete


## Session 40: Room tenant selection and assignment

**Date**: 2026-09-24
**Task**: Room tenant selection and assignment
**Branch**: `main`

### Summary

Added existing or new tenant choices during room creation, disabled conflicting tenants until unbound, enabled new tenants from room occupancy, and verified isolated staged code with Go tests.

### Main Changes

(Add details)

### Git Commits

| Hash | Message |
|------|---------|
| `c99c379` | (see git log) |

### Testing

- [OK] (Add test results)

### Status

[OK] **Completed**

### Next Steps

- None - task complete
