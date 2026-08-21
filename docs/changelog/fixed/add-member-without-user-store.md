- Adding a member to a team on a deployment with no user store answers 503
  rather than 500. Every other "not configured" answer in the API is 503; this
  one was the outlier.
