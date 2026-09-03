- Fixed a regression on deployments using the local-filesystem artifact backend
  (including the default Docker Compose stack) where a finished task run's stored
  result was truncated to empty and its artifact content came back blank.
