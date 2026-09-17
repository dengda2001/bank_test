# 聚合查询与后端接口

## Goal

实现全局、房产、房间、租客和房间详情的统一聚合查询与 HTTP handlers，处理空置、待处理、排序、筛选、分页、URL 上下文和催缴/支出/付款操作。

## Requirements

- 从统一账务事实实现全局、房产、房间、租客和房间详情查询。
- 支持空置、待处理、逾期、部分缴纳、未到期未缴和已交满状态。
- 实现房产/房间/租客视角切换、月份、房产、房间、搜索、状态和分页参数。
- 提供房产下钻、房间详情、付款预览/确认/撤销、支出和催缴所需的 HTTP handlers。
- 所有接口显式校验 `user_id`、`property_id` 和 `room_id`，不回退到任意记录。

## Acceptance Criteria

- [ ] 房产、房间、租客三个列表和房间详情金额与账务服务一致。
- [ ] 空置房进入房间视角但不进入租金金额、已交满或未交满统计；缺账单房间显示待处理。
- [ ] 默认排序为 `needs_review -> overdue -> partial -> open -> paid -> vacant`，筛选分页不改变汇总口径。
- [ ] URL 能保留月份、视角、房产、房间、筛选和分页上下文。
- [ ] 空列表、不存在目标、越权目标和操作失败都有明确响应。

## Dependencies

- 前置：`09-18-repository-and-query-boundary`、`09-18-core-rent-ledger`。
- 后置：前端工作台依赖本任务的页面数据和操作接口。

## Verification

- Handler/HTML 渲染测试。
- 多房产、多房间、空置、待处理和统一排序测试。
- URL 参数、分页汇总、越权和错误响应测试。

## Notes

- Keep `prd.md` focused on requirements, constraints, and acceptance criteria.
- Lightweight tasks can remain PRD-only.
- For complex tasks, add `design.md` for technical design and `implement.md` for execution planning before `task.py start`.
