# Survey Responses Example Data

Survey answers in long format — one row per question per respondent — for
option distributions, cross-tabulation, and completion rates.

## Files

| File | Contents |
|---|---|
| `responses.csv` | Response ID, question ID, question text, selected option, respondent |

## Columns

- **response_id** — response record ID
- **question_id** — `Q1` through `Q4`
- **question_text** — the question as shown to the respondent
- **option** — the chosen option; semicolon-separated when multiple choice
- **respondent** — respondent name, possibly anonymous

## Query ideas

- Count each option per question, for a distribution
- Average score for a rating question such as `Q3`
- Cross-tabulate two questions, for example the share of people who chose Go
  who also answered yes
- Completion rate, by counting distinct `respondent` values
