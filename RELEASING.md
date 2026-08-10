# Releasing

Releases are tag-driven so binaries exist before a GitHub release becomes
public.

## Preconditions

1. Update `CHANGELOG.md`.
2. Confirm every intended change is committed.
3. Run `make check`.
4. Confirm the default branch CI is green.
5. Choose a strict Semantic Versioning tag such as `v0.2.1`.

## Publish

Create and push a signed tag:

```sh
git tag -s v0.2.1 -m "v0.2.1"
git push origin v0.2.1
```

The release workflow:

1. validates the tag;
2. reruns the full verification gate;
3. tests and builds on five native OS/architecture runners;
4. creates five native archives and one portable Agent Skill ZIP;
5. packages the VS Code client and versioned JSON Schema;
6. generates checksum-pinned Homebrew, Scoop, and WinGet manifests;
7. generates an SPDX source-dependency SBOM and `checksums.txt` for every asset;
8. signs provenance, checksum-manifest, and native-archive SBOM attestations
   through GitHub and Sigstore;
9. creates or reuses a draft GitHub release and attaches every asset;
10. publishes the draft; and
11. verifies checksums, attestations, the portable Agent Skill, and every
    downloaded native binary.

A tag with a prerelease component, such as `v0.2.1-rc.1`, is published as a
GitHub prerelease and is never marked Latest.

Do not publish an empty release manually before pushing the tag. The workflow
uses a draft so all assets are present before immutable-release protections
apply.

## Verify

After the workflow completes:

- confirm the release is no longer a draft;
- confirm the native and portable archives, VSIX, schema, package-manager
  manifests, SPDX SBOM, and `checksums.txt` are present;
- verify all archive checksums;
- run `mori version`, `mori languages`, and `mori skill install` from release
  archives;
- compare the portable Agent Skill with the tagged source; and
- inspect workflow logs for native test failures or skipped jobs.
- verify representative downloaded assets with
  `gh attestation verify ASSET --repo Cyberlane/mori`.

If a published immutable release is wrong, create a new patch release. Do not
move or reuse its tag.
