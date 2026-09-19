# 技术设计：详情页对齐

父任务 `design.md` 的架构与边界同样适用，此处只记录本子任务独有的决策。

## 1. 边界

改：四个详情页模板（`property-detail.html`、`room-detail.html`、`tenant_detail.go`、`transaction-detail.html`）与对应 `pages/*.css`。

不改：账务计算、责任分摊算法、已生成账单的历史事实。

## 2. 关键决策

### 2.1 窄屏专用块的正确写法：外层 wrapper

这是本子任务唯一的已确认结构性缺陷，也是最有价值的修复。

`room-detail.html:104`：

```html
<dl class="room-facts room-mobile-facts">…</dl>
```

`room-detail.css`：

```css
:20  .room-responsibility-mobile-list,.room-mobile-facts,.detail-back-mobile,.detail-mobile-edit { display:none; }
:25  .room-facts { display:grid; … }
```

两条规则优先级相同（0,1,0），`:25` 在源码中更晚，因此对同一个元素胜出 —— 移动端专用字段在 PC 上重复渲染。

正确写法在 `property-detail.html:83`：

```html
<section class="panel surface property-mobile-facts">…<dl class="property-facts">…</dl></section>
```

**要点：`display:none` 必须落在与 `.room-facts` 不同的元素上**，让两条规则不再竞争同一个元素。修 `room-detail` 时照此办理，不要靠 `!important` 或调换行序 —— 那只是把冲突藏起来。

修完要顺带核对其他三个详情页有没有同样的双子句写法（同一个元素同时挂"桌面类 + 移动类"）。

### 2.2 「未分配」格的语义

原型「代付分配详情」是 4 个固定格：收款金额 / 本人责任 / 同住人责任 / **未分配**。当前实现的 `room-allocation-metrics` 格子是动态的（已覆盖 + 每租客 + 未覆盖），没有固定的「未分配」。

合并口径：保留每租客格（它承载了原型 4 格里表达不了的多人分摊细节），**追加**一个固定语义的「未分配」格。追加比替换风险低 —— 每租客格有既有数据装配与测试依赖。

「未分配」的定义需与既有 `未覆盖` 区分清楚：前者是收款中没有归属到任何责任的余额，后者是责任中未被覆盖的部分。二者不是同一个量。实现前先确认数据层能否区分，不能则先记录，不要用 `未覆盖` 顶替。

### 2.3 付款参考码：数据已取但未渲染

`tenant_detail.go:137` 已取 `reference`，但 `:290` 的渲染里没有它。这是纯渲染补全，不涉及新的数据查询。加复制按钮时注意 `responsive-conventions.md` 的可点击区域要求。

### 2.4 流水详情三操作：只做入口，复用已有动作

动作实现在列表行内 `billing_page.go:308,338`。详情页入口**必须复用同一套动作**，不新写业务逻辑。

关键风险：列表行内动作可能带 detail-return 上下文参数。详情页入口若不带同样的上下文，返回链路会断（`responsive-conventions.md:209-214`）。实现时先读列表行内的链接构造方式，照搬参数集。

### 2.5 一处未解观察

既有截图 `verified-current-desktop-room-detail.png` 中：租客责任表「付款来源」列为 `—`，关联收款表有表头无数据行，而 KPI 显示付款记录 1 笔。模板（`room-detail.html:73`、`:80`）与视图模型（`rent_workspace.go:474`、`:497`、`rent_workspace_page.go:292-293`）从代码看都已接通。

**无法仅凭代码判定**是数据问题还是渲染问题。实现时实机渲染该页确认，结论写入 `research/`。不要预设它是缺陷，也不要因为"代码看着对"就跳过。

## 3. 兼容与回滚

四个详情页各自独立，可分别提交与 revert。详情页的返回链路与行内跳转是既有契约，不得破坏。

## 4. 验收方式

- 四个详情页各自实机渲染，四档宽度。
- `room-detail` 需在 PC 与窄屏**两侧**各看一次，确认字段不再重复且窄屏仍显示。
- 代付分配详情需在有未分配余额与无未分配余额两种数据下各看一次。
- 断言 `scrollWidth === clientWidth`。
