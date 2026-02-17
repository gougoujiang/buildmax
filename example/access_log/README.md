# 访问日志示例数据 (Access Log)

用于按路径/状态码统计、错误率、高频接口等演示。

## 文件

| 文件 | 说明 |
|------|------|
| `access_log.csv` | 简化访问日志：时间、IP、路径、状态码、耗时(ms) |

## 字段

- **timestamp**: 请求时间
- **ip**: 客户端 IP
- **path**: 请求路径
- **status_code**: HTTP 状态码（200/401/404/500）
- **duration_ms**: 响应耗时（毫秒）

## 示例查询

- 按 path 统计请求次数（高频接口）
- 按 status_code 统计（错误率、4xx/5xx）
- 按 ip 统计（活跃 IP）
- 慢请求（duration_ms 大于某阈值）
