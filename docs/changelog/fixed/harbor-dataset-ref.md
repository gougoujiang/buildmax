- Fix the documented Terminal-Bench run command, which named the dataset without
  its pinned ref. Harbor resolves a bare name to `latest` while the importer
  stamps the pinned digest on every bundle it writes, so a run started from the
  README filed its evidence under a dataset version it had not measured. The run
  command and the reproduction command recorded on each bundle are now built by
  one function, so neither can drift from the pins again.
