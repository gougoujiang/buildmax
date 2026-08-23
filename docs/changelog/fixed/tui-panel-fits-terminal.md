- A TUI panel such as `/model` or `/tools` now trims its list to what the
  terminal can show. It listed a fixed number of rows whatever the height was,
  so on a short terminal the panel pushed the input box and the footer off the
  top of the screen, and nothing could scroll them back. Panel lines also no
  longer wrap inside the panel border, which was quietly doubling the height of
  the `/tools`, `/skills`, and `/diff` panels.
