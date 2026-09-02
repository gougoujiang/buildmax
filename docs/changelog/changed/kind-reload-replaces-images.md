- `./make kind reload` replaces `./make kind images`: it still builds and loads
  the local images, and now also restarts the `buildmax-server` and
  `buildmax-portal` deployments so a code change takes effect without a full
  `./make kind up`.
