- A worker reports what the server actually said when a call to the worker API
  fails, instead of only the HTTP status. A run that could not be claimed or
  patched used to log "500 Internal Server Error" and discard the sentence
  explaining why.
