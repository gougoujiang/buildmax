- `./make build desktop` packages the Wails desktop app on its own, without
  spending the server, worker, and Portal builds to get at it. CI now runs it
  on macOS and Windows after a merge that touches the app, weekly, and on
  demand: nothing built the packaged app before, so a break in the asset
  embedding, the Wails configuration, or the native link waited for whoever ran
  `./make build` next.
