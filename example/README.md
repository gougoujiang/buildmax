# Example Data for Demo & Testing

本目录提供多场景示例数据，供 BuildMax Agent 工具（read_file、grep、editfile 等）做演示与测试。各子目录内含 CSV 数据与 README 说明。

## 场景一览

| 目录 | 场景 | 主文件 | 典型用途 |
|------|------|--------|----------|
| [access_log](access_log/) | 访问日志 | access_log.csv | 按路径/状态码统计、错误率、慢请求 |
| [books](books/) | 图书目录 | books.csv | 按作者/分类/价格筛选、库存预警 |
| [employees](employees/) | 员工/HR | employees.csv | 按部门/职位/工龄统计 |
| [expenses](expenses/) | 记账/开支 | expenses.csv | 按分类/月度汇总、支出趋势 |
| [fitness](fitness/) | 健身/运动 | fitness.csv | 周月汇总、类型分布、卡路里统计 |
| [grades](grades/) | 学生成绩 | grades.csv | 按班级/科目统计、不及格名单 |
| [inventory](inventory/) | 库存/仓储 | inventory.csv | 低库存预警、按仓库汇总 |
| [meetings](meetings/) | 会议/日程 | meetings.csv | 冲突检测、某人日程、会议室占用 |
| [movies](movies/) | 电影 | movies.csv | 排行、类型分布、年度 Top N |
| [orders](orders/) | 订单/电商 | orders.csv | 状态筛选、按日/月汇总、退款率 |
| [recipes](recipes/) | 食谱/菜谱 | recipes.csv | 按食材查菜、按难度/分类筛选 |
| [sales](sales/) | 销售 | sales_data.csv, sales/ | 区域/产品销量、收入汇总 |
| [survey](survey/) | 问卷/调查 | responses.csv | 选项分布、多题交叉、完成率 |
| [tasks](tasks/) | 项目任务/看板 | tasks.csv | 逾期任务、按人/项目统计、状态流转 |
| [weather](weather/) | 天气记录 | weather.csv | 城市对比、温度趋势、降水统计 |

根目录另有 `shakespeare.txt` 等文本示例，可用于 read_file / grep 等工具演示。
