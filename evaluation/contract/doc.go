// Package contract defines the BuildMax-owned evaluation contract: the task,
// subject, trial, grader, and experiment shapes every runner, adapter, and
// viewer agrees on.
//
// It depends on the standard library alone. That is a decision rather than an
// accident: docs/design/evaluation-system.md section 15.3 makes a
// standard-library Go controller the default so an operator can qualify a
// deployment from one binary, and so evaluation never adds a dependency to the
// product's go.mod.
//
// Nothing here imports the BuildMax runtime. A trial bundle is qualification
// evidence that outlives the revision which produced it, so the format has to
// stay readable without a BuildMax process.
package contract

// Version is the contract generation stamped on every task, subject manifest,
// trial bundle, and experiment.
//
// A reader meeting a version it does not know must refuse the file rather than
// interpret the fields it happens to recognise. A partial read of qualification
// evidence produces a confident wrong answer, which is worse than no answer.
const Version = 1
