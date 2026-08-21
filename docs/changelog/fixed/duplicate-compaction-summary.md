- The system prompt no longer carries the compaction summary twice. Both the
  caller and the agent loop appended it, so every run after its first in-run
  compaction sent two copies.
