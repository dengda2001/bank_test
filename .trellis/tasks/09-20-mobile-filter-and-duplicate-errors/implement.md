# 执行计划：移动端筛选开关与重复错误提示修复

需求与验收见 `prd.md`。本文件只记执行顺序、验证命令与风险点。

## 开工前置（已完成）

② `e47702e`、③ `e2e88fb`、④ `f538a6f`、⑤ `c3c992c` 均已提交，
`09-19-pc-ui-fidelity-alignment` 父子任务树已归档 —— 本任务要改的三个文件
（`workspace-nav.html`、`cash-receipts.html`、`page_data_routes.go`）不再有在途改动。

## 执行顺序

三个缺陷互相独立，按**爆炸半径从小到大**排，每步做完单独验证再进下一步：

### 第 1 步：缺陷 2（最小，先做）

同一错误码 `cash_overbalance` 的三份文案收敛成一份、一个渲染点。
`cash-receipts.html:24` 与 `:51` 在同一次渲染里都会命中，是用户直接看到的那对；
`cash_receipt_handlers.go:216` 的内联模板是另一条渲染路径。
保留哪一条措辞由实施者定，但必须全仓库唯一。

验证：`grep -rn 'cash_overbalance' cmd/truelayer-demo`，用户可见文案只允许一处；
其余命中只能是错误码字面量或测试断言。
实机：`/cash-receipts?error=cash_overbalance` 页面渲染出的该消息**恰好一条**。

### 第 2 步：缺陷 3（中等）

三处裸回显改成白名单，**保留 `data-toast`**（去掉它等于退掉 ② 的对齐成果）：

| 位置 | 处置 |
|---|---|
| `page_data_routes.go:1524` `bankPageTemplate` | `{{if eq .Message "refreshed"}}`，文案与 `billing_page.go:264` 对同一码的既有措辞一致（「银行数据已刷新。」） |
| `page_data_routes.go:1511` `legacyCashReceiptPageTemplate` | 死模板，同样守卫即可；**本任务不删它** |
| `web/templates/pages/rent-workspace.html:21` | 直接删掉该行，并清掉 `rent_workspace_page.go:82` 的 `message` 形参及其 `:118`、`:146` 的传递 |

`rent-workspace.html` 那处无正规产出方（全仓库没有任何 redirect 往 `/rent-workspace`
带 `message=`），删参数是为了不留一个「永远传空」的死参数。
**注意 `pageError` 参数仍在使用，不要误删。**

同时按 `prd.md` 的 AC 加一条 Go 测试：扫描全部模板与 Go 模板字面量，
禁止任何 `data-toast` 附近的裸 `{{.Message}}`。
**该测试在修复前必然失败，必须与修复同批提交**（否则等于把一条会红的测试留在仓库里）。

验证：`grep -rn 'data-toast>{{\.Message}}' cmd/truelayer-demo/` 无输出；
实机构造 `/bank?message=注入测试文本` 不出现绿色成功提示，
走正规路径（POST `/bank/sync` 后重定向）仍能看到「银行数据已刷新。」。

### 第 3 步：缺陷 1（最大，最后做）

`workspace-nav.html` 的 `<script>` 里所有在顶层查询**页面正文**元素的语句，
一律推迟到 `DOMContentLoaded` 之后。至少覆盖三类：
`[data-mobile-search]`、`.object-list-filter-toggle`、`.object-list-filter-fields`。
**不要只修被报告的那一个。**

同文件里已有一个正解可参照：`09-19-shell-alignment` 新写的 toast 脚本
把查找推迟到 `DOMContentLoaded`，并在注释里写明了这个陷阱
（`workspace-nav.html:57-59`、`:80`）。照它写，不要另创一套。

**筛选提交只允许留一条路径。** ④ 为绕行加的 `onchange="this.form.submit()"`
（`properties.html`、`rooms.html`）要么被外壳的通用机制取代、要么被明确保留并写明理由 ——
不允许「修好了但没人知道哪套在起作用」。做完必须在 `implement.md` 或代码注释里说清楚。

验证：实机 390×844 下 `/properties`、`/rooms` 点「筛选」后 `aria-expanded`
由 `false` 变 `true`、`.object-list-filter-fields` 计算样式由 `display:none` 变 `display:grid`，
再点一次收起；≥641 档按钮行为不变；`/tenancies`（无开关，用 `type="submit"`）
在两档下都仍能正常提交。

## 验证命令

```bash
go test ./cmd/truelayer-demo/...
go vet ./...
git diff --check

# 全仓库唯一性复核
grep -rn 'cash_overbalance' cmd/truelayer-demo
grep -rn 'data-toast>{{\.Message}}' cmd/truelayer-demo/
```

实机验证一律走 `scripts/run-audit-local.sh` 起的临时实例 + `scripts/audit/launch.mjs`：

```bash
MYSQL_PORT=3306 APP_PORT=18098 scripts/run-audit-local.sh
```

**绝不要指向 `:8081` / `bank.ddpl.top`** —— 那是生产应用连生产库。

## 风险点与回滚

| 风险 | 说明 |
|---|---|
| `workspace-nav.html` 是**全部 11 个页面**的共享外壳 | 爆炸半径最大的一处改动，独立提交、可单独 revert；改后必须逐页回归，不能只看 `/properties` |
| `rent_workspace_page.go` 的参数清理 | 删 `message` 时易误删仍在用的 `pageError`；改完用 `go build` 与总览页实机双向确认 |
| 死模板 `legacyCashReceiptPageTemplate` | 本任务只守卫、不删除；删遗留模板另有专门任务 |

## 提交切分

三个缺陷各自独立提交，理由是两个「全仓库唯一性」断言（缺陷 2 的文案、缺陷 3 的裸回显）
需要能单独 revert；缺陷 1 的外壳改动单独一笔，便于出问题时定点回滚。

## 实施记录

### 缺陷 2：保留哪一条措辞、哪一处渲染点

- 措辞取 `cash-receipts.html:24` 的那条（信息更全：说出了原因与下一步动作）：
  「这笔现金会超过该月份未收余额，请核对已有收款。」
- 渲染点取抽屉里的那处（原 `:51`）。理由：`loadCashReceiptListPage` 的
  `ShowForm = add == "1" || error != ""`，所以只要带 `error=cash_overbalance`
  抽屉必然打开，页面级横幅（原 `:24`）则被 `position: fixed` 的抽屉背板压在下面，
  ≤640px 的底部 sheet 更是整个盖住它。删掉页面级那处后，一次渲染只剩一条消息。
- **两份文案收敛成一个常量** `cashOverbalanceText`（`cash_receipt_handlers.go`），
  两个模板都通过 `cashOverbalanceNotice` 模板函数渲染它。这是必要的：
  `cash_receipt_handlers.go` 里的内联 `cashReceiptTemplate` 不是死模板 ——
  `main.go:501` 把 `/cash-receipts/new` 路由到 `handleCashReceiptNew`，
  而 `cashReceiptFormErrorURL` 在表单没带 `return_to=cash-receipts` 时正好把
  `cash_overbalance` 送到这个 URL。两条路径**并列可达**（不同路由，不会同时渲染），
  所以两边都要能显示该错误，而全仓库只保留一份文案。若直接删掉内联模板那一处，
  `/cash-receipts/new?error=cash_overbalance` 就会静默不给任何提示。

### 缺陷 1：筛选提交保留哪一条路径

**保留外壳的通用绑定**（`workspace-nav.html` 的
`.object-list-filter-fields select/input` → `change` → `requestSubmit()`），
**删掉 ④ 加的 `onchange="this.form.submit()"`**（`properties.html` 2 处、
`rooms.html` 3 处）。理由：外壳那条是通用机制，pages 不必各自记住；
两套并存时一次 `change` 会提交两次。实机已用请求计数确认单次提交
（`/properties`、`/rooms` 各 `requests=1`）。该约定写在 `workspace-nav.html`
的注释里（"The single submit path for the object-list filter fields"），
后续页面不得再自加 `onchange`。

### 缺陷 3：`rent_workspace_page.go` 的清理

删掉 `rentWorkspacePageFromData` 的 `message` 形参、`:146` 的实参、
`:118` 的赋值，并一并删掉 `rentWorkspacePageData.Message` 字段
（模板那行删掉后它再无读取方，留着就是「永远传空的死字段」）。
`pageError` / `Error` 原样保留。

### 缺陷 2 补做：`cash_receipt_failed` 是同一个缺陷，不是「同类」

复核时把范围外的判断推翻了：该码修前有**三份**措辞、`cash-receipts.html:23` 与 `:50`
在同一次渲染里都命中 —— 与 `cash_overbalance` 修前逐处对应，所以它属于本缺陷本身
（任务不变量是「同一错误码不给两条不同措辞」）。按已有机制收敛：

- 措辞取「现金补录失败，请检查租客、月份、金额和日期。」。内联那份写的是「币种」，
  但 `parseCashReceiptForm` 校验的四个可编辑字段是 tenant_id / period / amount /
  received_at，`currency` 虽也校验，可两个表单里该输入都是 `readonly`（只有 EUR），
  用户无从填错，指引他去检查一个不可改的字段是死路 —— 故取「日期」。
- 渲染点与 `cash_overbalance` 同策略（抽屉那处，页面级那处删掉）。删前先确认了
  前置条件：`ShowForm` 与 `Error` 出自同一个 query，`error != ""` 本身就打开抽屉，
  所以删掉页面级那处不会静默无提示。该前置条件已抽成纯函数
  `cashReceiptDrawerIsOpen(url.Values)` 并用 `TestCashReceiptDrawerIsOpenWheneverAnErrorCodeIsSet`
  钉住（落地在 `/cash-receipts` 列表页的两个 redirect 产出方都自带 `add`：
  `cash_receipt_handlers.go:304` 与 `cashReceiptFormErrorURL` 里硬编码的 `add=true`，
  但守卫不依赖这一点）。

### 实机验证脚本

- `scripts/audit/probe-mobile-filter-toggle.mjs`（缺陷 1）
- `scripts/audit/probe-notice-convergence.mjs`（缺陷 2、3；两个错误码 × 两条渲染路径）
- `scripts/audit/probe-0920-independent.mjs`（主会话独立重写，33 条，不共用实施者的假设）
- `scripts/audit/probe-0920-checker-gap.mjs`、`probe-0920-checker-error-echo.mjs`（复验阶段）

## 复验记录（trellis-check）

复验者逐条证伪，四条主要声明全部成立（含两次红证：完整退回 HEAD 版外壳 → 红；
只把 `.object-list-filter-toggle` 一条提到顶层 → 红；把旧措辞放回 → 红；
把 `bankPageTemplate` 改回裸回显 → 红）。同时改了三处、报了两处未修。

### 1. `/bills` 是缺陷 3 的第四处（已修，收进本任务）

字面量形态的扫描**看不见函数中介的回显**：

```
rent_collection_pages.go:84   {{if .Message}}…{{billsNotice .Message}}…
collection_settle_form.go:151 billsMessageText(…) default: return code
```

所以 `/bills?message=任意文本` 会在绿色 toast 里打印任意文本 —— 与缺陷 3 同一机制、
同一可达面。修法：`default` 返回 `""`，守卫改 `{{if billsNotice .Message}}`，
新增 `TestBillsMessageRendersOnlyWhitelistedCodes`（渲染行为断言）。

**`billsErrorText` 的红条原样回退没动**：它由 `list_pages_alignment_test.go:58`
钉住，且红条不是构造链接能冒充的「系统确认」。主会话已独立复核：
`billsMessageText` 全仓库只有 `billsNotice` 一个用途，三个合法码的产出方恰好三处
（`dashboard_manual_balance.go:162/164`、`rent_collection_pages.go:41`），
合法路径零损失；并用 `git show HEAD:…` 确认旧版确是 `default: return code`
＋ `{{if .Message}}`，即该泄漏在 HEAD 上必然存在。

**这条扫描盲区已写进 `responsive-conventions.md` §8.1**，连同「消息腿与错误腿
可以合法地采用不同回退策略」—— 否则下一个人会照着 grep 去「统一」它们。

### 2. `cash_receipt_pages.go` 未通过 gofmt（已修）

`template.FuncMap{` 的键值对齐少两格。`go vet` 与 `go test` 都不报，属静默欠格式化。

### 3. 主会话实测修正了一处因果描述（计划阶段记录有误）

计划阶段与代码注释都写「页面级提示被抽屉背板挡住，所以用户只看得到一条」。
实机实测（1440×900 与 390×844，`elementsFromPoint` 读绘制栈）**按档位分才对**：
背板是 `rgba(10,18,24,.22)` 半透明遮罩，桌面档横幅被压暗但文字完全可读 ——
用户确实同时看到两条；≤640 抽屉变底部 sheet 后整个盖住横幅，才只剩一条。
两处描述（PRD 缺陷 2 段、`cashReceiptDrawerIsOpen` 注释）已按实测改写。
**这不改变修复的正确性**：缺陷是「同一错误码一次渲染出两份措辞」，两种档位下都成立。

### 4. 报而未修，留给后续任务

- **`?error=` 也裸回显**（`/bank`、`/bills` 实测可注入，全仓库 7 处裸 `{{.Error}}`）。
  未修的理由：本任务缺陷 3 与 AC 都显式限定在「绿色成功提示 / `data-toast`」，
  而 `billsErrorText` 的原样回退是 `list_pages_alignment_test.go:58` 钉住的既有契约，
  顺手改会同时改掉契约并把爆炸面扩到 7 个页面。风险低于绿 toast（红条不会被误认成
  「系统确认」），但仍是「受信 UI 显示攻击者文本」，应单独立项。
- **若干页面「任意 `?message=` 都弹一条固定文案的成功 toast」**
  （`/properties`、`/rooms`、`/property-detail`、`/room-detail`）：文案由模板选定、
  不来自请求，所以不是注入，但构造链接能让用户看到一条他并未触发的「操作已完成」。
  属 §8.1 边缘，未修。
- **`/bank` 正向腿非端到端**：探针读的是 `GET /bank?message=refreshed`，
  不是真的 POST `/bank/sync`（审计账号 `connected=false`，渲染不出同步表单）。
  复验者判定这**足以**证明守卫本身生效，理由是产出方已静态钉死 ——
  `main.go:1684` → `bankRefreshRedirect` → `/bank?message=refreshed`，
  与探针访问的 URL 逐字相同；没被走到的只剩「POST 是否到达那一行」，与守卫无关。
  **残留**：没有测试把「产出方的码字符串」与「模板的 `{{if eq .Message "refreshed"}}`」
  绑在一起，将来改了产出方的码会静默失去匹配（toast 消失、测试全绿）。
