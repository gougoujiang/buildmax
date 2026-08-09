# Inventory / Warehousing Example Data

Stock levels across warehouses, for low-stock alerts, per-warehouse totals, and
restocking analysis.

## Files

| File | Contents |
|---|---|
| `inventory.csv` | SKU, warehouse, quantity, last restock date, safety stock |

## Columns

- **sku** — stock keeping unit
- **warehouse** — `北京仓` (Beijing), `上海仓` (Shanghai), `广州仓` (Guangzhou)
- **quantity** — units currently on hand
- **last_restock_date** — date of the most recent restock
- **safety_stock** — threshold below which the SKU should be reordered

## Query ideas

- Low stock, where `quantity < safety_stock`
- Out of stock, where `quantity = 0`
- Total units per warehouse
- SKUs not restocked for a long time, by `last_restock_date`
