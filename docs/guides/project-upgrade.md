# Project contract and upgrade

`mori project upgrade` uses `.mori-project.json` as the tracked Mori-specific
contract. It is separate from `.mori.json`, which remains project-owned scan
policy. The contract records the Mori version, embedded Agent Skill revision
and digest, the hook-contract revision and digest, configuration, report,
review-receipt and baseline schema revisions, and normalization version.

`--check` and `--dry-run` are read-only. Required managed drift or a managed
conflict causes exit 5; policy, protected baseline evidence, and project-owned
automation are reported as advisory or manual findings. `--apply` writes only the version
pin, contract, and a missing/current/recorded-official Agent Skill. Unknown
skill content is never overwritten. Existing managed files receive recoverable
sibling backups before replacement.

Apply does not stage or commit. When the Mori update itself will be committed,
inspect and include the updated pin, project contract, and managed skill
package together so a fresh checkout receives the same contract. Keep backup
paths local and out of the commit.

When either `.mori-version` or `.mori-project.json` opts a project into this
contract, `scan` and `review` run a cheap local compatibility gate before
analysis. Required managed drift and Mori-managed conflicts stop with exit 5
and point to `project upgrade --dry-run`; `--no-config` does not bypass this
gate. The check never contacts GitHub or another network service. Projects
without either marker keep standalone behavior.

`mori hook pre-commit` is the canonical staged-check entrypoint for
project-owned hooks. It has the same fixed staged inclusion and coverage
contract as `mori review staged check`. Set
`MORI_STAGED_REVIEW_RECEIPT=1` to validate the default private Git metadata
receipt (`mori/staged-review.json`); unset or empty performs an unacknowledged
check, and any other value is rejected.

## Maintainer contract

A release that changes the embedded skill, canonical hook behavior, config or
evidence schemas, or normalization must update the desired project contract
and its lifecycle tests. Contract-schema changes must keep strict migration
support for supported prior official contracts; an official managed project
must not become a local-customization conflict solely because Mori was
upgraded.
