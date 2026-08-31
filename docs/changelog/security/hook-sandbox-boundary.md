- `command` and `http` hooks now run through the same sandbox confinement
  that already applies to `Bash` and `WebFetch`, instead of reaching a shell
  or the network unconstrained regardless of the sandbox being enabled.
