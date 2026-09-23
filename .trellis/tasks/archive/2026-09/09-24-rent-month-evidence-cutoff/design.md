# Design

## Boundary and data flow

- TrueLayer 银行流水统一经 `bankTransactionPeriod` 判断：先清理 description 内银行自带的交易日期，再要求租金语义与可识别的月份共同出现，然后调用现有 `parseReferencedPeriod` 解析月份。保持 reference 缺席。
- 清理需覆盖 `TxnDate: 25Apr2026` 等完整日期，避免独立日期成为账期。既有 `parseReferencedPeriod` 继续服务非银行/旧调用方；银行规则在专用入口收窄。
- 没有明确月份时，按 TrueLayer `timestamp` 转换为 `Europe/Dublin` 日历日期，使用可配置的 `nextMonthFromDay`（默认 15）生成待确认建议。`Month` 可展示，`Explicit=false`，`explicitMonth()` 必须继续返回 nil。
- 环境配置提供单一调整入口（`RENT_NEXT_MONTH_FROM_DAY`）；仅接受 1–31，缺失/无效回退 15。更改后重启应用即可生效，无需数据库迁移。
- 同步写入、列表投影、抽屉和严格匹配继续调用统一入口。已分配流水仅重算展示证据，不重写分配或账单。

## Compatibility and limits

- 现有明确租金月份格式继续支持。`TxnDate` 即使出现在 description 中，也只作普通银行日期处理。
- 纯月份文本不能安全断定为租金（现有三条反例均为其他用途），因此只做日期建议。
- 15 日界线是暂定启发式，受时区和入账延迟影响，故始终要求人工确认。
- 环境配置不改变用户历史已确认的匹配；回滚只需恢复前一配置或版本。
