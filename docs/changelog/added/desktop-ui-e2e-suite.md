- Add `./make e2e desktop-ui`, which drives desktop/frontend's React app and
  its bound Go methods through `wails dev`'s browser dev server against a
  fresh, discarded `BUILDMAX_HOME`; `./make run desktop-dev` starts that same
  dev server for ad hoc use, and `.buildmax/skills/drive-desktop/` is a
  Playwright REPL for poking at it by hand.
