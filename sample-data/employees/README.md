# Employees / HR Example Data

A staff roster, for counting and filtering by department, position, tenure, and
salary band.

## Files

| File | Contents |
|---|---|
| `employees.csv` | Employee ID, name, department, position, hire date, salary band |

## Columns

- **employee_id** — employee number
- **name** — employee name
- **department** — one of `研发` (engineering), `产品` (product), `设计`
  (design), `市场` (marketing), `人事` (HR)
- **position** — job title, for example `后端工程师` (backend engineer)
- **hire_date** — date joined
- **salary_band** — `A`, `B`, or `C`, where `A` is highest

## Query ideas

- Headcount per `department`
- Filter by position or salary band
- Compute tenure from `hire_date`
- List everyone in a given department
