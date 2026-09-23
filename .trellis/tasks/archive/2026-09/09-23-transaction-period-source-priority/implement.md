# 实施计划

1. 定稿 description 明确月份、转账月份待确认建议、reference 仅供核对的界面文案。
2. 读取 Trellis 后端、前端规范；梳理 `parsed_period_month` 的全部消费点及已有未提交改动。
3. 实现来源明确的月份解析与页面投影，新导入数据只持久化明确月份。
4. 调整列表、详情、匹配抽屉和历史行的来源标记；转账月份只用于待确认建议。
5. 审核严格自动匹配和月度账单事实生成对历史 `description_reference` 行的行为，确保 reference-only 和转账月份都不会自动入账。
6. 补充有意义的解析、历史兼容、匹配边界和渲染测试；运行 `go test ./cmd/truelayer-demo -count=1`、`go test ./...`、`go vet ./...`、`git diff --check`。可用时运行 MySQL 相关测试。
7. 按 Trellis 检查和收尾流程更新任务记录；只提交本任务相关改动，保留工作区已有的其他修改。

## 风险点

- 历史 `description_reference` 来源不区分 description/reference，必须重算才能确保自动匹配边界正确。
- `transaction_match_review.go`、对应模板和部分测试已经有未提交修改；编辑时保留这些内容，提交时按改动范围核对。
- 本地 MySQL 当前无法连接，真实流水中 reference 的月份分布无法直接验证。

## 执行记录

- 已完成 description-only 持久化、旧 TrueLayer 流水的读时重新判断、转账月份待确认提示、严格匹配与月度事实的明确月份保护、列表/详情/抽屉/历史行展示及回归测试。
- 排序仍按现有数据库 `parsed_period_month` 排列明确识别月；待确认的转账月份是提示，不改变该排序字段。
- 并行的列表筛选修改与测试同步更新后，`go test ./... -count=1`、`go vet ./...`、`git diff --check` 全部通过。提交时只包含本任务的改动片段。
