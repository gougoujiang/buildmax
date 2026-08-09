# Sales Example Data

Sales figures split across directories by year and quarter, which makes this the
useful fixture for `glob` and for tools that walk a tree rather than read a
single file.

## Files

| Path | Contents |
|---|---|
| `../sales_data.csv` | Flat table at the top level: date, region, product, category, units, unit price, discount, revenue |
| `2024/sales-q1.csv` … `2024/sales-q4.csv` | Per-quarter files for 2024 |
| `2025/sales-q1.csv`, `2025/sales-q2.csv` | Per-quarter files for 2025 |

## Columns

The per-quarter files use:

- **date** — transaction date
- **region** — `North`, `South`, `East`, or `West`
- **product** — product name, for example `Widget A`
- **quantity** — units sold
- **unit_price** — price per unit
- **total** — line total

The top-level `sales_data.csv` adds **category** and **discount**, and names its
quantity column **units_sold** and its total **revenue**.

## Query ideas

- Glob `sales/**/*.csv` and aggregate across every quarter
- Revenue per region or per product, within a year or across years
- Quarter-over-quarter growth
- Effect of `discount` on `revenue` in the top-level file
