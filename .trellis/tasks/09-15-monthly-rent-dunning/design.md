# 单月邮件催缴：技术设计

## 目标与边界

在月度 Dashboard 现有账单行上提供单月催缴抽屉。抽屉只使用当前选定月份和当前 Dashboard 页的有效账单投影；预览是纯读取，发送时重新读取账单并为每位租客独立投递、独立记录。邮件服务采用供应商无关的 SMTP 边界，未配置完整 SMTP 发件配置时发送必须被阻止。

## 关键决定

- 新增 `dunning_sender_configs` 保存每个用户的显示名称和 Reply-To；SMTP 主机、认证信息和服务发件地址只来自环境变量，不伪造房东地址。
- 新增 `dunning_send_attempts` 保存每次受理／成功／失败尝试的账单与邮件快照；`(user_id, request_key, obligation_id)` 唯一约束保证同一发送动作不会重复投递。失败重试使用新请求键并关联原失败记录。
- 候选列表沿用 Dashboard 当前页的有效 `rent_obligations` 投影，选择历史、当前或未来月份时重新生成适用账单；切换月、页、搜索或状态以整页 GET 清空浏览器选择和预览。
- Dublin 本地日历决定模板：应缴日当天及之前使用 `rent_reminder`，应缴日次日起未结清使用 `rent_overdue`；逾期天数为 Dublin 日期差，不按 UTC 小时计算。
- 每位收件人单独调用 `mailDelivery.Send`，收件人地址永不进入同批次其他邮件的 To/Cc/Bcc。SMTP 发送在已写入 `accepted` 记录后执行，重复请求遇到已受理／已成功记录只返回现有结果；失败项可用新请求键重试。

## 数据流

```text
Dashboard page/current rows
  -> dunning candidate read (user + period + current obligation projection)
  -> preview POST (no delivery, typed message snapshot)
  -> send POST (reload sender config + obligation + tenant, idempotency guard)
  -> accepted attempt -> SMTP delivery -> sent/failed attempt
  -> dashboard drawer result + audit history
```

## 接口与模型契约

- `dunningSenderConfig`: `DisplayName` 非空、`ReplyToEmail` 为严格合法邮箱；仅当前用户可读写。
- `dunningCandidate`: 租客、房间、账单月、应缴日、应收／实收／未收、状态、邮箱状态和最近成功发送日；仅 `balance > 0` 可预览或发送。
- `dunningPreview`: 候选 ID、收件人、模板类型、英文主题、英文正文、账单月、余额、应缴日、逾期天数及 sender snapshot；模板字段不可由表单覆盖。
- `dunningSendAttempt`: user/tenant/obligation/period、recipient、template、subject/body、amount snapshot、sender snapshot、status、operation/request key、retry parent、error、timestamps。
- 环境变量：`DUNNING_SMTP_HOST`、`DUNNING_SMTP_PORT`、`DUNNING_SMTP_USERNAME`、`DUNNING_SMTP_PASSWORD`、`DUNNING_SMTP_FROM`；SMTP 未配置时禁止发送。
- HTTP：Dashboard 内嵌抽屉；`POST /dunning/config` 保存配置，`POST /dunning/preview` 只预览，`POST /dunning/send` 逐人发送。所有路由要求 v2 或兼容会话并传入 user ID。

## 兼容与回滚

- 迁移只新增表，不修改既有租金／流水事实；删除新表即可回滚数据结构，但已发送邮件不能撤回。
- 预览和发送用同一模板生成器；发送前重新读取余额，已缴清或账单不存在时该人被跳过并记录安全结果。
- JSON fallback Dashboard 不显示可发送抽屉；没有数据库事实时不允许构造催缴邮件。

## 风险控制

- 所有 ID 查询同时带 `user_id`，并用租客与账单关联校验；posted ID 不能跨用户探测。
- Reply-To 通过 `net/mail` 严格校验并在 MIME 头中编码；主题和正文由服务端模板生成，避免头注入和任意模板注入。
- SMTP 发送器支持 context 超时和内存 fake；测试默认不读取真实凭据、不访问网络。
