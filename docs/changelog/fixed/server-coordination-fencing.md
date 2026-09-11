- On a multi-replica Server, a conversation turn whose coordination lease
  expired and was taken over can no longer write behind the new holder: message
  writes now carry the lease's fencing token and a stale one is rejected.
