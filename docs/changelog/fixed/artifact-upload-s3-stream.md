- Fixed artifact uploads failing with an internal error on deployments whose
  artifact storage is an S3-compatible store reached over plain HTTP, such as
  the bundled MinIO: the streamed upload is now sent through the S3 transfer
  manager instead of a single request the SDK cannot sign.
