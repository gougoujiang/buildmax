- A team can activate published plugin releases for its background runs, pinned
  to an exact version and digest. A team either curates that list or leaves it
  open, in which case naming a plugin in an agent activates it. Releases
  contributing hooks or MCP servers cannot be activated yet.
  `buildmax plugin activations --team <id>` reads what a team activated.
