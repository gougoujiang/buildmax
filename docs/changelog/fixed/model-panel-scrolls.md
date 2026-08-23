- The TUI `/model` and `/tasks` panels scroll. With more entries than fit the
  terminal each listed the first few and a `… N more` row, while the arrow keys
  kept moving a selection through the ones it was not showing — so a model past
  the fold could be switched to, and a job past it stopped with `s`, without
  ever being seen. Both lists are now a window that follows the selection, and
  `/model` opens on the model in use rather than at the top.
