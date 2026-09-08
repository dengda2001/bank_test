# 账单页租客管理一期技术设计

## 架构边界

一期继续保留当前单进程 Go HTTP 服务，不引入数据库、前端构建链或后台任务系统。

主要边界：

- 本系统登录：demo 级账号密码登录，用环境变量配置管理员账号密码，用 HTTP cookie session 保护账单页和银行回调。
- 银行授权：复用当前 `/login` TrueLayer OAuth flow，但在产品语义上把入口命名为“绑定银行账户”。
- 银行刷新：复用当前 `/refresh` saved login flow，通过 refresh token 换 access token，再拉取账户、余额和交易。
- 账单页：服务端渲染 HTML，第一屏展示银行连接状态、手动刷新按钮和收入交易列表。
- 交易归一化：从 TrueLayer raw transaction JSON 中抽取一期展示所需字段；保留 provider-dependent 字段的不确定性。

## 路由设计

建议路由：

- `GET /`：未登录时显示账号密码登录页；已登录时重定向到账单页。
- `POST /login-local`：校验 demo 管理员账号密码，成功后写入 session cookie 并跳到账单页。
- `POST /logout`：清除 session cookie，回到登录页。
- `GET /billing`：需要本系统登录态；渲染账单/银行收入交易页。
- `GET /login`：需要本系统登录态；进入当前 TrueLayer 授权流程。
- `GET /callback`：需要本系统登录态；校验 OAuth state，交换 code，保存 refresh token，拉取数据，写日志，然后重定向 `/billing`。
- `POST /refresh` 或 `GET /refresh`：需要本系统登录态；使用保存的 refresh token 拉取数据。为兼容现有 demo，可以保留 `GET /refresh`，页面按钮调用它或普通跳转。

## 登录和 Session

一期使用 demo 级登录：

- `APP_ADMIN_USERNAME`：管理员用户名，未设置时可使用本地默认值。
- `APP_ADMIN_PASSWORD`：管理员密码；生产/真实部署必须设置。
- `APP_SESSION_SECRET`：session 签名密钥；未设置时可在进程内生成随机值，重启后登录态失效。

Session cookie：

- 使用 `HttpOnly`。
- `SameSite=Lax`，以便 OAuth redirect 回 `/callback` 时浏览器仍可带上 cookie。
- 在非 localhost HTTPS 部署时启用 `Secure`；本地 demo 可不开。
- cookie 内容不保存银行 token，只保存已签名的登录状态和过期时间。

## 银行连接状态

一期不调用额外 provider 状态接口，只基于本地 token 文件和刷新结果判断：

- token 文件不存在或 refresh token 为空：显示“未绑定”，展示“绑定银行账户”按钮。
- token 文件存在：显示“已绑定/可尝试刷新”，隐藏绑定按钮，展示“手动刷新”按钮。
- 手动刷新失败且错误指向 token/授权不可用：显示“需要重新绑定”，重新展示绑定按钮。

TrueLayer Data API v1 没有可直接查询的 connection status 资源；连接/用户同意通常有 90 天周期。`offline_access` 下 refresh token 可反复换取 access token，但同意过期、用户撤销、银行要求重新认证或 token 长时间未使用时，刷新仍会失败。

## 数据流

授权成功：

1. 用户已登录系统。
2. 用户点击“绑定银行账户”进入 `/login`。
3. TrueLayer 完成银行授权后跳回 `/callback`。
4. 服务校验系统 session 和 OAuth state。
5. 服务交换 code，保存 refresh token。
6. 服务拉取账户、余额、交易并写入 JSONL 日志。
7. 服务重定向 `/billing`。

手动刷新：

1. 用户已登录系统。
2. 用户在 `/billing` 点击“手动刷新”。
3. 服务读取 refresh token。
4. 服务换取 access token。
5. 服务拉取账户、余额、交易并写入 JSONL 日志。
6. 页面展示最新收入交易，或展示刷新失败/需要重新绑定。

## 收入交易归一化

从 raw transaction JSON 生成一期展示模型：

- `transaction_id`：优先 raw `transaction_id`。
- `source_id`：优先 `normalised_provider_transaction_id`、`provider_transaction_id`、`meta.provider_transaction_id`、`meta.bank_transaction_id` 等 provider 字段；没有则显示未知。
- `payer_name`：优先可能的 provider/remitter 字段；其次从 `description` 或 `merchant_name` 做保守展示，并标记为推断；没有则未知。
- `timestamp`：优先 `timestamp`。
- `amount`、`currency`：直接取 raw 字段。
- `account_id`、`account_name`：来自当前被遍历的账户。
- `description` / `reference`：优先 raw `description`，同时可展示 meta 中常见 reference 字段。
- `status`：一期只做展示状态，例如 `unknown_payer`、`inferred_payer`、`confirmed_payer`、`needs_review`。

收入判断：

- 如果 `transaction_type == "CREDIT"`，视为收入。
- 如果没有 transaction type，但 `amount > 0`，视为收入。
- 明确 `DEBIT` 或 `amount < 0` 不进入收入列表。

付款方字段风险：

- TrueLayer Data API v1 账户交易示例不保证提供结构化 remitter/payer id 和 name。
- 银行 reference 信息可能藏在 provider-dependent `meta` 字段。
- UI 必须把“银行明确返回”和“系统推断”区分开。

## 页面设计

账单页沿用现有 rent management dashboard 的克制运营工具风格，但一期信息密度更聚焦：

- 顶部：当前页面标题、银行连接状态、手动刷新、绑定银行账户。
- 概览：收入总额、收入笔数、未知付款方笔数、需要重新绑定状态、最后刷新时间。
- 主列表：银行收入交易列表。
- 详情区：选中交易的 raw-ish 关键字段、付款方识别来源、可能匹配提示。

一期不做完整导航深层页面；侧边栏可以保留视觉结构，但未实现模块应弱化或禁用。

## 兼容性

- 保留现有 `/login` 作为 TrueLayer 授权入口，避免破坏 README 和当前 demo 使用习惯。
- 保留现有 JSONL 日志格式，必要时新增归一化展示模型，不重写历史日志。
- 保留现有 refresh token 文件格式。
- 现有测试必须继续通过。

## 回滚点

- 登录/session 改动集中在 handler 和 helper 函数，若出现问题可回退到无登录 demo。
- 交易归一化只影响展示模型，不改变 provider raw payload 的保存。
- `/callback` 从 JSON 响应改为重定向是明确产品行为变更；调试需要可通过日志或内部接口补充。
