- Entity identifiers are now opaque and unprefixed: `ivyoh5qcfu6ypfkhyedq`
  rather than `t_9f3k2m8x1qwe7rt4zy0p`. Each is 96 bits of crypto-random data
  written as 20 lowercase base32 characters, case-insensitive on input and safe
  unchanged in a URL, a filename, a Kubernetes name, and a shell. The type
  prefix is gone because nothing dispatched on it — a route, a JSON field, and a
  column already name the type — and because it was the only thing standing
  between a presentation choice and the database schema. Agent and workflow
  revisions, catalog plugins, and plugin releases carry no identifier at all
  now; they are addressed by their parent plus a revision number, by name, and
  by name plus version. Login-chain, trace-file, and Desktop-project identifiers
  keep their prefixes: none of them names a database row. **This is a breaking
  change with no compatibility path.** Every existing identifier, access token,
  refresh session, worker token, Portal link, bookmark, and stored object key is
  invalid. Recreate the database and object store rather than upgrading.
