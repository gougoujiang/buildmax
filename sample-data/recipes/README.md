# Recipes Example Data

A recipe list, for looking up dishes by ingredient, filtering by cuisine or
difficulty, and sorting by cooking time.

## Files

| File | Contents |
|---|---|
| `recipes.csv` | Dish name, ingredients, category, difficulty, cook time |

## Columns

- **dish_name** — name of the dish
- **ingredients** — semicolon-separated main ingredients
- **category** — one of `家常` (home cooking), `川菜` (Sichuan), `粤菜`
  (Cantonese), `素菜` (vegetarian), `凉菜` (cold dishes), `主食` (staples)
- **difficulty** — `easy` or `medium`
- **cook_minutes** — cooking time in minutes

## Query ideas

- Find dishes using an ingredient — `grep` the `ingredients` column
- Filter by category or difficulty
- Quick meals, where `cook_minutes` is below a threshold
- Sort by cooking time
