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
