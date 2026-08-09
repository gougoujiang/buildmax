---
id: 002-add-function
title: Add a new function to an existing file
timeout: 120
---

The file `greet.go` contains a `Greet(name string) string` function.

Add a new function `GreetAll(names []string) string` to `greet.go` that:
- calls `Greet` for each name in the slice
- joins all the greetings with a newline character (`\n`)
- returns the joined string

The test file `greet_test.go` already has a `TestGreetAll` test — make it pass.

---grader---

go test -run TestGreetAll .
