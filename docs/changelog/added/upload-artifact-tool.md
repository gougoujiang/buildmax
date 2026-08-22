- Agents can publish a finished file with the `UploadArtifact` tool and cite the
  `ar_` reference in their answer. It appears only where there is a server to
  publish to — a logged-in CLI or Desktop session, or a worker running a team's
  task — so a session running straight against a model provider is not offered
  a tool that could only fail. Nothing is uploaded automatically: the agent
  names the one file, which is what keeps `.env` files, caches, and intermediate
  output out of a team's artifacts. What a run publishes shows up on its issue
  alongside the run's own output.
