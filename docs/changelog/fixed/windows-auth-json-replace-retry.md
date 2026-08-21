- Refreshing credentials now retries a short-lived Windows file-sharing
  conflict while atomically replacing `auth.json`. Without that retry, a
  concurrent caller could read the old refresh token after a successful
  exchange and spend it a second time.
