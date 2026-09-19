# 移动端筛选开关与重复错误提示修复

独立任务，不属于 `09-19-pc-ui-fidelity-alignment` 父子任务树。

## 执行次序

**排在最后。** 用户 2026-09-20 决定：等 `09-19-pc-ui-fidelity-alignment` 的
② ③ ④ ⑤ 全部落地之后再开工，**不与它们并行**。

原因：缺陷 1 要改的 `web/templates/partials/workspace-nav.html` 正是 ② 的产物，
缺陷 2 要改的 `web/templates/pages/cash-receipts.html` 在 ②④ 的写入面内，
缺陷 3 要改的 `page_data_routes.go` 在 ④ 的写入面内。
现在插进去会与在途改动抢同一批文件。

开工前先确认 ② ③ ④ ⑤ 均已提交，工作区没有它们的在途改动。

进度（2026-09-20 更新）：② 已提交 `e47702e`，③ 已提交 `e2e88fb`，
⑤ 已提交 `c3c992c`；**只剩 ④ 在途**。④ 提交后即可开工。

## Goal

修掉三个既有缺陷。三者都不是本轮原型对齐**改出来**的，都在 `HEAD` 里就已经坏了；
由 `09-19-shell-alignment` 的实施者与复验者在做工与复验时发现并如实上报，
经主会话用 `git grep HEAD` / `git show 8c62c3f^:…` 独立复核确认，遂单独立项。

## Confirmed Facts

两条都用 `git grep <符号> HEAD -- cmd/truelayer-demo` 在**改动前的提交**上复核过，
确认不是本轮引入。

### 缺陷 1：≤640px 时 `/properties`、`/rooms` 的筛选区打不开

> **2026-09-20 勘误（主会话实机复核）**：原文写「三个列表页」并把 `/tenancies` 列入，
> **是错的**。`/tenancies` 用的是普通的 `<button class="btn subtle" type="submit">筛选</button>`
> （`tenancies.html:28`），没有开关按钮、也没有 `.object-list-filter-fields` 包裹层，
> 它不受本缺陷影响。涉及页面只有 **`/properties`、`/rooms`** 两个。
> 实机证据：390×844 下点击开关后 `aria-expanded` 仍为 `false`、
> `.object-list-filter-fields` 的计算样式仍为 `display:none`。

- 涉及页面：`/properties`、`/rooms`。
- 按钮标记（`HEAD` 即存在）：`properties.html:48`、`rooms.html:54`、
  `object-lists.css:19/68`（移动档 `display: block`）。
- 失效链完整形态：≤640 档 `.object-list-filter-fields { display: none }`（`object-lists.css:64`），
  只有 `.object-list-filters.filters-open` 才把它变回 `display: grid`（`:67`），
  而加 `filters-open` 类、翻 `aria-expanded` 的正是下面这段死监听。
  **所以本缺陷的后果不是「按钮没反应」，是筛选功能在 ≤640 档完全够不着。**
- 绑定逻辑在 `web/templates/partials/workspace-nav.html`。该 partial 渲染在
  `<div class="app">` 内、`<main class="content">` **之前**，所以它里面的
  `<script>` 在解析到该点时立即执行，此刻 `<main>` 及其内部的
  `.object-list-filter-toggle` 还不存在，`querySelectorAll` 取到空集合，
  监听器一个都没绑上。
- `HEAD` 版第 52 行就是裸的顶层语句，没有 `DOMContentLoaded` 包裹：

  ```js
  document.querySelectorAll(".object-list-filter-toggle").forEach((button) => {
    button.addEventListener("click", () => { /* … */ });
  });
  ```

  同一段落里的 `[data-mobile-search]` 绑定（`HEAD:38`）同样在顶层。
- 同一个文件里已经有一个正解可参照：`09-19-shell-alignment` 新写的 toast 脚本
  把查找推迟到 `DOMContentLoaded`，并在注释里写明了这个陷阱
  （`workspace-nav.html:57-59`、`:80`）。修法照它即可，不要另创一套。

#### 同一个根因还有第二个受害者（2026-09-20 由 ④ 发现并上报）

`09-19-list-pages-alignment` 的实施者在实机探测时发现，同一个 `<script>` 里
绑 `change → requestSubmit` 的那段（`.object-list-filter-fields select/input`，
`workspace-nav.html:110-112`）**同样是死代码**，且**不分屏宽**，桌面档一样失效。

后果：`/properties` 与 `/rooms` 在桌面档**根本没有筛选提交路径** —— 这两页的筛选控件
只有下拉，没有 `type="submit"` 按钮；唯一的提交来源就是这段死掉的 `change` 绑定。

④ 在自己的两个列表模板上加了 `onchange="this.form.submit()"` 作为**局部绕行**
（与 `/bills`、`billing_page.go:283` 的既有写法一致），**没有动 ② 的外壳文件**。
所以：

- `/properties`、`/rooms` 桌面档现在能提交了（靠绕行）。
- `workspace-nav.html:110-112` 的死代码**原样留着**，仍是死代码。
- ≤640px 的「筛选」开关**依然点不开** —— 开关打不开，里面能提交的下拉也够不着。
  本缺陷成立，且修复时必须把 `workspace-nav.html` 里那段死代码一并处理，
  否则会出现「同一个功能两套提交机制」的局面。

### 缺陷 2：`/cash-receipts?error=cash_overbalance` 对同一个错误渲染两条不同措辞的消息

`HEAD` 里同一个错误码在**三处**各写了一份文案：

| 位置 | 文案 |
|---|---|
| `web/templates/pages/cash-receipts.html:24` | 这笔现金会超过该月份未收余额，请核对已有收款。 |
| `web/templates/pages/cash-receipts.html:51` | 入账金额大于当前未收余额。 |
| `cash_receipt_handlers.go:216`（内联模板） | 这笔现金会超过该月份的未收余额，请重新核对银行与现金收款。 |

`cash-receipts.html` 的 `:24` 与 `:51` 在同一次渲染里**都**会命中，
所以用户一次失败看到两条措辞不同的消息。`cash_receipt_handlers.go:216` 那份是
另一条渲染路径（内联模板），与 `.html` 是并列关系，是否同时可达需在实施时确认。

### 缺陷 3：三处把 URL 查询参数当文本原样渲染成绿色成功提示

（2026-09-20 由 `09-19-shell-alignment` 的复验 agent 发现并上报，主会话用
`git show 8c62c3f^:…` 独立复核后归入本任务。）

`{{.Message}}` 在本仓库有**两种**写法，含义不同：

- **白名单**（多数页面，正确）：`{{if eq .Message "refreshed"}}…{{end}}` ——
  查询参数是**码**，文案写在模板里。
- **裸回显**（少数几处，错误）：`{{if .Message}}…{{.Message}}…{{end}}` ——
  查询参数被当作**文本**打印。

裸回显让「链接里写什么，页面就原样显示什么」，且带 `notice ok` 绿色成功样式：
攻击者构造一条链接，即可在受信任的应用界面内弹出**任意文本**的「成功」提示。
`html/template` 会转义 `<`/`>`，所以不是 XSS，是**内容注入／社会工程**面。

三处裸回显及其可达性（主会话逐条实测）：

| 位置 | 正规产出方 | 判定 |
|---|---|---|
| `page_data_routes.go:1524` `bankPageTemplate` | `main.go:1684` → `bankRefreshRedirect` → `/bank?message=refreshed`（POST `/bank/sync` 后） | **可达**，需按码守卫 |
| `page_data_routes.go:1511` `legacyCashReceiptPageTemplate` | **无** —— 全仓库没有任何 `Execute` 调用它，是死模板 | 死代码，但仍是陷阱 |
| `web/templates/pages/rent-workspace.html:21` | **无** —— `rent_workspace_page.go:146` 直接 `r.URL.Query().Get("message")` 灌进来，全仓库没有任何 redirect 往 `/rent-workspace` 带 `message=` | **不可达但可注入** |

主会话复核的关键事实：`8c62c3f` **之前**，`page_data_routes.go` 里有 **2 处**
裸回显、**0 处** `data-toast`。也就是说裸回显是既有缺陷；`8c62c3f`（②）只是给它
加了 `data-toast`，把它从一条静态横幅**提升成了一条会自己消失的 toast** ——
toast 比横幅更像系统自己发出的通知，欺骗性更强。所以本缺陷成立，
且修复时**不要**把 `data-toast` 去掉当作解法（那会退掉 ② 的对齐成果），
要把文案收敛成白名单。

## Requirements

- 缺陷 1：把 `workspace-nav.html` 里需要页面正文的 DOM 查找推迟到
  `DOMContentLoaded` 之后。同时检查同一 `<script>` 内**所有**在顶层查询正文元素的语句
  （至少 `[data-mobile-search]`、`.object-list-filter-toggle`、
  `.object-list-filter-fields` 三类），不要只修被报告的那一个。
- 缺陷 2：同一个错误码收敛成**一份**文案、**一个**渲染点。保留哪一条措辞由实施者
  决定，但必须全仓库唯一。修完用 `grep -rn 'cash_overbalance'` 复核没有残留的第二份文案。
- 缺陷 3：三处裸回显全部改成白名单，**保留** `data-toast`：
  - `bankPageTemplate` → `{{if eq .Message "refreshed"}}`，文案与 `billing_page.go:264`
    对同一码的既有措辞保持一致（「银行数据已刷新。」），不要再造第二种说法。
  - `legacyCashReceiptPageTemplate` → 它是死模板，按同样写法守卫即可；
    **不要**在本任务里删除它（删遗留模板另有专门任务）。
  - `rent-workspace.html` → 无正规产出方，直接删掉那一行，并把
    `rent_workspace_page.go:82` 的 `message` 形参及其 `:118`、`:146` 的传递一并清掉，
    避免留下一个「永远传空」的死参数。注意 `pageError` 参数仍在使用，不要误删。
  - 修完用 `grep -rn '{{if \.Message}}{{\.Message}}\|data-toast>{{\.Message}}' cmd/truelayer-demo/`
    复核全仓库没有残留的裸回显。
- 三处修复都要在**真实页面**上验证，不能只看代码。

## Acceptance Criteria

- [ ] ≤640px 下 `/properties`、`/rooms` 的「筛选」按钮点击后筛选区展开，
      `aria-expanded` 由 `false` 变 `true`、`.object-list-filter-fields` 的计算样式
      由 `display:none` 变 `display:grid`；再点一次收起。
- [ ] 同样两页在 **≥641px** 下按钮行为不变（该档按钮本就是 `display: none`，
      要确认没有因这次改动而报错或行为变化）。
- [ ] `/tenancies` 的筛选**不受影响**：它用 `type="submit"` 按钮、无开关，
      改动前后 ≤640 与 ≥641 都必须能正常提交（这是勘误后新增的反向断言，
      防止把开关机制硬套到没有开关的页面上）。
- [ ] `workspace-nav.html` 的 `<script>` 内**所有**查询页面正文元素的语句都已推迟到
      `DOMContentLoaded` 之后；用一次「把 partial 注入到不含 `<main>` 的最小页面」
      的渲染测试钉住，防止回归。
- [ ] `/properties`、`/rooms` 的筛选提交**只有一条路径**：④ 为绕行加的
      `onchange="this.form.submit()"` 要么被外壳的通用机制取代、要么被明确保留并说明
      为什么两套并存 —— 不允许出现「修好了但没人知道哪套在起作用」。
- [ ] 移动搜索框按钮（`[data-mobile-search]`）在 ≤640px 下仍能正常展开搜索框 ——
      它与缺陷 1 同源，一并回归。
- [ ] `/cash-receipts?error=cash_overbalance` 页面渲染出的该错误消息**恰好一条**。
- [ ] `grep -rn 'cash_overbalance' cmd/truelayer-demo` 后，该错误码对应的**用户可见文案**
      在全仓库只有一处；其余出现只能是错误码字面量本身或测试断言。
- [ ] 构造 `/bank?message=注入测试文本`，页面**不出现**任何绿色成功提示；
      而走正规路径（POST `/bank/sync` 后重定向）仍能看到「银行数据已刷新。」。
- [ ] 构造 `/rent-workspace?message=注入测试文本`，页面不出现绿色成功提示。
- [ ] `grep -rn 'data-toast>{{\.Message}}' cmd/truelayer-demo/` 无输出 ——
      没有任何查询参数被当作 toast 文本原样渲染。
- [ ] 上一条要有**测试**兜住，不能只靠 grep：在 `workspace_shell_test.go` 里
      （`TestOnlySuccessFlashesAreMarkedForTheToast` 旁边）加一条扫描全部模板与
      Go 模板字面量的断言，禁止任何 `data-toast` 附近的裸 `{{.Message}}`。
      该断言在修复前必然失败，所以**必须与修复同批提交**。
      参考 `.trellis/spec/frontend/responsive-conventions.md` §8.1 新加的
      「A marked notice renders fixed text chosen by the template」一段。
- [ ] 同理，`responsive-conventions.md` §8.2（980 档必须显式撤销 sticky）已有
      `TestTheCollapsedTierUnswebsTheStickyRail` 兜住，不要重复造。
- [ ] 上述验证在 `scripts/run-audit-local.sh` 起的真实实例上用真实浏览器完成
      （`scripts/audit/launch.mjs`），不指向 `:8081` / `bank.ddpl.top`。
- [ ] `go test ./cmd/truelayer-demo/...` 与 `go vet ./...` 通过。

## Out of Scope

- ≤640px 之外的移动端布局问题（属 `09-16-mobile-friendly-workspace`）。
- `09-19-pc-ui-fidelity-alignment` 父子任务树里的原型对齐工作。
- 错误提示的整体设计改版 —— 本任务只收敛重复，不改措辞风格。

## 备注

`09-19-shell-alignment` 的实施者把这两条当作**范围外**如实上报、没有顺手改，
这个判断是对的：当时该任务正在改同一个 `workspace-nav.html`，顺手改会让
「外壳对齐」的验收面混入两个无关缺陷，事后无法判断是哪次改动影响了哪条行为。
