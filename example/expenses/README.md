# Personal Expenses Example Data

A personal spending ledger, for totals by category or month, spending trends,
and per-account breakdowns.

## Files

| File | Contents |
|---|---|
| `expenses.csv` | Date, category, amount, payment account, note |

## Columns

- **date** — transaction date
- **category** — one of `餐饮` (dining), `交通` (transport), `购物` (shopping),
  `住房` (housing), `娱乐` (entertainment), `学习` (education), `医疗`
  (medical), `通讯` (telecom), and others
- **amount_cny** — amount in CNY
- **account** — payment method: `支付宝` (Alipay), `微信` (WeChat Pay),
  `银行卡` (bank card), or `无` (none)
- **note** — free-text note

## Query ideas

- Sum amounts per `category`
- Totals over a date range or by month
- Total spend on one account
- Large single transactions, where `amount_cny` exceeds a threshold
