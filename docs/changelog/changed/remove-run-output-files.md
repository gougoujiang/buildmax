- Removed the per-run "output files" list. A task run's reply is still recorded
  and shown as its result; files a run means to keep are published with
  `UploadArtifact` and appear as the space's artifacts, and a Task's working
  files are recovered through its workspace checkpoint rather than downloaded
  file by file.
