# Mori documentation

Start with the guide that matches what you are trying to do.

## Learn Mori

- [Getting started](getting-started.md) — install, first scan, profiles, and
  coverage checks.
- [Reviewing results](guides/reviewing-results.md) — selection, focus,
  prioritization, statement blocks, and source inspection.
- [How scoring works](scoring.md) — normalization, feature bags, similarity,
  identities, and explanatory evidence.

## Use Mori in a project

- [Project configuration](configuration.md) — every `.mori.json` field and
  ignore-file behavior.
- [SQL and embedded SQL](guides/sql.md) — generic SQL, PostgreSQL, and opt-in
  embedded queries.
- [Automation and baselines](guides/automation-and-baselines.md) — coverage
  policy, changed-file focus, CI exits, and reviewed suppression.
- [Editors and coding agents](guides/editors-and-agents.md) — SARIF,
  unsaved-buffer overlays, VS Code, and the bundled Agent Skill.
- [Project contract and upgrades](guides/project-upgrade.md) — managed-asset
  compatibility, safe migration, and the stable pre-commit adapter.

## Reference and development

- [Languages and parser limits](reference/languages-and-parser-limits.md)
- [Scan selection](scan-selection.md)
- [Machine integration](machine-integration.md)
- [Architecture](architecture.md)
- [Adding a language](adding-a-language.md)
- [Contributing](../CONTRIBUTING.md)

Mori's results are structural review evidence. They do not prove semantic or
behavioral equivalence, defects, or that a refactor is safe.
