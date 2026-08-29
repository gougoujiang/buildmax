- A quota limit that cannot be read now refuses the work instead of admitting
  it. A failed team, tier, or usage lookup was reported as "allowed", so a
  deployment whose database was unreachable served unmetered runs and managed
  inference and recorded nothing about having done so. Such a failure is a 500
  naming the read that failed, distinct from the 429 an over-quota team gets;
  a team with no record, no tier, or a tier that names nothing is still
  admitted, because absence of a limit is not the same as not knowing. Team
  usage reports the same way rather than showing a zeroed snapshot.
