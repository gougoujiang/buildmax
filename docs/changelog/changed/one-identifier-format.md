- Login-chain, trace-file, and Desktop-project identifiers lost their `as_`,
  `rt_`, and `p_` prefixes and are now ordinary opaque IDs, leaving one
  identifier format in the codebase. None of the three is read by a person or
  dispatched on; they had kept a prefix only because they are not database
  rows, which was where the previous change stopped rather than a reason to
  keep one. Background jobs are the exception and keep `jb_`: a job ID reaches
  the model as a bare string inside tool output, and free prose is the one
  place a type prefix says something the surrounding context does not.
