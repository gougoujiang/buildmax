# Sample Data

Ready-made datasets to give the agent something real to work on, in either
surface:

- **Portal** — upload them into a team workspace so a fresh deployment has
  content to reason about from the first conversation. Files page →
  **Upload Folder**, which preserves the directory structure.
  [deploy/compose.md](../docs/deploy/compose.md#put-something-in-the-workspace)
  walks through it.
- **CLI** — point a run at one of them directly:
  `buildmax --workspace sample-data/access_log`.

Each subdirectory holds a CSV plus a README describing its columns, so the agent
has both the data and its schema. Between them they exercise `Read`, `Grep`,
`Glob`, and `Edit` over tabular data, nested directories, and prose.

**Nothing in the project depends on this directory.** No test, build step, or
release archive references it — it is here so that trying BuildMax does not
start with "first, find some data". Deleting it breaks nothing.

The data is deliberately Chinese-language in most files — product names,
categories, city names. That is intentional: it keeps the fixtures honest about
multibyte text, which is where naive line, column, and offset handling tends to
break. The documentation is in English, and every README lists the literal
values you need in order to write a query.

## Scenarios

| Directory | Scenario | Main file | Typical use |
|---|---|---|---|
| [access_log](access_log/) | Web access log | access_log.csv | Counts by path or status code, error rate, slow requests |
| [books](books/) | Book catalog | books.csv | Filter by author, category, or price; low-stock alerts |
| [employees](employees/) | Employees / HR | employees.csv | Counts by department, position, or tenure |
| [expenses](expenses/) | Personal expenses | expenses.csv | Totals by category or month, spending trends |
| [fitness](fitness/) | Workout log | fitness.csv | Weekly and monthly totals, activity mix, calories |
| [grades](grades/) | Student grades | grades.csv | Stats by class or subject, failing lists |
| [inventory](inventory/) | Stock / warehousing | inventory.csv | Low-stock alerts, totals by warehouse |
| [meetings](meetings/) | Meetings / calendar | meetings.csv | Conflict detection, one person's schedule, room usage |
| [movies](movies/) | Films | movies.csv | Rankings, genre mix, top N by year |
| [orders](orders/) | E-commerce orders | orders.csv | Filter by status, daily or monthly totals, refund rate |
| [recipes](recipes/) | Recipes | recipes.csv | Find dishes by ingredient, filter by difficulty or cuisine |
| [sales](sales/) | Sales | sales_data.csv, sales/ | Volume and revenue by region or product |
| [survey](survey/) | Survey responses | responses.csv | Option distribution, cross-tabulation, completion rate |
| [tasks](tasks/) | Project tasks / board | tasks.csv | Overdue tasks, counts by assignee or project, status flow |
| [weather](weather/) | Weather records | weather.csv | City comparison, temperature trends, precipitation |

The top level also holds plain-text samples such as `shakespeare.txt`, useful
for exercising `Read` and `Grep` on prose rather than tabular data.
