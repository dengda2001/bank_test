# 设计：统一自动生成与入住分担领域

## 边界

继续使用现有 `tenancy_agreements`、`agreement_parties`、`rent_charges`、`rent_obligations` 作为内部版本和账务事实。新增／编辑租客资料与房间安排仍通过现有领域服务和事务写入；不引入第二套合同模型，不删除旧列或表。

## 月度账务流

```text
业务入口(userID, month, intent)
  → EnsureMonthlyRentFacts
      ├─ 本月／历史，或明确分配未来预付款：按房间有效安排生成一条 rent_charge 和个人 rent_obligations
      ├─ 普通未来月份读取：返回安排预测，不写账务事实
      ├─ 无结构化房间安排的 legacy 租客：走受控兼容生成器
      └─ legacy / structured 同月冲突：拒绝双生成并返回可识别冲突
  → workspace / history / matching / cash / dunning 读取持久化责任
```

统一服务复用 `ensureRentCharge` 和现有 repository，不让调用方分别决定 legacy 或结构化路径。结构化租客通过月份有效的 arrangement party 集合得出责任；当月为空房时不生成个人责任。数据库约束不足以保证业务组合唯一时，在事务中锁定同一用户、房间、月份的写入范围并以查询／冲突校验实现幂等；先核对当前 MySQL 索引和并发实现，再决定是否需要只增 migration。

未来月份的生成意图必须显式区分 `preview` 与 `settle/allocate`。预览只读；账务事实只在月份已开始，或服务调用者明确传入未来付款分配意图时创建。已存在的总应收及个人责任始终是分配、现金和催收所依赖的稳定事实。

## 安排版本

所有安排写入继续经过 `saveRentArrangementInTx`。服务先按 `userID` 校验房间和租客，再检查受影响月份是否已有锁定账务事实；通过后在同一事务内关闭旧版本、创建新版本并写完整参与人集合。跨房搬迁在一个事务内同时更新两间房。金额变更时保留现有责任比例缩放策略或明确提交的新责任额，并验证整数分加总严格等于房间总租金。

任何 `rent_charges` 已固化的月份不可由安排服务覆盖。已存在的 legacy obligation 与新结构化责任发生冲突时采取阻断策略；历史 obligation、allocation、cash receipt 和 dunning 关系不删除、不迁移、不猜测。

## 兼容与错误

- `EnsureMonthlyRentFacts` 识别结构化来源后，legacy `ensureMonthlyObligations` 不再为这些租客／月份创建重复责任。
- 旧租客仍可通过受控 legacy 生成路径继续收租。
- 金额不守恒、跨用户关系、已固化月份锁定和来源冲突使用可区分的领域错误，以便现有 handler 映射稳定响应。
- 本任务默认无 schema migration；只有代码与既有索引不能提供所需的幂等保证时才追加非破坏性 migration，并保护已有重复及资金关系。
