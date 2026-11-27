# Changelog entries

Each change that users will notice adds a file to this directory instead of editing
[CHANGELOG.md](../CHANGELOG.md). Separate files never conflict, so pull requests can be merged in any
order. When a version is released, the entries move into CHANGELOG.md and the files are removed.

## Writing an entry

Name the file after the change, for example `romanian-comma-below.md`, and use the sections of
[Keep a Changelog](https://keepachangelog.com/en/1.1.0/) with the same layout as in CHANGELOG.md:

```markdown
### Fixed

- `Encoder.Encode` maps Romanian ș and ț to the cedilla letters of the Romanian national option sub-set
```

- The sections are `### Added`, `### Changed`, `### Deprecated`, `### Removed`, `### Fixed` and
  `### Security`. A file can have several.
- Keep each entry to one or two lines that say what changed for users and name the affected API or
  tool. Indent continuation lines by two spaces. End the entry of a breaking change with
  "This is a breaking change".
- A pull request with several changes can add a file per change, or one file with all its entries.
- Changes that users don't notice, such as tests, CI and refactoring, need no entry.

[`scripts/check-changelog.sh`](../scripts/check-changelog.sh) checks the files. It runs in CI, with
`make check`, and as a pre-commit hook when CHANGELOG.md or this directory changes.

## At a release

The pull request that prepares release X.Y.Z

1. adds `## [X.Y.Z] - YYYY-MM-DD` below `## [Unreleased]` in CHANGELOG.md, with the entries of all
   files here under one heading per section, in the order Added, Changed, Deprecated, Removed, Fixed,
   Security. Entries can be edited on the way, for example to merge related ones;
2. removes the entry files, so that only this README remains;
3. updates the links at the end of CHANGELOG.md: `[X.Y.Z]` to the release and `[Unreleased]` to the
   comparison from vX.Y.Z to HEAD.

While the latest version in CHANGELOG.md has no tag, the check fails if this directory holds entry
files. Tag `vX.Y.Z` only on a commit where the check passes, and run the check with the tag as
argument, `scripts/check-changelog.sh vX.Y.Z`, as a release workflow should.
