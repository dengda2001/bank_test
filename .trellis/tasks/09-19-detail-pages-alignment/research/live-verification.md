# 详情页对齐 — 实机验证记录

对象：`09-19-detail-pages-alignment`（子任务 ⑤）。
实例：`MYSQL_PORT=3306 MYSQL_HOST=127.0.0.1 APP_PORT=18092 scripts/run-audit-local.sh`
（一次性库 `rentops_audit_<timestamp>`，非生产；生产 `:8081` / `bank.ddpl.top` 全程未触及。）

## 复现步骤

```bash
# 1. 起一次性实例（后台）
AUDIT_KEEP=1 MYSQL_PORT=3306 MYSQL_HOST=127.0.0.1 APP_PORT=18092 \
  bash scripts/run-audit-local.sh            # 输出 Base URL / Account / Password

# 2. 四页 × 四档截图 + scrollWidth 断言 + DOM 证据
AUDIT_BASE=http://127.0.0.1:18092 AUDIT_USER='<account>' AUDIT_PASS='<password>' \
  AUDIT_OUT=.trellis/tasks/09-19-detail-pages-alignment/research/screenshots \
  node scripts/audit/detail-pages.mjs

# 3. 复制按钮 / 页头三操作的交互级验证（会写库，仅对一次性实例）
AUDIT_BASE=http://127.0.0.1:18092 AUDIT_USER='<account>' AUDIT_PASS='<password>' \
  node scripts/audit/detail-pages-actions.mjs
```

## AC 1 — 区块 / 指标卡数量 / 侧栏结构

脚本按「实际渲染（`getBoundingClientRect` 非零）」统计，忽略窄屏专用变体：

| 页面 | 可见指标卡 | 主区区块 | 侧栏区块 |
|---|---|---|---|
| property-detail | 本月应收 / 已收租金 / 本月支出 / 经营净额（4） | 房间收款概览 / 本月收支构成 / 关联支出 | 房产资料 / 需要关注 |
| room-detail | 本月房间应收 / 已覆盖 / 未付 / 付款记录（4） | 租客责任 / 关联收款 / 代付分配详情 | 房间与租约 / 收租时间线 |
| tenant-detail | 本月个人责任 / 个人责任已覆盖 / 本月未收 / 本月状态（4） | 缴费历史 / 代付与被代付 / 责任与代付 | 租客档案 / 付款识别 |
| transaction-detail | 流水金额 / 处理状态 / 已分配金额 / 未分配余额（4） | 匹配建议 / 关联账单与对象 / 处理记录 | 原始流水 / 原始描述 / 查看银行原始记录 |

原型（`figma/rentops-desktop-suite.html` `buildDetails()`）四页各 4 卡：
房产（本月应收 / 已收租金 / 本月支出 / 经营净额）、房间（本月房间应收 / 已覆盖 / 未付 / 房间净额）、
租客（本月个人责任 / 已覆盖 / 仍需支付 / 近 12 月准时率）、流水（入账金额 / 建议置信度 / 已分配 / 未分配余额）。
**卡片数量四页都是 4 对 4。**被脚本列为 hidden 的 `未收`（房产）与重复的 `房间与租约`（房间）
分别是 ≤640 专用 / 桌面专用的变体，桌面档不渲染（`property-detail.css:84`）。

差异（保留，未删）：

- 房产主区多「关联支出」、租客主区多「责任与代付」、流水侧栏多「查看银行原始记录」折叠块 —— 均为既有功能块，prd/design 未授权删除。
- 指标卡**文案**与原型不同名（如 入账金额→流水金额、建议置信度→处理状态、房间净额→付款记录、仍需支付→本月未收）。
  AC1 只约束数量与强调，未要求逐字文案；未作改动。

## AC 11 — 四页 × 四档无横向溢出

`scripts/audit/detail-pages.mjs` 输出（`report.json` 同目录）：

```
===== 1024 =====
ok   property-detail      http=200 client=1024 scroll=1024 overflow=0
ok   room-detail          http=200 client=1024 scroll=1024 overflow=0
ok   tenant-detail        http=200 client=1024 scroll=1024 overflow=0
ok   transaction-detail   http=200 client=1024 scroll=1024 overflow=0
===== 1366 / 1440 / 1920 =====   （同上，client == scroll，overflow=0）
===== 390 (phone) =====
ok   room-detail          http=200 client=390 scroll=390 overflow=0
        desktopFacts=false mobileFacts=true
no document-level horizontal overflow on any audited detail page
```

截图（19 张）见同目录 `screenshots/`：`{property,room,tenant,transaction}-detail-{1024,1366,1440,1920}.png`、
`room-detail-390.png`、`room-detail-fully-allocated-1440.png`。

## AC 2 — room-detail 移动端字段

DOM 证据（1440）：`desktopFactsVisible=true, mobileFactsVisible=false`。
DOM 证据（390）：`desktopFactsVisible=false, mobileFactsVisible=true`。
即 PC 档只出桌面事实区，≤640 才替换成移动事实区。

## AC 3 — 未分配 cell 两种状态

| 房间 | 条件 | 已覆盖 | 未分配 | 未覆盖责任 |
|---|---|---|---|---|
| room 5 | 一笔 120000 收款只分配 30000 | EUR 700.00 | **EUR 1450.00** | EUR 1750.00 |
| room 6 | 无收款/已全额分配 | EUR 0.00 | **EUR 0.00** | EUR 1360.00 |

两个数不相等，说明「未分配」（收到的钱没人认领）与「未覆盖责任」（责任没人付）确实是两个量。

## AC 4 / 5 / 6 —租客详情

`select[name=range]`：`value="12"`，选项 `["近 12 个月", "近 24 个月"]`。
`付款识别`：`RENT-2026-09-LINA`，`data-copy-value="RENT-2026-09-LINA"`。
交互验证（`detail-pages-actions.mjs`）：

```json
{ "copyButtonValue": "RENT-2026-09-LINA",
  "clipboard": "RENT-2026-09-LINA",
  "buttonFlash": "已复制",
  "referenceMatchesClipboard": true,
  "buttonRestored": "复制" }
```

`代付与被代付` 表头 `["责任月份","实际付款人","责任所有者","分配金额","结果"]`，
真实数据行 `["2026年8月","志强 代付","Lina","EUR 950.00","本人被代付"]`
（来自 `PaidByOther` 查询，非占位行；数据由 `payment_allocations.tenant_id=3` 与
`payment_transactions.matched_tenant_id=4` 不一致构造）。

## AC 7 / 8 / 9 — 流水详情

`原始描述`：`descriptionText` 保留换行（`FASTER PAYMENTS RECEIPT REF:...\nFROM: LI NA\nREMITTANCE: ...`），
`descriptionWhiteSpace="pre-wrap"`。
事实区标签：`["银行姓名","摘要 / 参考","交易时间","入账账户","数据来源","外部流水号","付款人编号","账户标识","识别租金月份"]`，
`交易时间 = "2026-09-03 09:15"`。

页头三入口与同名列表行动作逐字段比对：

```json
"headerEntryLabels": ["编辑分配", "标记非租金", "确认匹配"],
"parity": { "1": true, "5": true }
```

`parity[tx]` = 详情页头每个表单都能在**同一笔流水**的列表行里找到 action 与字段集合完全相同的表单。
从详情页头提交 `标记非租金` 后：

```
urlAfterIgnore = http://127.0.0.1:18092/transactions?match_status=unmatched&message=transaction_action_saved
landedOnList = true
```

即 `return_to` 链路成立，动作落到它来的那个列表。

## AC 12 — 工程校验

```
go test ./cmd/truelayer-demo/...   → ok  bank/cmd/truelayer-demo  1.065s
go vet ./...                       → （无输出）
git diff --check                   → （无输出）
gofmt -l cmd/truelayer-demo/       → （无输出）
```

## 反向测试（断言非空证明）

逐条把实现改坏，确认断言失败后再还原：

| 断言 | 破坏方式 | 结果 |
|---|---|---|
| `TestRoomDetailMobileFactsOwnTheirWrapper` | 把 `room-mobile-facts` 重新焊到桌面 `<dl>` 上 | FAIL: `mobile-only facts are still welded onto the desktop .room-facts element` |
| `TestRoomDetailUnallocatedCellRendersBothStates` | 删掉 `未分配` 单元格 | FAIL: `has no 未分配 cell for value "€120.00"` |
| `TestPropertyDetailRendersFinancialBridge` | 删掉「本月收支构成」整块 | FAIL: `property detail 本月收支构成 missing "本月收支构成"` |
| `TestTransactionDetailTemplateRendersPrototypeSectionsAndEscapesSourceData` | 删掉事实区「交易时间」行 | FAIL: `missing escaped marker "交易时间"` |
| `TestTransactionDetailHeaderActionsReuseListRowEndpoints` | 去掉页头表单的 `return_to` | FAIL: `matched header missing "name=\"return_to\" value=\"/transactions?page=2\""` |
| `TestTenantDetailTemplateShowsNameOnlyPayerAndHistoryControls` | 把范围控件退回 `from_month` | FAIL: `still renders retired markup "from_month"` |
| `TestTenantDetailTemplateRendersPaidByOtherRows` | 清空代付表表体 | FAIL: `paid-by-other table missing "责任月份"` |
| `TestTenantHistoryRangePresetWinsOverExplicitMonths` | 删掉 `range` 预设分支 | FAIL: `range=2026-09..2026-09 page=2 size=12 want 2024-10..2026-09 page 2 size 12` |

## 未覆盖 / 未验证

- `roomUnallocatedCents` 与 `listTenantPaidByOtherRows` 只用**实机数据**验证，未写 MySQL 直连单测；
  这两个查询的边界（多币种、`record_status=voided`、`matched_tenant_id IS NULL`）由代码读法判断，未逐条造数。
- `design.md §2.5` 的原始观察样本已被证明是夹具产物，见 `room-detail-payment-source.md`；
  该结论**未**在旧截图对应的分支上直接重放（无法在不改动当前工作树的前提下切分支），
  而是通过重放生成该截图的测试夹具代码得出。
