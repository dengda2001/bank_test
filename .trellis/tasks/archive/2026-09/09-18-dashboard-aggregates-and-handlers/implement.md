# 实施计划：聚合查询与后端接口

## 实施顺序

### 1. 固定查询契约

- 新增工作台筛选、视角、状态优先级和 URL 上下文纯函数。
- 固定空置、缺账单、房产/房间/租客行状态和分页不影响汇总的测试。

### 2. 实现统一聚合服务

- 读取用户范围内 properties、rooms、charges、obligations、tenants、有效分配、现金收款和支出。
- 以 room 为最小聚合单元，再派生 property 和 tenant 视角，避免三个页面各自重算。
- 给每个视角提供稳定排序和分页后的行，同时保留全量金额/数量摘要。

### 3. 接入页面 handlers

- `/rent-dashboard` 默认 `view=properties`，支持三视角切换和房产/房间下钻。
- 新增 `/rooms/{room_id}` 详情 handler，显式校验用户归属。
- 复用既有付款、现金、支出和催缴 handler；链接统一保留 period/view/property/room/page 上下文。

### 4. 回归与交接

- 增加 handler/HTML 渲染、空状态、越权和 URL 测试。
- 运行 dashboard 纯测试、完整后端测试、`go vet ./...` 和 `go test ./...`。
- 若本地 MySQL DSN 存在，运行多房产多房间真实聚合和详情测试；否则明确记录跳过。
- 为后续模板外置任务保留稳定页面数据契约，不在本任务引入独立前端构建链。

## 预计文件

- `cmd/truelayer-demo/rent_workspace.go`
- `cmd/truelayer-demo/rent_workspace_test.go`
- `cmd/truelayer-demo/dashboard.go`
- `cmd/truelayer-demo/main.go`
- `cmd/truelayer-demo/tenant_detail.go` 或新增房间详情 handler 文件

## 验收命令

```bash
gofmt -w cmd/truelayer-demo/rent_workspace.go cmd/truelayer-demo/rent_workspace_test.go
go test ./cmd/truelayer-demo -run 'RentWorkspace|Dashboard|RoomDetail' -count=1
go test ./cmd/truelayer-demo -count=1
go vet ./...
go test ./...
git diff --check
```

## Review Gate

- 先通过纯函数和聚合测试，再接页面 handler。
- 所有金额必须能回溯到有效 charge/obligation/coverage/expense；不能只修改展示层字段。
- 新页面不展示参考号；参考号只保留在内部兼容查询或后续审计需要的模型中，不重新加入前端输出。
- 任务完成后才进入模板外置与桌面/移动端边界任务。
