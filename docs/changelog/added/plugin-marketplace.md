- Private plugin Marketplace: a System Administrator publishes a plugin
  directory with `buildmax plugin publish`, and anyone signed in to that
  deployment installs it by name with `buildmax plugin install`. Releases are
  immutable and identified by version and SHA-256 digest, which the client
  verifies against the catalog before anything is unpacked.
