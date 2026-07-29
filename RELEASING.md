# Releasing

Releases are tag-driven so binaries exist before a GitHub release becomes
public.

## Preconditions

1. Update `CHANGELOG.md`.
2. Confirm every intended change is committed.
3. Run `make check`.
4. Confirm the default branch CI is green.
5. Choose a strict Semantic Versioning tag such as `v0.1.0`.

## Publish

Create and push a signed tag:

```sh
git tag -s v0.1.0 -m "v0.1.0"
git push origin v0.1.0
```

The release workflow:

1. validates the tag;
2. reruns the full verification gate;
3. tests and builds on five native OS/architecture runners;
4. creates `.tar.gz` archives for Unix-like systems and `.zip` for Windows;
5. generates `checksums.txt`;
6. creates or reuses a draft GitHub release;
7. attaches every asset; and
8. publishes the draft.

A tag with a prerelease component, such as `v0.1.0-rc.1`, is published as a
GitHub prerelease and is never marked Latest.

Do not publish an empty release manually before pushing the tag. The workflow
uses a draft so all assets are present before immutable-release protections
apply.

## Verify

After the workflow completes:

- confirm the release is no longer a draft;
- confirm five archives plus `checksums.txt` are present;
- compare at least one downloaded asset with its checksum;
- run `mori version` from a release archive; and
- inspect workflow logs for native test failures or skipped jobs.

If a published immutable release is wrong, create a new patch release. Do not
move or reuse its tag.
