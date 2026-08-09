# 订单/电商示例数据 (Orders)

用于订单状态筛选、按日/月汇总、退款与取消率等演示。

## 文件

| 文件 | 说明 |
|------|------|
| `orders.csv` | 订单表：订单号、用户、商品、数量、金额、状态、日期 |

## 字段

- **order_id**: 订单号
- **user_id**: 用户 ID
- **product**: 商品名称
- **quantity**: 数量
- **amount_cny**: 金额（元）
- **status**: 状态（pending / shipped / completed / cancelled / refunded）
- **order_date**: 下单日期

## 示例查询

- 按状态筛选（待发货、已取消等）
- 按日期或用户汇总金额
- 退款率、取消率统计
- 某用户订单列表或某商品销量
