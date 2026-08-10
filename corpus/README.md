# Labeled calibration corpus

This directory contains small, original fixtures distributed under Mori's MIT
license. It is a regression and calibration corpus, not evidence for a
universal similarity threshold or proof of semantic equivalence.

`manifest.json` records reviewer classifications, language pairs, thresholds,
expected score ranges and ranks, and optional pinned reference scores. Run:

```sh
go run ./internal/cmd/corpuseval
```

The deterministic JSON report includes fragment sizes, score and rank movement,
precision at k over labeled review-relevant groups, actionability by distinct
case, and per-classification score distributions. CI fails when a case drifts
outside its reviewed range or rank. Every source change, label change, or range
change requires human review; the evaluator does not create ground truth.
Reference movements in the initial manifest were replayed with the official
v0.22.0 binary and normalization version 8 over the same ten labeled cases.
Cases for languages added after that release omit reference movement until a
compatible pinned reference is available; they still enforce reviewed current
score ranges and ranks.

Classifications:

- `accepted-positive`: a structurally useful review lead;
- `intentional-similarity`: correctly similar structure that is intentionally
  retained; and
- `false-positive`: a scored resemblance that should not drive a refactor.
