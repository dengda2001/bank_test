# 流水匹配核对抽屉：实施计划

## Before implementation

- [x] 用户审核 `prd.md` 与 `design.md`，明确同意进入实施阶段。
- [x] 读取前端 responsive、navigation、sortable heading 规范及后端 database/error handling 规范；保留前一轮未提交的列表改动，不覆盖它们。
- [x] `task.py start` 激活本任务。

## Implementation

- [x] 增加只读核对投影：唯一识别租客、全部租客选择、目标月责任/有效原流水证据、同名付款人历史及其实际匹配月份；分页与所有权过滤。
- [x] 为列表行生成可保留筛选上下文的抽屉 URL；桌面和手机的内联匹配表单改为同一抽屉入口。
- [x] 实现宽抽屉模板与样式：当前流水、身份、更换租客、月责任与已交证据、同名历史、预估覆盖与确认；实现关闭和焦点行为。
- [x] 复用现有确认服务，成功回列表、失败回抽屉；参数由服务端构造并过滤。
- [x] 处理解析月份已缴满、无账单、币种不符、没有可选月份、同名历史为空以及部分匹配等空态和边界。

## Verification

- [x] 服务测试覆盖已识别租客但无可匹配月份、已缴满月的有效分配证据、撤销分配排除、跨用户隔离、同名历史分页与真实匹配月份。
- [x] 渲染测试覆盖抽屉默认租客、全部租客可搜索、月份禁用/可选状态、表单值、URL 上下文与安全回跳。
- [x] `gofmt`、`go test ./... -count=1`、`go vet ./...`、`git diff --check`。
- [x] 使用本地浏览器在 1440、1024、390 宽度检查无裁切/重叠/文档横向溢出，长姓名与 description 可读，键盘 Escape、焦点返回、历史查看和确认流程可用。
- [x] 审核与前一轮未提交列表修改的差异边界；记录任何未覆盖的真实数据库验收限制。

## Verification record

- `go test ./... -count=1`, `go vet ./...`, and `git diff --check` passed on 2026-09-23.
- Browser preview at 1440, 1024, and 390 px verified drawer width, scroll, title visibility, no document overflow, form values, Escape close, and focus restoration. The rent workspace's month calendar popover also needed a right-edge alignment fix.
- An opt-in MySQL integration test covers account isolation for transactions, tenants, and same-payer history. `RENTOPS_MYSQL_TEST_DSN` was absent locally, so that test was skipped; real database confirmation remains an environment-level acceptance check.
- Existing uncommitted transaction description and property/room context edits were preserved and verified with the drawer work.
