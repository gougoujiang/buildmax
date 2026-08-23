- Closing a TUI panel such as `/model` no longer leaves its box drawn above the
  input. The terminal renderer the CLI depends on stopped erasing the lines a
  shrinking frame vacates, so every dismissed panel stayed on screen; the
  dependency is pinned back to a working revision.
