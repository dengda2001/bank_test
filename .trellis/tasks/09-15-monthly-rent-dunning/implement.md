# 单月邮件催缴：实施计划

## 启动门槛

- [x] 审阅催缴 PRD、父任务依赖和 Dashboard 读模型。
- [x] 检索仓库，确认没有现成邮件供应商、SMTP 适配层或发送记录表。
- [x] 采用供应商无关 SMTP + 内存 fake 的本地安全实现假设；真实 SMTP 未配置时发送拒绝。
- [x] 执行 `trellis-before-dev`，读取 backend database/error/quality 及跨层规范。

## 纵向切片

### 切片 1：邮件模板、配置和候选读模型

- [x] 新增发送配置与催缴尝试迁移，定义 typed sender/candidate/preview/attempt。
- [x] 实现 Dublin 模板选择、金额／逾期天数和严格邮箱校验。
- [x] 实现按用户、单月、当前 Dashboard 页的候选查询与最近成功发送状态。
- [x] 覆盖单月范围、跨月不混欠款、已缴清／无邮箱／同日成功过滤和用户隔离。

### 切片 2：预览、SMTP 投递和重试保护

- [x] 实现内存 fake 与供应商无关 SMTP 适配器；缺少 SMTP 配置时拒绝发送。
- [x] 预览不写发送记录、不调用投递器；发送前重读账本余额。
- [x] 实现每人独立 accepted/sent/failed 记录、请求幂等、失败重试和同日成功二次确认。
- [x] 覆盖预览纯读、失败重试、请求幂等、收件人隔离、发送快照和跨用户授权。

### 切片 3：Dashboard 抽屉和配置入口

- [x] 在 Dashboard 当前页增加催缴入口、单月选择、候选勾选、预览和逐人结果。
- [x] 搜索／状态／月份／页码变化清空选择与预览；只允许当前页选择。
- [x] 展示缺配置、无效邮箱、已发送、失败、已缴清和成功状态，并提供单项重试。
- [x] 更新 HTTP／模板测试；当前环境无可用 MySQL DSN／DevTools MCP，浏览器数据库路径留待集成环境复核。

## 检查点

- [x] `go test ./cmd/truelayer-demo -count=1`、`go test ./... -count=1`、`go vet ./...` 和 `git diff --check` 通过。
- [x] `dunning_send_attempts` 每条记录都有 user、账单月、金额和邮件快照；没有真实 SMTP 配置时测试不会联网。
- [x] 所有 dunning 查询／写入严格按 user ID；Dashboard 原有银行／现金明细无回归。

## 完成门槛

- [x] PRD 六条验收标准都有单元、数据库或 HTTP 证据；MySQL／浏览器验收受当前环境缺少 DSN 与 DevTools 的限制，保留为可执行集成测试。
- [x] 更新 backend database spec，完成 Trellis quality check。
- [ ] 独立提交、推送、归档并记录 journal；之后再启动 API E2E 子任务。
