- Fixed TUI shutdown so Ctrl+C cancels and joins the active agent run instead
  of leaving stream senders or background goroutines behind.
