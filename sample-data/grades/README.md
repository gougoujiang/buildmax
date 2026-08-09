# Student Grades Example Data

Exam results, for statistics by class or subject and for finding failing
students.

## Files

| File | Contents |
|---|---|
| `grades.csv` | Student ID, name, class, subject, score, exam date |

## Columns

- **student_id** — student number
- **name** — student name
- **class** — `高一(1)班` or `高一(2)班` (grade 10, classes 1 and 2)
- **subject** — one of `语文` (Chinese), `数学` (maths), `英语` (English),
  `物理` (physics), `化学` (chemistry)
- **score** — 0 to 100
- **exam_date** — date of the exam

## Query ideas

- Mean and maximum score per class
- Students failing a subject, `score < 60`
- All subjects and the total for one `student_id`
- Filter by exam date
