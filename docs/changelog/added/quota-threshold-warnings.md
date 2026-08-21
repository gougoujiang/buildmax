- Quota now says something before it starts refusing work. Passing 80% of a
  space's run or token limit records `quota.threshold_reached`, and work
  refused at the limit records `quota.exceeded` — at most once per limit per
  period, so a space that keeps submitting does not fill its own trail with
  retries. The actor is the deployment, not whoever submitted the work that
  tipped the total over. Portal states the same thing in space settings, and a
  space with no limits reports nothing rather than reporting comfort.
