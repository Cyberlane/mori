# Actionability calibration fixtures

These ten original MIT fixtures model review situations encountered in large
repositories. They contain no copied target-project source. Labels are provisional
Mori development judgments, not acceptance by the maintainers of any public
project. In particular, `false-positive` means a poor consolidation lead in this
fixture's stated context; it does not mean Mori incorrectly measured structure.

Run independently from the historical sixteen-case corpus:

```sh
go run ./internal/cmd/corpuseval corpus/actionability
```

The separate manifest preserves the original corpus's ranking reference. Its
normalization-14 score windows record the observed abstraction, including known
high-scoring negatives; they are regression checks, not desired precision targets.
The evaluator's precision/actionability metrics describe only these provisionally
labeled pairs, not production search precision or a maintainer review outcome.

| Case | Provisional interpretation |
| --- | --- |
| suite-scaffolding | Unrelated account and image tests share a wrapper; nested test intent differs. |
| duplicated-test-helper | Repeated response validation is a useful shared-helper review candidate. |
| append-versus-remove | Opposite mutations remain structurally close; merging solely by score is wrong. |
| axis-easing | Deliberate horizontal/vertical symmetry; retain for review without requiring consolidation. |
| receive-versus-peek | Removing the head differs from observing it, despite shared guards and returns. |
| unrelated-equality | Distinct domain types legitimately retain their own equality methods. |
| read-write-adapters | Interface symmetry is useful context, not evidence of equivalent I/O. |
| platform-copy | Same copy loop on two platforms is a useful consolidation candidate in this fixture. |
| inline-test-oracle | An independently written test oracle can deliberately resemble production logic. |
| swift-outer-panels | Similar presentation shells perform different actions in nested closures. |

`internal/corpus/actionability_test.go` additionally scans the full fixture root
and its `code/` subtree. It verifies useful production copies survive scoping,
while test helper findings are present in the root scan and absent from the
production-path scan. File scoping alone still includes the Rust inline test
module; fragment selection is tested separately in parser and CLI regressions.

The fixtures are intentionally small and bounded. Future ranking changes should
review both sides of each affected case, retain useful positives, and record why
labels or expected ranks changed. Do not optimize away all test helpers or force
opposite operations to have low structural scores without a separate justified
normalization rule and nearby negative fixtures.
