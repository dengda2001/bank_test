# 业务 HTTP 接口清单（2026-09-16 代码盘点）

本清单来自 `cmd/truelayer-demo/main.go`、`transaction_handlers.go`、
`tenant_detail.go`、`cash_receipt_handlers.go`、`dunning_handlers.go` 和页面表单。
E2E runner 必须为每个适用接口补成功、非法输入、越权、幂等或状态冲突断言。

## 认证与会话

| 方法 | 路径 | 业务结果/错误路径 |
| --- | --- | --- |
| GET | `/` | 登录页或已登录跳转 |
| POST | `/login-local` | 登录成功建立 v2 session；错误凭据拒绝 |
| POST | `/logout` | 清理会话 |
| GET | `/login` | TrueLayer 授权入口；无会话拒绝 |
| GET | `/callback` | OAuth state 校验；不交换伪造 code |

## 租客、房源与付款人

| 方法 | 路径 | 业务结果/错误路径 |
| --- | --- | --- |
| GET | `/tenants` | 当前用户租客列表、历史摘要、筛选入口 |
| POST | `/tenants` | 创建/更新租客；非法金额、日期、邮箱拒绝；重复更新不跨户 |
| GET | `/tenants/{tenantID}` | 当前用户租客详情与期间历史；越权不泄露 |
| POST | `/tenants/{tenantID}/payers` | 添加付款人关系；非法/越权拒绝 |
| POST | `/tenants/{tenantID}/payers/remove` | 移除关系；重复/越权安全处理 |

## 银行流水、匹配、分类与拆分

| 方法 | 路径 | 业务结果/错误路径 |
| --- | --- | --- |
| GET | `/billing` | 查询、筛选、排序、分页、待处理和期间汇总 |
| GET/POST | `/refresh` | 同步/导入银行结果；错误状态保留；仅当前用户 |
| POST | `/import-legacy` | 受控原始流水导入；重复导入幂等 |
| POST | `/billing/confirm` | 租金匹配确认；金额、月份、租客归属校验 |
| POST | `/billing/allocate` | 租金/押金/其他收入拆分；金额守恒、币种和幂等 |
| POST | `/billing/ignore` | 忽略流水；重复请求幂等 |
| POST | `/billing/restore` | 恢复流水；状态冲突和重复请求 |
| GET | `/billing/revoke` | 撤销预览；越权不泄露 |
| POST | `/billing/revoke` | 撤销自身分配；重配后金额守恒、幂等 |
| POST | `/billing/payer/preview` | 历史付款人候选预览；纯读 |
| POST | `/billing/payer/confirm` | 付款人/账单月份确认；跨用户拒绝 |

## 现金收款

| 方法 | 路径 | 业务结果/错误路径 |
| --- | --- | --- |
| GET | `/cash-receipts/new` | 当前用户租客/月份表单；越权不泄露 |
| POST | `/cash-receipts/preview` | 纯读余额预览；超余额、币种、日期拒绝 |
| POST | `/cash-receipts` | 入账；同幂等键不重复，余额锁定 |
| GET | `/cash-receipts/void` | 作废预览；越权不泄露 |
| POST | `/cash-receipts/void` | 作废/重复作废/更正；审计和余额恢复 |

## Dashboard 与邮件催缴

| 方法 | 路径 | 业务结果/错误路径 |
| --- | --- | --- |
| GET | `/rent-dashboard` | 月份、搜索、状态、排序、分页、金额和历史明细 |
| POST | `/dunning/config` | 保存当前用户显示名/Reply-To；格式和头注入拒绝 |
| POST | `/dunning/preview` | 当前页逐人固定模板预览；纯读、无 SMTP |
| POST | `/dunning/send` | 逐人发送、accepted/sent/failed/skipped、请求幂等、同日确认、单项重试 |

## 通用验收约束

- 除公开登录页外，所有接口先验证会话；数据库业务查询/写入必须带 `user_id`。
- 每个跨用户 ID、非法金额/币种/月份、重复请求和状态冲突都要有精确预期。
- 页面 302 不是成功断言本身；runner 必须跟随并读取目标页面或查询结果。
- `refresh`/真实 OAuth/真实 SMTP 只有在显式 sandbox/stub 凭据存在时才运行。
