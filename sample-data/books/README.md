# Book Catalog Example Data

A book catalog, for demonstrating filtering, aggregation, and sorting.

## Files

| File | Contents |
|---|---|
| `books.csv` | ISBN, title, author, category, publication year, price, stock |

## Columns

- **isbn** — International Standard Book Number
- **title** — book title (Chinese and English titles both appear)
- **author** — author name
- **category** — one of `技术` (technology), `文学` (literature), `科幻`
  (science fiction), `历史` (history), `科普` (popular science), `经管`
  (business), `推理` (mystery)
- **publish_year** — year of publication
- **price** — list price in CNY
- **stock** — units on hand

## Query ideas

- Count titles or sum stock per `category`
- Find every book by one author, for example `刘慈欣` or `余华`
- Filter by price range or publication year
- Low-stock alerts, for example `stock < 50`
- Sort by price or publication year
