- A daily release check now prepares a reviewable pull request once the latest
  alpha is at least 72 hours old and user-visible changes are waiting. Merging
  that pull request creates the version tag and starts the existing publication
  workflows; an empty or unreviewed release is never published on the timer.
