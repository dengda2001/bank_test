# Implementation

1. 读取后端质量规范、日期/匹配测试与共享思考指南；保留工作区已有其他任务的未提交修改。
2. 先补针对真实 description 形态的行为测试：`TxnDate`、明确 `Sep Rent`、非租金月份、14/15 日、跨年、爱尔兰日期边界、可调界线与严格匹配。
3. 在银行月份证据入口集中实现日期清理、租金语义限定及待确认建议；提供单一可调整界线并记录环境变量。
4. 检查列表/抽屉/新同步/严格匹配路径，确保统一结果且既有分配没有数据写入。
5. 运行相关 Go 测试、`go test ./... -count=1`、`go vet ./...`、`git diff --check`；执行 Trellis 检查并更新规范。
6. 仅提交本任务文件及对应代码/测试/文档，归档任务并记录会话。

## Review gate

- 方案已在对话中由用户确认：忽略 `TxnDate`；明确租金月份取 description；无明确月份时按 15 日界线做待确认建议；保持已有分配。实施前再核对任务记录与此一致。

## Verification

- `go test ./... -count=1` passed before concurrent drawer work changed its template. The final shared-tree run has one unrelated failure: `TestTransactionMatchDrawerKeepsPaidMonthEvidenceAndFullTenantList` still expects the old single-match hidden fields, while the in-progress multi-target drawer now builds drafts in JavaScript. `go test ./... -skip 'TestTransactionMatchDrawerKeepsPaidMonthEvidenceAndFullTenantList' -count=1` passes. The MySQL-backed group is skipped without `RENTOPS_MYSQL_TEST_DSN`; this change does not write or migrate database records.
- `go vet ./...` passed.
- `git diff --check` passed.
- Regression tests cover real description shapes, new import, legacy display, strict matching, cutoff configuration and Dublin calendar boundaries.
