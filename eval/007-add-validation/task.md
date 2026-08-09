---
id: 007-add-validation
title: Add input validation to a registration function
timeout: 150
---

The file `register.go` contains a `Register` function that currently performs
no validation and always returns nil.

Read `user.go` to understand the sentinel errors and `RegisterRequest` fields,
then add validation to `Register` following these rules (in this order):

1. `Name` must not be empty → return `ErrEmptyName`
2. `Email` must contain `@` and `.`, and must not start with `@` → return `ErrInvalidEmail`
3. `Password` must be at least 8 characters → return `ErrWeakPassword`

Do not modify `user.go` or `register_test.go`. Only edit `register.go`.

---grader---

go test .
