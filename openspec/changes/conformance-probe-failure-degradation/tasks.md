# Tasks: Degrade a conformance run to UNKNOWN when a probe fails

## 1. Tolerant probe execution

- [x] 1.1 Add a tolerant probe path that records a failed probe with its
  reason and continues with the other probes, without re-collecting raw data
  a check already fetched.
- [x] 1.2 Keep the strict probe path unchanged for native `--probes` runs;
  enable the tolerant path only in conformance mode.
- [x] 1.3 Carry the failed probes from the scan result into the conformance
  input and report.

## 2. Evaluation and output

- [x] 2.1 Evaluate a requirement whose mapping names a failed probe as
  UNKNOWN with the probe and reason in the explanation, keeping any partial
  findings as evidence.
- [x] 2.2 Evaluate requirements whose probes completed exactly as before.
- [x] 2.3 Add the top-level `probeFailures` field to conformance JSON,
  present only in a degraded run.
- [x] 2.4 Roll the Gemara log result up as Unknown when a probe failed
  (unless a requirement genuinely Failed) and name the failed probes in the
  description.

## 3. Validation

- [x] 3.1 Add unit tests for the tolerant runner, degraded evaluation, and
  both degraded output formats, including the no-duplicate-fetch guarantee.
- [x] 3.2 Confirm the conformance golden files and existing tests are
  unchanged on the normal path.
- [x] 3.3 Run the affected package unit tests (with `SKIP_GINKGO=1` for the
  network-backed `pkg/scorecard` e2e suite), the race detector, and
  `make check-linter`. The linter's remaining findings pre-exist on the
  baseline; this change adds none.
