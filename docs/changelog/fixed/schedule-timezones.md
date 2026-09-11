- Schedules now accept any IANA timezone (for example `Asia/Shanghai`), not only
  `UTC`: the server binary embeds the timezone database, so a named zone resolves
  in container deployments that ship no system zoneinfo.
