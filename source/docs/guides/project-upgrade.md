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

## Project workflow suggestions

Upgrade inspection also recognizes known adoption patterns that can make
ordinary work unnecessarily expensive: checking the latest release before each
source change, inspecting every focused group, and wrappers that may reject
partial staging. The automation component includes the affected file, the
reason to review it, and proposed replacement policy or command text. These are
heuristic suggestions, not proof that arbitrary instructions or scripts are
incorrect. Existing strict enforcement and receipt requirements still apply.

Prefer trusting the pinned version during ordinary work, scheduling upgrades
separately, and deeply reviewing a bounded shortlist while retaining the complete
bounded report and disclosing unreviewed findings. A shortlist does not authorize
acceptance of its unreviewed remainder. For an obsolete staged wrapper, consider
`mori review staged check .` after verifying equivalent project enforcement.

Inspection reads conventional workflow files, Mori-named scripts, tracked-hook
locations (`.githooks/*` and `.husky/pre-commit`), and root `AGENTS.md`. It does
not execute scripts or follow symlinked files or parent directories. Content
inspection is bounded to 128 files of at most 1 MiB each; skipped or unreadable
regular files are reported. Custom locations and unfamiliar instruction wording
can require manual review. These advisory findings do not change the managed
compatibility gate, and `--apply` never rewrites project instructions or hooks.
The upgrade-plan JSON remains version 2 with its existing component fields.
