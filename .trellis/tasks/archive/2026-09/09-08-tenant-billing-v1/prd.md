# 账单页租客管理一期

## 目标

把当前 TrueLayer 银行数据 demo 改造成租客管理系统的第一个可用切片：先做账号密码登录，登录后进入账单页，重点展示银行收入交易和轻量账单状态。

一期要让操作者能够：

- 从首页用账号密码登录。
- 登录后进入账单页。
- 在账单页点击“绑定银行账户”进入现有 TrueLayer 授权流程。
- 在已有 `refresh_token` 时不重复授权，直接刷新银行数据。
- 查看收入交易列表，并尽可能区分转入的人，展示付款方 id 和 name；如果银行数据没有明确字段，要标明未知或推断。

## 代码库已确认事实

- 当前应用是 `cmd/truelayer-demo/main.go` 里的小型 Go HTTP 服务。
- 当前路由包括 `/`、`/login`、`/callback`、`/refresh`。
- `/login` 会进入 TrueLayer OAuth authorization code 授权流程。
- 当前 `/callback` 会校验服务端保存的 OAuth state，交换授权码，保存返回的 refresh token，拉取账户、余额、交易，追加写入 JSONL 日志，然后返回格式化 JSON。
- `/refresh` 会读取本地保存的 refresh token，刷新 access token，重新拉取账户、余额、交易，追加写入 JSONL 日志，然后返回格式化 JSON。
- 默认 scope 包含 `offline_access`，说明当前 demo 已经按 refresh token 复用来设计。
- 当前 token 文件只保存 `refresh_token` 和 `saved_at`，不保存 access token。
- 当前账户模型包含 `account_id`、`account_type`、`display_name`、`currency`、provider raw JSON。
- 当前交易数据还是 raw JSON，没有归一化成 typed transaction model。
- 现有测试覆盖 OAuth URL、服务端 state 校验、refresh token 持久化、交易查询日期边界和结果日志写入。
- `demo/rent-management-v1-dashboard.html` 已经有租赁账单工作台视觉稿，包括月度汇总、账单列表、审核队列、选中账单详情和银行连接状态。
- 现有 V1 设计文档建议 Google 登录，但也明确提到客户需要时可以改为密码登录；本任务以当前明确需求为准，一期使用账号密码登录。
- TrueLayer Data API v1 交易示例字段包括 `transaction_id`、`timestamp`、`description`、`amount`、`currency`、`transaction_type`、`transaction_category`、`transaction_classification`、`merchant_name`、`running_balance`、`meta`。
- TrueLayer 文档说明 `transaction_id` 可能在不同请求之间变化；更稳定的 provider transaction id 可能出现在可选 provider 字段中。
- TrueLayer 文档说明交易的额外 reference 信息因银行而异，一般在 `meta` 下；Data API v1 的账户交易示例没有保证提供干净的付款方/remitter id 和 name 字段。

## 需求

- 首页显示账号密码登录页，不再显示当前 TrueLayer demo 说明页。
- 一期登录采用 demo 级门禁：用环境变量配置一个管理员账号和密码，登录后用 cookie session 保护账单页。
- 一期不做用户表、注册、改密码、正式密码哈希迁移、角色体系和邀请流程。
- 登录成功后进入账单页。
- 未登录用户不能直接访问账单页。
- TrueLayer `/callback` 也必须校验本系统登录态；没有登录态时不能交换授权码、保存 token 或展示银行数据。
- TrueLayer 授权成功后重定向回账单页，并让用户在账单页看到最新刷新状态/收入列表；不再把格式化 JSON 作为用户主界面。
- 调试 JSON 可以作为内部接口、日志或开发辅助保留，但不能绕过登录态保护。
- 账单页是登录后的第一个页面；一期不做完整租客、房源、合同管理。
- 账单页包含“绑定银行账户”按钮。
- 点击“绑定银行账户”进入当前 `/login` 使用的 TrueLayer 授权流程。
- 当存在可用 refresh token 时，“绑定银行账户”按钮隐藏，或替换为已连接银行状态。
- 当存在可用 refresh token 时，应用可以不重新授权，直接刷新交易数据。
- 一期账单页只做手动刷新按钮，不做定时刷新。
- 手动刷新直接使用当前 `/refresh` 的 saved login 行为：读取本地保存的 refresh token，换取新的 access token，再拉取账户、余额和交易。
- 用户不需要每次刷新都重新进行银行授权；只有没有 refresh token、refresh token 不可用、银行授权/同意过期、用户撤销授权或 provider 返回需要重连时，才需要重新绑定银行账户。
- TrueLayer `offline_access` 的 refresh token 可用于反复换取 access token，但连接/用户同意通常有 90 天有效期；如果 refresh token 一开始长时间未使用或同意过期，刷新可能失败，需要重新授权。
- 一期不做定时刷新，也不做后台保活任务；页面需要把刷新失败显示为“需要重新绑定/重新授权”的状态。
- 账单/交易列表重点展示收入，不展示支出流水。
- 收入交易判断优先使用 `transaction_type == "CREDIT"`，也兼容 `amount > 0`。
- 交易列表至少展示：交易 id、付款方/来源 id、付款方名称、日期时间、金额、币种、收款银行账户、描述/reference、匹配或审核状态。
- 由于付款方 id/name 字段取决于银行返回值，UI 必须区分“银行明确返回”和“从 description/meta 推断”的值。
- 如果没有明确付款方 id/name，要显示“未知”，不能静默当作已确认租客身份。
- 一期主列表明确做“银行收入交易列表”，不做“账单 + 匹配收入”的混合主视图。
- sample 租客/账单信息可以作为交易状态、可能匹配对象或视觉辅助出现，但不作为一期的数据主线。
- 如果没有持久化租客/合同数据，不要求真实生成月度应收账单。
- 现有敏感 token 和银行 payload 处理不能退化。

## 验收标准

- [ ] 访问 `/` 显示账号密码登录页。
- [ ] 使用配置的 demo 管理员账号密码可以登录并进入账单页。
- [ ] 未登录用户直接访问账单页会被拦回登录页或收到未授权响应。
- [ ] 未登录用户访问 `/callback` 不会完成 token exchange，也不会写入 refresh token 或银行数据日志。
- [ ] 无保存 refresh token 时，账单页显示绑定银行账户入口。
- [ ] 点击绑定银行账户会进入现有 TrueLayer 授权流程。
- [ ] 授权成功并拿到 refresh token 后，账单页能显示已连接银行状态，而不是继续要求绑定。
- [ ] 授权成功后浏览器回到账单页，而不是停留在 raw JSON 页面。
- [ ] 账单页包含手动刷新按钮，并在可用 refresh token 存在时调用 saved login 刷新数据，不要求用户重新银行登录。
- [ ] refresh token 不存在、不可用或 provider 返回授权过期时，页面提示需要重新绑定银行账户。
- [ ] 收入交易以表格或列表展示，包含交易 id、付款方/来源 id、付款方名称、日期、金额、币种、收款账户、描述/reference 和状态。
- [ ] 未知或推断的付款方 id/name 在 UI 上有明确标识。
- [ ] 现有 OAuth state、refresh token 持久化、交易查询和日志写入测试继续通过。
- [ ] 新测试覆盖 demo 登录访问控制、refresh token 连接状态，以及收入交易归一化。

## 暂不做

- 完整租客/房源/合同 CRUD。
- 真实月度应收账单生成。
- 自动匹配和自动分摊引擎。
- CSV/Excel 导入。
- 手动收款录入，除非保留为视觉占位。
- Google 登录或邀请式组织访问。
- 正式多用户认证系统。
- 发起付款或在线收租。

## 待确认问题

- 无。

## 备注

- 本 PRD 只记录需求、约束和验收标准。
- 该任务涉及登录、页面、银行数据归一化和路由行为，属于复杂任务；实现前需要补充 `design.md` 和 `implement.md`。
