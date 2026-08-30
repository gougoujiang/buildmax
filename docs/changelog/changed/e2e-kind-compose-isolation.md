- `./make e2e local` now picks a fresh Compose project name and ports for
  every run instead of a fixed one, so it never collides with a contributor's
  persistent stack or another concurrent run. `./make compose up`/`kind up`
  can also be pointed at a second, differently named and ported stack with
  `BUILDMAX_COMPOSE_PROJECT`/`BUILDMAX_KIND_CLUSTER` and matching port
  variables, so multiple deployments can run side by side.
