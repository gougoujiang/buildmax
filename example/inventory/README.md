# 库存/仓储示例数据 (Inventory)

用于低库存预警、按仓库汇总、周转与补货分析演示。

## 文件

| 文件 | 说明 |
|------|------|
| `inventory.csv` | 库存表：SKU、仓库、数量、最后入库日、安全库存 |

## 字段

- **sku**: 商品 SKU
- **warehouse**: 仓库（北京仓、上海仓、广州仓）
- **quantity**: 当前数量
- **last_restock_date**: 最后入库日期
- **safety_stock**: 安全库存阈值

## 示例查询

- 低库存预警（quantity < safety_stock）
- 缺货（quantity = 0）
- 按仓库汇总数量
- 超期未补货（last_restock_date 较早）
