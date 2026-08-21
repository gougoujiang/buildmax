- On Windows, a command that read the saved login while another one was renewing
  it could fail with "the process cannot access the file because it is being
  used by another process". Replacing `auth.json` is a rename, which a
  concurrent reader survives on macOS and Linux but not on Windows; reads and
  the replacement are now serialized within a process, so a run that renews its
  token no longer trips another caller in the same binary.
