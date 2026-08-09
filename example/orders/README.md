# E-commerce Orders Example Data

An order table, for status filtering, daily and monthly rollups, and refund or
cancellation rates.

## Files

| File | Contents |
|---|---|
| `orders.csv` | Order ID, user ID, product, quantity, amount, status, date |

## Columns

- **order_id** — order number
- **user_id** — customer ID
- **product** — product name, in Chinese, for example `无线耳机`
  (wireless earbuds)
- **quantity** — units ordered
- **amount_cny** — order total in CNY
- **status** — `pending`, `shipped`, `completed`, `cancelled`, or `refunded`
- **order_date** — date the order was placed

## Query ideas

- Filter by `status`, such as awaiting shipment or cancelled
- Revenue by date or by customer
- Refund and cancellation rates
- One customer's orders, or units sold for one product
