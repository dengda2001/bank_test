# 移动端筛选开关与重复错误提示修复

独立任务，不属于 `09-19-pc-ui-fidelity-alignment` 父子任务树。

## 执行次序

**排在最后。** 用户 2026-09-20 决定：等 `09-19-pc-ui-fidelity-alignment` 的
② ③ ④ ⑤ 全部落地之后再开工，**不与它们并行**。

原因：缺陷 1 要改的 `web/templates/partials/workspace-nav.html` 正是 ② 的产物，
缺陷 2 要改的 `web/templates/pages/cash-receipts.html` 在 ②④ 的写入面内。
现在插进去会与在途改动抢同一批文件。

开工前先确认 ② ③ ④ ⑤ 均已提交，工作区没有它们的在途改动。

## Goal

修掉两个既有缺陷。二者都不是本轮原型对齐改出来的，都在 `HEAD` 里就已经坏了；
由 `09-19-shell-alignment` 的实施者在做工时发现并如实上报，经主会话用 `git grep HEAD`
独立复核确认，遂单独立项。

## Confirmed Facts

两条都用 `git grep <符号> HEAD -- cmd/truelayer-demo` 在**改动前的提交**上复核过，
确认不是本轮引入。

### 缺陷 1：≤640px 时三个列表页的「筛选」按钮点了没反应

- 涉及页面：`/properties`、`/rooms`、`/tenancies`。
- 按钮标记（`HEAD` 即存在）：`properties.html:48`、`rooms.html:54`、
  `object-lists.css:19/68`（移动档 `display: block`）。
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

## Requirements

- 缺陷 1：把 `workspace-nav.html` 里需要页面正文的 DOM 查找推迟到
  `DOMContentLoaded` 之后。同时检查同一 `<script>` 内**所有**在顶层查询正文元素的语句
  （至少 `[data-mobile-search]`、`.object-list-filter-toggle`、
  `.object-list-filter-fields` 三类），不要只修被报告的那一个。
- 缺陷 2：同一个错误码收敛成**一份**文案、**一个**渲染点。保留哪一条措辞由实施者
  决定，但必须全仓库唯一。修完用 `grep -rn 'cash_overbalance'` 复核没有残留的第二份文案。
- 两处修复都要在**真实页面**上验证，不能只看代码。

## Acceptance Criteria

- [ ] ≤640px 下 `/properties`、`/rooms`、`/tenancies` 三页的「筛选」按钮点击后
      筛选区展开，`aria-expanded` 由 `false` 变 `true`；再点一次收起。
- [ ] 同样三页在 **≥641px** 下按钮行为不变（该档按钮本就是 `display: none`，
      要确认没有因这次改动而报错或行为变化）。
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
