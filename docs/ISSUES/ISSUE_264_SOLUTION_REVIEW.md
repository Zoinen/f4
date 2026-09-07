# Issue #264 solution review

The report combines two symptoms: manual builds can be compared as if their
commit hash were a release version, and declining an update could suppress a
later manual check. The second behavior is already covered by the merged
fix for #374; this change addresses the remaining version-comparison bug.

## Candidate 1: compare the manual build string with the release tag

This is the current behavior. A commit hash, a development label, and a
semantic version are different value types, so any unequal strings look like
an available update. It can ask a newer local checkout to install an older
release and is rejected.

## Candidate 2: disable stable update checks for manual builds

This avoids false downgrade prompts, but also hides a real update when a user
builds from an older commit. It is not acceptable for a development build.

## Candidate 3: compare VCS/build time with release publication time (chosen)

Release-tagged builds continue to use exact tag equality. A manual build that
only exposes a commit hash is compared with the release PublishedAt timestamp:
newer releases are offered, while older releases are not. Missing or malformed
metadata keeps the previous safe behavior of offering the release.

## Three-pass review

1. Exact release builds retain tag-based behavior; manual builds no longer
   compare unlike identifiers.
2. An older checkout still receives an update prompt, while a newer local
   checkout cannot be downgraded accidentally.
3. Missing metadata remains conservative, and the existing #374 session-level
   decline behavior is untouched. A focused regression test covers newer and
   older release timestamps plus exact release equality.
