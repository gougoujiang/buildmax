# 项目任务/看板示例数据 (Tasks)

用于逾期任务、按人/项目统计、状态流转等演示。

## 文件

| 文件 | 说明 |
|------|------|
| `tasks.csv` | 任务表：任务 id、项目、负责人、状态、截止日、优先级 |

## 字段

- **task_id**: 任务 ID
- **project**: 项目名称
- **assignee**: 负责人
- **status**: 状态（todo / in_progress / done / overdue）
- **due_date**: 截止日期
- **priority**: 优先级（high / medium / low）

## 示例查询

- 逾期任务（due_date 已过且 status 非 done）
- 按项目或负责人统计任务数
- 按状态筛选（进行中、待办等）
- 按优先级排序或统计
