- The `/rewind` and `/fork` pickers no longer offer an assistant message that
  asked for a tool. Choosing one left the conversation holding a tool call with
  no result, which OpenAI and Ollama refuse; the picker now offers the reply
  that ended each turn.
