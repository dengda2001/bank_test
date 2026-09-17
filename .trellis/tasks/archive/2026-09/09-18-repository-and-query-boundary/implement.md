# DAO / Repository 层：执行计划

## 实施顺序

1. 先写 Repository 契约测试和用户隔离测试，覆盖空结果、不存在目标、月份过滤以及跨用户读写。
2. 增加 typed filter/input 与 `landlordRentRepository`，先实现房产、房间、租约和参与人查询/写入。
3. 增加月度总应收、租客责任、付款分配和支出查询，并为跨表条件补齐 `user_id` join。
4. 实现关联创建前的归属校验，确保错误目标不会写入半成品关系。
5. 运行本任务测试、后端测试、`go vet` 和 `git diff --check`；真实 MySQL DSN 可用时执行 opt-in 集成测试。

## 预计文件

- `cmd/truelayer-demo/landlord_rent_repository.go`
- `cmd/truelayer-demo/landlord_rent_repository_test.go`
- `cmd/truelayer-demo/landlord_rent_repository_mysql_test.go`

## 验收场景

- 两个用户拥有同名房产时，按 ID 和列表查询均互不可见。
- 用户 A 不能通过用户 B 的房产、房间、租约或租客 ID 创建关联事实。
- 按月份读取房间总应收和租客责任时，不混入其他月份或其他房间。
- 有责任但没有付款分配时返回空分配列表；有支出但没有房间时作为房产公共支出返回。
- 跨用户目标与不存在目标使用相同的安全结果，不暴露另一用户数据。

## 回滚点

- Repository 和测试均为新增文件，可独立回退，不改动已有服务行为。
- 若后续服务发现方法边界不合适，优先调整 typed filter/input，不引入通用 ORM 层。
