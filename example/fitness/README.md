# Workout Log Example Data

An exercise log, for weekly and monthly rollups, activity mix, calorie totals,
and goal tracking.

## Files

| File | Contents |
|---|---|
| `fitness.csv` | Date, activity type, duration in minutes, calories, note |

## Columns

- **date** — date of the session
- **activity_type** — one of `跑步` (running), `力量` (strength), `游泳`
  (swimming), `骑行` (cycling), `瑜伽` (yoga), `休息` (rest day)
- **duration_min** — duration in minutes
- **calories** — estimated calories burned
- **note** — free-text note

## Query ideas

- Sessions or total minutes per week or month
- Distribution across `activity_type`
- Total or daily-average calories
- Ratio of rest days to training days
