- The TUI `/model` panel scrolls. With more models configured than fit the
  terminal it listed the first few and a `… N more` row, while the arrow keys
  kept moving a selection through the ones it was not showing — so the models
  past the fold could be switched to but never seen. The list is now a window
  that follows the selection, and the panel opens on the model in use rather
  than at the top.
