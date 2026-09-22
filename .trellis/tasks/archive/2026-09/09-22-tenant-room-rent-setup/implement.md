# 实施计划：新增租客联动入住与房间租金

## 前置与边界

- 基于 migration 014 后的 `room_rent_plans` / `room_rent_plan_members` 模型实施。
- 不增加数据库列，不把租金写回 `rooms` 或 `tenants`。
- 主会话按 Codex inline 模式直接实现；开始编码前加载 `trellis-before-dev`，完成后加载 `trellis-check`。

## 实施顺序

1. 先更新契约测试：新建房间必须出现月租、生效月份、交租日和新按钮文案；新增租客必须出现可选入住区，编辑租客不出现该区；删除当前“房间／租客表单不得出现计划字段”的过期断言。
2. 为分担算法补红灯单元测试：空成员、全部留空均分、部分固定＋剩余均分、全部固定守恒、固定金额超额、无正数剩余、稳定余数顺序。
3. 重构租金计划服务，提取可在外层事务复用的保存核心；允许空成员计划，跳过成员校验与当前月事实固化，并保留时间线版本、锁与账户作用域。
4. 加入空成员计划的月度事实测试，确保普通读取和显式生成都不会创建无个人责任的 charge；在统一生成入口和 ledger 入口加入防御。
5. 增加“创建房间＋空成员租金计划”组合服务与测试，确保成功时版本为 1，任何计划校验／所属权失败都不留下房间。
6. 增加新增租客入住表单解析与 DTO：加载房产、房间、全部计划版本和目标月成员，支持预选房产／房间／月份，并保证查询全部带 `user_id`。
7. 增加“创建租客＋可选更新房间计划”组合服务与测试：无房间只建档；有房间时校验计划版本和已有成员集合，替换新租客占位 ID，原子保存完整计划；失败整体回滚。
8. 修改房间 handler 与抽屉模板：解析必填月租、生效月份、交租日；两个动作都创建租金计划；第二动作跳到预选的新增租客抽屉。
9. 修改新增租客 handler、模板、CSS 与页面脚本：房产筛选房间、按月份加载计划、展示房间租金、新租客独立区、已有租客弱化只读区、可选租金及实时金额预览；编辑租客保持资料模式。
10. 完善错误映射和重定向上下文，覆盖无计划房间、陈旧版本、金额无效、账务锁定、跨房冲突和跨用户 ID。
11. 更新 migration 014 后端规范，明确组合表单仍写入唯一的房间计划服务，并记录空成员租金规则的事实生成约束。
12. 运行质量与浏览器验收，修复发现的问题后再进入提交／归档阶段。

## 重点文件

- `cmd/truelayer-demo/room_rent_plans.go`
- `cmd/truelayer-demo/monthly_rent_facts.go`
- `cmd/truelayer-demo/landlord_rent_ledger.go`
- `cmd/truelayer-demo/landlord_domain_operations.go`
- `cmd/truelayer-demo/tenants.go`
- `cmd/truelayer-demo/main.go`
- `cmd/truelayer-demo/page_data_routes.go`
- `cmd/truelayer-demo/web/templates/partials/room-create-drawer.html`
- `cmd/truelayer-demo/web/templates/partials/tenant-form-drawer.html`
- `cmd/truelayer-demo/web/static/css/pages/entity-drawers.css`
- 相关领域、handler、模板、桌面与移动测试

## 验证命令

```bash
go test ./cmd/truelayer-demo -run 'Test.*(RoomRentPlan|RoomCreate|TenantForm|TenantRoom|MonthlyRentFacts).*' -count=1
go test ./cmd/truelayer-demo -count=1
go test ./... -count=1
go vet ./...
git diff --check
```

若 `RENTOPS_MYSQL_TEST_DSN` 可用，运行相关 MySQL 计划时间线、锁和跨房并发测试。浏览器在 1440px 和 390px 下至少完成：

1. 新建房间并验证默认当月／1 号及必填租金。
2. 点击“保存并设置租客”，确认房产、房间、月份已预选且房间租金可见。
3. 给空房新增首位租客，不填金额，确认承担全部租金。
4. 给已有租客房间新增租客，确认已有租客位于独立弱化区且只能改租金。
5. 全部留空验证均分；只填写部分金额验证剩余均分；超额验证前后端拒绝。
6. 不选房间只保存租客，确认不产生计划成员或月度责任。

## 回滚点

- 领域算法与组合事务先独立通过测试，再连接 handler；若页面改造失败，可回退模板／handler 而保留兼容的空计划能力。
- 不做 schema migration，因此无需数据级回滚；不得删除已经产生资金引用的历史事实。
