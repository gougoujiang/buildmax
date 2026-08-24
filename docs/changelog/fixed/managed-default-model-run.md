- A background run on a deployment that leaves `worker.llm.model` unset reaches
  the deployment's default model again. The run was assembled with no model at
  all and failed with `model not found: ""`; naming a model in `worker.llm` was
  the only way around it.
