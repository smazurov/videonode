---
name: release-changelog
description: Prepend a new release entry to videonode's chglog-driven changelog.yml. Use when user asks to "release", "prepare a changelog", "cut rcN", "tag vX.Y.Z", or otherwise add a new version to changelog.yml. The skill enforces themed grouping — it does not dump git log into the file.
---

# release-changelog

The repo's `changelog.yml` feeds `chglog` to build the Debian changelog and the docs site changelog. New releases are prepended to the top of the file.

## Schema

```yaml
- semver: 4.0.0-rc3
  date: 2026-05-27T00:00:00Z
  packager: Stepan Mazurov <smazurov@gmail.com>
  changes:
    - commit: <short-sha>
      note: "<themed one-liner>"
  deb:
    urgency: low
    distributions:
      - stable
```

Date is the current date in UTC, midnight. Packager copies from the previous entry.

## Workflow

1. Read `changelog.yml` to find the previous top entry's commit references.
2. Resolve the previous release boundary: `git log <last-commit-in-prev-entry>..HEAD --oneline` for the candidate range. If the user names a skip (e.g. "rc3 skipping rc2"), still diff against the last entry actually written in the file.
3. **Group commits by theme**, not by file or by conventional-commit type. A theme is a coherent piece of work a reader cares about ("docs site", "release pipeline rewrite", "auth hardening + auto-start", "composer UI tweaks"). Multiple commits collapse into one entry.
4. For each theme, pick **one representative commit** as the anchor sha and write a single substantive note. Notes describe what changed in human terms, not the commit subject.
5. Prepend the new entry above the previous one. Preserve blank-line separation between entries.

## The terseness rule

Terseness is at the **group level, not the line level**.

- Wrong: one bullet per commit, copying the commit subject. That's just `git log` in YAML.
- Wrong: ultra-short bullets like `"ui: rotate buttons"` that strip the substance.
- Right: 4–8 themed bullets across the whole release, each substantive enough to tell a reader what shipped.

Two UI commits both about the composer canvas → one composer-UI bullet. A CI commit plus a packaging commit that together implement the new release pipeline → one release-pipeline bullet.

## Anti-patterns

- Dumping `git log --oneline` into `changes:`.
- Splitting one theme across multiple bullets to look thorough.
- Bullets that only restate the conventional-commit prefix (`ci: foo`, `fix: bar`).
- Inventing dates — always use today's UTC date for new entries.
- Editing or renumbering prior entries unless the user explicitly asks.

## Example

A release covering ~30 commits collapsed into 5 themed bullets:

```yaml
- semver: 4.0.0-rc3
  date: 2026-05-27T00:00:00Z
  packager: Stepan Mazurov <smazurov@gmail.com>
  changes:
    - commit: cdd37c1
      note: "VitePress docs site with Mermaid, search, versioned navbar"
    - commit: feff078
      note: "release pipeline rewrite: matrix build replaces goreleaser, chglog-driven .deb"
    - commit: f62e5a3
      note: "shadow-group-gated auth + postinst enrollment, service auto-start on install"
    - commit: 74749ac
      note: "composer UI: auto-slot on input add, canvas rotation moved to inspector"
    - commit: 16f5389
      note: "source reports placeholder dims while V4L2 inactive"
  deb:
    urgency: low
    distributions:
      - stable
```

The bullets reflect the work, not the log.
