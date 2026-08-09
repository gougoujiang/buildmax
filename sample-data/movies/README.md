# Films Example Data

A film list, for rankings, genre distribution, per-year top N, and rating
filters.

## Files

| File | Contents |
|---|---|
| `movies.csv` | Title, genre, year, rating, box office |

## Columns

- **title** — film title
- **genre** — one of `科幻` (sci-fi), `剧情` (drama), `喜剧` (comedy), `战争`
  (war), `动画` (animation), `动作` (action), and others
- **year** — release year
- **rating** — 0 to 10
- **box_office_cny_m** — box office in millions of CNY

## Query ideas

- Count or total box office per `genre`
- Sort by rating, highest or lowest
- All films from one year, or the top N
- Films above a box-office threshold
