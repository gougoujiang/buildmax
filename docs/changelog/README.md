# Changelog Entries

> **Audience:** contributors · **Status:** current

One unreleased entry per file. A release folds them into a dated section in
[CHANGELOG.md](../../CHANGELOG.md) and empties this directory.

## Why files instead of one list

`CHANGELOG.md` had a single `Unreleased` section, and every branch that needed
an entry appended to the end of the same subsection. Appending rather than
inserting was already a concession to that: two branches inserting at the top
conflict every time. Appending only moves the collision — two branches still add
adjacent lines at the end of the same section, and git still asks a human which
order they go in. A six-part stack hit it twice in one afternoon.

A file per entry has no shared line to conflict on. Two branches conflict only
if they choose the same filename, which means they are describing the same
change.

## Adding one

```bash
./make changelog new fixed request-id-header
```

That writes `docs/changelog/<category>/<slug>.md`, where category is one of
`added`, `changed`, `fixed`, or `security` — the headings a release section
uses. It refuses to overwrite an existing entry: two branches choosing the same
filename are describing the same change.

The file holds the entry exactly as it will appear: a Markdown list item,
wrapped, with continuation lines indented two spaces.

```markdown
- The server answers every request with an `X-Request-Id` header, and its logs
  now record each request when it finishes rather than when it starts.
```

The slug names the change, not the branch or the pull request: a reader
scanning this directory should be able to tell what is unreleased without
opening anything. `request-id-header.md`, not `pr-116.md`.

An entry is for anything a user or operator would notice: new or changed
behavior, new configuration, removals, and fixes to released behavior. Internal
refactors, test-only changes, and documentation edits do not need one.

## Reading and releasing

```bash
./make changelog          # print the unreleased entries, grouped, as they will appear
./make changelog release 0.1.0-alpha.2
```

`release` writes the section into `CHANGELOG.md` under the version and today's
date, moves the `[Unreleased]` compare link onto the new version and adds that
version's own link beneath it, then deletes the files it folded in. It refuses a
version the file already links, so a second fold cannot append a duplicate
section. Order within a category follows the
filename, which is arbitrary — release preparation is where entries get ordered
for readers.

That section is also the GitHub Release body: `./make release notes <version>`
composes it with the install and alpha text around it, and the release workflow
publishes the result. An entry nobody wrote is a change nobody announces.
