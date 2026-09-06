- Portal task runs now execute in a single `workspace/` directory — the agent's
  working directory, holding the space's files — instead of splitting them into
  separate read and output directories. Files an agent means to keep are
  published with `UploadArtifact`; the run's reply is still recorded as its
  result.
