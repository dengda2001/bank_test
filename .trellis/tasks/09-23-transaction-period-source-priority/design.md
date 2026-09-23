# 流水租金月份来源优先级：技术设计

## 边界与数据流

- 对银行流水建立明确的月份判断：description 可解析时是“明确月份”；description 无月份时，以有效 `transaction_time` 的 UTC 月份作为“待确认建议”。reference 只在页面展示原文，不参与月份推断。
- 导入新流水时，只有明确月份写入 `parsed_period_month`，来源写为 `description`。转账月份建议不写入这个字段，避免严格自动匹配、月度账单事实生成和其他消费者将其当成明确月份。
- 旧银行流水的 `parsed_period_source=description_reference` 不能证明月份来自 description。读取或重新核对时从原始 description 重新判断明确月份；已有已确认分配保持原样。旧行只含 reference 月份时，严格自动匹配必须按“没有明确月份”处理。
- 列表、匹配抽屉与同名历史流水使用同一展示投影，显示月份和来源（description / 转账月份·待确认）；月卡可提示建议月份，但仍需人工选择，不能把建议当成已匹配月。
- 手工余额等非银行来源已经有明确的账期，不用银行 description 规则覆盖。

## 兼容与验证

- 无需数据库迁移；显示投影从现有 `description`、`transaction_time`、`reference` 和解析字段计算，兼容历史行。
- 匹配、账单事实生成、月份排序与详情页如果使用旧的 `parsed_period_month`，需逐点检查，避免旧 `description_reference` 行继续产生自动行为或展示冲突。
- 测试覆盖 description 与转账时间冲突、无 description 月份的建议、缺失时间、旧 reference-only 行、严格自动匹配及已匹配分配不变。

## 取舍

- 忽略 reference 可能错过少数只写在 reference 中的租金月份；这些流水仍有转账月份建议并待人工确认。示例数据中未发现因此损失明确月份的情况。
