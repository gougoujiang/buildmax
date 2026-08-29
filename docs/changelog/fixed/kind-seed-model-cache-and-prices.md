- `./make kind seed` reads a model's `cache_control` and `pricing` blocks and
  passes them to the catalog, so a seeded deployment answers with the same
  cache policy and rates as the local settings it was seeded from. It no longer
  reads the removed `prompt_cache` key, whose flag the catalog command dropped —
  a settings file that still carried it failed the whole seed.
