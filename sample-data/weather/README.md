# Weather Records Example Data

Daily weather observations for several cities, for comparisons, temperature
trends, and precipitation statistics.

## Files

| File | Contents |
|---|---|
| `weather.csv` | Date, city, high, low, precipitation, conditions |

## Columns

- **date** — observation date
- **city** — `北京` (Beijing), `上海` (Shanghai), `广州` (Guangzhou)
- **temp_max** — daily high in °C
- **temp_min** — daily low in °C
- **precipitation_mm** — precipitation in millimetres
- **conditions** — one of `晴` (clear), `多云` (cloudy), `阴` (overcast),
  `小雨` (light rain), `中雨` (moderate rain), `雷阵雨` (thunderstorms)

## Query ideas

- Temperature trend for one city over a period
- Compare temperature or rainfall across cities
- Days with rain, where `precipitation_mm > 0`
- Count days by `conditions`
