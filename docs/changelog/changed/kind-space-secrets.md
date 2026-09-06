- The local `kind` stack now generates an ephemeral Space Secret key-encryption
  key and mounts it, so the Secrets feature can be exercised end to end there
  instead of answering "secrets not configured"; the deployment baseline mounts
  the key from an optional Secret so other deployments are unaffected.
