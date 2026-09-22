# 模块化单体改造方案（提案）

## 状态

**提案中，尚未批准实施。** 本文记录一次只读架构评估后的建议，不代表已经做出技术决策，也不授权直接开始重构。实施前应先确认范围、优先级和验收基线。

## 摘要

建议将当前应用渐进整理为模块化单体：继续使用一个 Go 服务、一个 MySQL 数据库、一个部署产物和 Go 服务端模板；按业务领域建立 Go package 边界，让 HTTP、业务规则、持久化和页面展示各自有明确职责。

本提案不包含 SPA、独立前端部署、微服务、数据库重构或产品交互改版。未来如确实需要前后端分离，可在现有业务服务之外新增 JSON HTTP 适配层，而不复制业务规则。

## 背景与现状

截至 2026-09-20 的代码检查：

- 核心应用位于 `cmd/truelayer-demo`，约 2 万行生产 Go 代码，49 个生产 Go 文件都属于 `package main`。
- `newAppMux` 注册约 57 个路径；大多数页面交互使用 HTML 表单、重定向和服务端模板，目前没有供浏览器使用的 JSON API。
- 页面既有独立模板和静态资源，也有内嵌在 Go 文件中的模板字符串。
- 数据模型和迁移涉及约 21 张业务表，覆盖租客/租约、账单、流水分配、现金收款、支出、催缴和银行同步。
- 已存在若干可复用的 service/repository，例如流水、租客、支出、租金工作台和房产仓储；因此可逐步建立边界，不必重写业务逻辑。
- 测试资产包括 `cmd/truelayer-demo` 下约 47 个测试文件和约 340 个测试函数，以及 HTTP E2E 场景。
- 本次检查运行 `go test ./...` 时发现 `cmd/truelayer-demo` 有 11 个失败测试。这个结果应作为迁移前待确认的测试基线，不要把失败归因于本提案或在模块迁移中顺手改变相关行为。

以上数字是用于估算的仓库快照，不是长期不变的指标。

## 目标

1. 保持现有用户流程、URL、查询参数、表单字段、成功/错误提示码和重定向路径兼容。
2. 让 handler 只负责 HTTP 边界，不直接执行复杂业务规则或 GORM 查询。
3. 让业务模块可以独立测试，不依赖 `net/http`、HTML 模板和页面文案。
4. 让数据库读写的归属清晰，并继续强制按当前 `user_id` 做数据隔离。
5. 将页面模板和静态资源放在独立文件中，继续由 Go 二进制嵌入并提供服务。
6. 为将来增加 JSON API 留出稳定的业务接口，但本轮不建设 API。

## 非目标

- 不引入 React、Vue、前端构建链或独立前端仓库。
- 不拆微服务，不改变进程数、部署拓扑或数据库。
- 不重做登录和 Cookie 会话，不调整 TrueLayer OAuth 流程。
- 不改变表结构、迁移历史、账务语义或页面交互。
- 不为了“整洁”给每个小函数增加接口、工厂或通用框架。

## 目标结构

```text
cmd/truelayer-demo/
  main.go                       # 加载配置、创建依赖、启动 HTTP 服务

internal/
  app/
    bootstrap.go                # 依赖装配与服务初始化
  application/
    workflows/                  # 跨业务模块的写入用例编排
    workspace/                  # 工作台/列表聚合只读模型
  platform/
    config/                      # 环境变量和配置校验
    database/                    # MySQL 连接、迁移入口
  identity/                      # 用户认证、会话验证、当前用户身份
  portfolio/                     # 房产、房间、租约、租客及付款人
  ledger/                        # 租金责任、银行流水、分配、现金收款
  expense/                       # 支出与发票
  collection/                    # 催缴候选、预览、发送记录
  banking/                       # TrueLayer 连接、令牌和同步
  adapters/
    truelayer/                   # TrueLayer HTTP/OAuth 协议适配
    smtp/                        # 邮件协议适配
  httpui/
    router.go                    # 路由注册
    middleware.go                # 认证等 HTTP 中间件
    render.go                    # HTML 渲染及页面级错误适配
    handlers/                    # 按业务切片组织的 HTTP handler
  webassets/
    embed.go                     # go:embed 入口
    templates/                   # 页面、layout、partials
    static/                      # CSS、JS、图标等资源

migrations/                      # 保持当前 SQL migration 来源与顺序
```

依赖方向：

```text
cmd/truelayer-demo/main.go
      ↓
app bootstrap ─────────────→ httpui handlers
                                   ↓
                   application workflows / workspace queries
                                   ↓
                  portfolio / ledger / expense /
                  collection / banking / identity
                         ↙                 ↘
            module repositories          external ports
                    ↓                         ↓
                  MySQL             adapters ─────→ TrueLayer / SMTP
```

`application/workspace` 可组合多个业务模块的只读数据，为仪表盘、账单页和工作台提供聚合视图。跨域写入流程由 `application/workflows` 编排；不要让业务模块相互导入形成循环依赖。`adapters` 实现 TrueLayer、SMTP 等外部协议，依赖方向由 composition root 注入，业务包不反向依赖具体 adapter。

## 模块职责与边界

| 模块 | 拥有的规则/数据 | 不负责 |
|---|---|---|
| `identity` | 密码认证、签名会话、用户身份解析 | 页面跳转、页面文案 |
| `portfolio` | 房产、房间、租约、租客、付款人及其关系 | 流水匹配、HTTP 表单 |
| `ledger` | 租金责任、流水匹配/分配、账本状态、现金收款账务规则 | HTML 展示、请求 Cookie |
| `expense` | 支出业务规则、发票元数据和文件访问 | 工作台聚合、HTTP 重定向 |
| `collection` | 催缴候选、发送策略、发送尝试记录 | SMTP 协议细节、页面渲染 |
| `banking` | TrueLayer 授权/刷新/同步、银行令牌存储 | 账单页面状态 |
| `application/workspace` | 跨域只读聚合、筛选与分页读模型 | 取代各领域的写入规则 |
| `application/workflows` | 跨业务模块的写用例协调 | HTML、数据库模型细节 |
| `httpui` | 路由、请求解析、响应、HTML、状态码和兼容重定向 | 领域决策、直接业务 SQL |
| `platform` | 配置、DB 初始化、基础设施生命周期 | 业务流程 |
| `adapters` | TrueLayer、SMTP 等外部协议实现 | 领域规则和页面响应 |

### 跨模块查询例外

租金工作台和流水页面需要组合多个业务表。允许 `application/workspace` 持有专用只读查询/聚合代码，但必须满足：

- 查询显式按 `user_id` 隔离，并对关联数据作同样的归属验证。
- 只负责构建读模型，不负责更新领域状态或绕过领域写入规则。
- 对外输出稳定的、与模板无关的 DTO；模板专用的展示格式在 `httpui` 转换。
- 查询语义和账务计算仍有对应测试；不得为了抽象层级而把复杂查询拆成大量往返数据库的调用。

### 持久化和接口策略

- 第一阶段允许每个业务 package 内同时放模型、service 和 GORM repository，避免一开始就铺设庞大的全局 persistence 抽象。
- GORM 查询只能出现在业务模块自己的 repository 或明确归属的 `application/workspace` 只读查询中，handler 不直接访问数据库。
- 只有确实存在第二种实现、独立测试替身需求或基础设施隔离收益时，才引入 repository interface；不要预先为每个 repository 机械地创建接口。
- SQL migration 仍是 schema 的唯一来源；不使用 GORM `AutoMigrate`。
- 业务 service 接收显式 `context.Context`、`userID` 和类型化 input，并返回 result/error；不得从 HTTP 请求或全局状态读取身份。

## 稳定契约

### 页面兼容

迁移 handler 或模板期间保持：

- 原页面路径和兼容别名有效。
- 原查询参数、表单字段名、`return_to` 语义和多值字段行为不变。
- 成功与校验失败时继续重定向到相同路径，使用已有 message/error code。
- 未登录 HTML 请求仍使用现有登录跳转策略。
- TrueLayer OAuth callback、静态资源路径和发票下载仍可用。

先搬迁职责和代码位置，不顺手改用新的 REST 语义。待独立决策后再考虑 JSON API。

### 业务接口示例

```go
type ConfirmRentMatchCommand struct {
    UserID           uint64
    TransactionID    uint64
    RentObligationID uint64
    RememberPayer    bool
}

type ConfirmRentMatchResult struct {
    TransactionID uint64
    TenantID       uint64
    Period         time.Time
}

func (s *Service) ConfirmRentMatch(
    ctx context.Context,
    cmd ConfirmRentMatchCommand,
) (ConfirmRentMatchResult, error)
```

此类接口不暴露 `http.Request`、表单字段名、状态码、URL 或页面提示文案。handler 负责将 form 转为 command，再将 service 结果/错误映射回现有页面行为。

### 错误映射

业务层使用可判别的错误（例如验证错误、未找到、冲突和基础设施错误）；HTTP 层负责映射：

| 错误类别 | HTML 页面处理 |
|---|---|
| 输入验证失败 | 回到原表单路径，保留现有 error code 和草稿语义 |
| 未登录 | 沿用当前登录跳转 |
| 资源不存在/不属于当前用户 | 返回安全的 404 或既有页面错误行为，不泄露其他用户数据 |
| 账务冲突/状态不允许 | 映射到现有冲突提示或预览流程 |
| 未预期基础设施错误 | 记录服务端错误并返回通用 500，不将内部错误细节显示给用户 |

错误适配需要逐 handler 迁移并有测试，不在第一阶段全局替换现有文案。

## 实施顺序与估算

以熟悉 Go 和现有业务的开发者估算，单位为人日；包括对应阶段的实现和验证，不含 SPA/JSON API、不含并行产品需求导致的返工。总量约 **23–37 人日**，日历时间受评审、测试数据库和现有功能开发影响。

| 阶段 | 范围 | 估算 | 验收点 |
|---|---|---:|---|
| 0. 基线与清点 | 确认测试现状；记录路由、表单、重定向及核心浏览器流程 | 3–5 | 有可比较基线；已知失败单独列出 |
| 1. 结构骨架 | bootstrap、路由注册、认证中间件、模板 renderer 和依赖规则 | 2–3 | 旧路由继续运行；新增代码按新入口组织 |
| 2. Portfolio 试点 | 房产、房间、租客、付款人和租约，按垂直切片迁移 | 4–6 | 行为兼容；handler 不含 GORM；跨用户保护测试通过 |
| 3. Ledger 核心 | obligation、流水匹配/分配/撤销、现金收款与人工平账 | 6–9 | 账务和幂等规则单一归属；MySQL 集成测试通过 |
| 4. 其他业务模块 | 支出/发票、催缴、TrueLayer 连接与同步 | 4–7 | 文件、SMTP、OAuth/同步流程可单独测试 |
| 5. Workspace 与资源清理 | 聚合只读模型；将遗留模板/CSS/JS 全部移为文件 | 3–5 | Go 文件不再内嵌整页 HTML；单二进制部署保留 |
| 6. 整体验收 | 全量测试、E2E、桌面/移动浏览器回归、文档更新 | 1–2 | 所有已知基线差异均解释或修复 |

建议先批准阶段 0–3，约 **15–23 人日**。它覆盖模块化收益最高、账务风险最大的边界；阶段 4–6 可根据前半段结果再决定。

## 迁移方式

按垂直业务切片迁移，不先做大规模文件搬家：

1. 先给目标切片补足行为测试，记录现有页面请求/响应契约。
2. 建立 package API，把一项业务规则从 handler 搬入 service。
3. 将旧 handler 改成薄适配层，调用新 service，继续返回旧页面响应。
4. 运行单元、HTTP、MySQL 和浏览器验证。
5. 仅在该切片稳定后再移动模板和删除重复实现。
6. 每个切片独立提交和回滚；不在一次 PR 中重写整组页面。

依赖顺序建议：房产/租约和租客边界 → 流水与账本 → 支出/现金/催缴/银行 → 工作台聚合和遗留模板清理。账务模块优先于纯展示整理，因为它状态复杂、影响面大。

## 测试与验收

每个迁移切片至少检查：

- service 单元测试：金额、状态转换、匹配、幂等和边界条件。
- HTTP handler 测试：方法、参数错误、认证、状态码、重定向和页面错误码。
- MySQL 集成测试：迁移、用户隔离、事务和唯一性约束。
- E2E/浏览器测试：主流程和桌面/移动视口行为。

全量质量门：

```sh
go test ./... -count=1
go vet ./...
./scripts/run-mysql-test-clean.sh ./cmd/truelayer-demo -count=1
./scripts/run-e2e-local.sh
```

`go test ./...` 若因已知基线失败，必须先确认每项失败的归属、修复或在评审中明确豁免，不能将“其余测试通过”表述为全绿。浏览器脚本只在其文档约定的本地服务配置下运行。

模块化完成的结构性验收：

- `cmd/truelayer-demo/main.go` 只做启动和依赖装配。
- HTTP handler 不直接访问 GORM，不承载核心业务计算。
- 业务包不导入 `net/http` 或 `html/template`。
- 所有查询都保持用户归属过滤；任何跨域读模型有显式归属和安全测试。
- 页面模板、CSS、JS 都是独立文件，并仍由 Go 服务单独提供。
- 迁移范围内的 URL、表单和重定向契约通过兼容测试。
- 现有单二进制部署方式、迁移顺序和数据保持不变。

## 风险与应对

| 风险 | 影响 | 应对 |
|---|---|---|
| 把重构与页面行为调整混在一起 | 难以定位回归，重估失真 | 本提案约束“先保持行为”；产品修改单独切片 |
| 当前存在测试失败 | 迁移回归无法与既有失败区分 | 阶段 0 建立、确认并维护基线 |
| 账务规则分散或重复 | 匹配、分配、责任余额产生差异 | ledger 切片先定义唯一业务入口，保留账本集成测试 |
| 用户数据隔离丢失 | 跨账户数据暴露或误改 | 所有 service 显式接收 userID；跨用户 E2E/SQL 集成测试 |
| 过度抽象 repository/interface | 产生额外样板和迁移成本 | 默认具体实现；仅在有替代实现或测试收益时引入接口 |
| workspace 查询跨域耦合 | 模块循环依赖或慢查询 | 只读聚合集中在 `application/workspace`；领域写操作仍回到所属 service |
| 持续 UI 改动撞上迁移 | 重复修改模板和测试 | 每次迁移范围小、feature 行为冻结或明确拆分 |
| 一次性搬迁过多文件 | PR 难审、回滚困难 | 先变依赖，再按模块移动；切片独立提交 |

## 方案选择与理由

### 采用：同进程模块化单体

- 保留现有部署简单性和 Go 服务端渲染体验。
- 先解决代码职责和依赖耦合，不引入当前没有直接收益的前端构建、API 认证和跨域部署问题。
- 现有 service/repository 和测试可逐步复用。
- 未来可让 HTML 和 JSON 两种 transport 共用相同业务 service。

### 暂不采用：前后端完全分离

这会同时新增 API 契约、JSON 错误语义、前端路由/状态/构建、API 认证与 CSRF/CORS、独立发布和测试迁移。只有在团队协作、客户端扩展或交互需求足以覆盖这笔持续成本时再单独立项。

### 暂不采用：微服务

现阶段没有独立扩缩容、隔离部署或团队自治的明确需求。按账务、支出、催缴拆进程会增加网络故障、事务边界、部署和运维成本。

## 评审时需确认

1. 是否接受“保持页面和业务行为不变”的重构范围？
2. 是否先只批准阶段 0–3，再以评审结果决定后续阶段？
3. 当前 11 个失败测试应作为重构前置修复，还是某些已被产品变更淘汰、需要正式更新测试契约？
4. 租客/付款人是否与房产/租约一起归属 `portfolio`，还是单独保留 `tenant` 模块？本提案为减少跨包依赖，先放在同一 portfolio 边界。

---

本文件是架构提案和实施拆解，不是已经批准的 ADR。确认采用后，可将状态改为“已接受”，并另行建立正式实施任务及阶段验收清单。
