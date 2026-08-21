- An MCP server that returns an image — a screenshot, a rendered chart — now
  sends the model the image instead of a base64 blob pasted into the tool
  result. Set `vision: true` on a model that can read images, or `--vision` on a
  catalog model. Left off, which is the default, the tool result says what came
  back (`(image: image/png, 43.2 KB)`) and the image is not sent, because a
  model without image support rejects such a request rather than ignoring it.
