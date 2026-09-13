- A Workflow step can now take an earlier step's output as input. A step
  declares `bindings` naming a prior step, and the run feeds that step's full
  output to the downstream Agent as labelled, untrusted context — so a
  multi-step Workflow can pass one Agent's result to the next.
