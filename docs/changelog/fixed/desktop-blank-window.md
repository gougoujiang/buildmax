- The desktop app no longer opens on a blank window. Its frontend bundle shipped
  two copies of React — one reached through the symlinked `@buildmax/gui`
  package — so the first hook of the first render threw and nothing was drawn.
