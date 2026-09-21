# 实施计划：精简导航表单与兼容路由

## 前置条件

- [ ] `.trellis/tasks/09-21-rent-domain-auto-facts` 已完成；统一账务事实生成、legacy 隔离和入住分担校验通过。

## 顺序

1. 检查导航、账单／租约页面、工作台动作、交易详情、现金、催收和匹配的路由调用图；特别区分 `/bills` 与 `/billing`。
2. 先加入路由／模板契约测试，覆盖导航入口、工作台责任列表、旧 GET 重定向、旧 POST、筛选上下文和安全 return_to。
3. 从桌面侧栏与移动底栏移除独立 bills 导航；将必要的责任状态和操作放入租客工作台视角。
4. 精简房间和租客表单，接入已验收的入住服务，补充责任分担预览；保留未绑定租客创建路径。
5. 更新房间／租客详情和账单／租约兼容页面；把内部旧链接迁移至租客／房间／月份上下文。
6. 运行路由、页面、模板测试，以及桌面／移动浏览器验收；更新父任务集成清单。

## 重点文件

- `cmd/truelayer-demo/rent_collection_pages.go`
- `cmd/truelayer-demo/transaction_detail.go`
- `cmd/truelayer-demo/workspace_shell.go`
- `cmd/truelayer-demo/web/templates/partials/workspace-nav.html`
- `cmd/truelayer-demo/web/templates/partials/collection-settle-form.html`
- 相关租客／房间模板、路由与页面测试

## 验证

```bash
go test ./cmd/truelayer-demo -run 'Test.*(Bill|Tenan|RentWorkspace|Navigation|Mobile|Desktop|Route).*' -count=1
go test ./...
git diff --check
```

若浏览器验收环境可用，验证 1440px 与 360/390/430px 下创建房间、绑定租客、调整分担、查看工作台和旧 URL 跳转。

## 风险控制

- `/billing` 是银行流水匹配别名，不能被 `/bills` 清理误删。
- 兼容 POST 必须保留既有认证、所有权、CSRF（若当前路由使用）和金额／状态校验。
- `return_to` 仅允许本站路径，默认回到当前工作台租客视角。
- UI 不直接编辑付款分配或重写已固化责任。
