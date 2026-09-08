# 账单页租客管理一期实现计划

## 实现顺序

1. 梳理现有 handler
   - 确认 `cmd/truelayer-demo/main.go` 中模板、handler、token、TrueLayer client 的边界。
   - 保持当前单文件结构，除非实现中明显需要拆分。

2. 增加 demo 登录能力
   - 增加 demo 管理员配置项：用户名、密码、session secret。
   - 增加登录校验 helper、session cookie 写入/读取/清除 helper。
   - `GET /` 渲染登录页或已登录时跳转 `/billing`。
   - `POST /login-local` 校验账号密码。
   - `POST /logout` 清除登录态。

3. 保护路由
   - `/billing` 必须登录。
   - `/login` TrueLayer 授权入口必须登录。
   - `/callback` 必须登录；未登录不能交换 code、保存 token 或写银行数据日志。
   - `/refresh` 必须登录。

4. 增加银行连接状态
   - 读取本地 token 文件判断是否存在 refresh token。
   - 账单页根据 token 存在与否展示“绑定银行账户”或“已连接 + 手动刷新”。
   - refresh 失败时返回可展示的错误，提示重新绑定。

5. 增加收入交易归一化
   - 定义展示用收入交易结构。
   - 从每个账户的 raw transaction `results` 中筛选收入交易。
   - 抽取 transaction id、provider/source id、payer name、payer 字段来源、时间、金额、币种、收款账户、描述/reference、状态。
   - 对 provider-dependent 字段做安全缺省：未知或推断，不能假装确认。

6. 渲染账单页
   - 服务端模板渲染账单页。
   - 沿用现有 dashboard 的运营工具视觉方向，但主列表改为银行收入交易。
   - 顶部提供绑定银行账户、手动刷新、退出登录。
   - 授权成功 `/callback` 拉取数据后重定向 `/billing`。

7. 测试
   - 增加登录成功/失败测试。
   - 增加未登录访问 `/billing`、`/login`、`/callback`、`/refresh` 的保护测试。
   - 增加 token 文件存在时银行连接状态测试。
   - 增加收入交易归一化测试：`CREDIT`、`amount > 0`、`DEBIT`/负数过滤、未知/推断付款方。
   - 保持现有 OAuth、refresh token、交易查询和日志测试通过。

## 验证命令

- `go test ./...`

## 风险文件

- `cmd/truelayer-demo/main.go`
- `cmd/truelayer-demo/main_test.go`
- `README.md`（如果路由和运行说明变化明显，需要更新）

## 回滚策略

- 如果登录/session 引入回归，先回滚 route protection 和登录模板，保留交易归一化测试。
- 如果交易字段解析不稳定，降级为只展示 raw `transaction_id`、`description`、`amount`、`currency`、`timestamp`，付款方显示未知。
- 如果 `/callback` 重定向影响调试，保留受登录保护的调试参数或日志，不恢复未保护 raw JSON 主流程。

## 实现前检查

- 用户已确认：一期使用 demo 级账号密码登录。
- 用户已确认：主列表为银行收入交易列表。
- 用户已确认：一期只做手动刷新，不做定时刷新。
- 用户已确认：授权成功后回到账单页，并且 `/callback` 必须校验本系统登录态。
