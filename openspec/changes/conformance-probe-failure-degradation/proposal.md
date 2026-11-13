# Proposal: Degrade a conformance run to UNKNOWN when a probe fails

## Why

Current behavior (target state of the unarchived `osps-baseline-conformance`
change): conformance mode runs the mapped probes through Scorecard's strict
probe path. When any probe's raw data collection or implementation returns
an error — for example branch protection data that cannot be fetched —
`runEnabledProbes` aborts the whole run with
`internal error: ending run`. No conformance report is written, so a single
unobservable setting loses every requirement result, including requirements
whose probes succeeded.

A probe failing is evidence that a requirement could not be assessed, not
evidence that the requirement is unmet. Reporting nothing is worse than
reporting UNKNOWN: a consumer cannot distinguish "Scorecard could not look"
from "the control is missing", and a run that mostly succeeded is discarded.

This is observed when running conformance against
`ossf/scorecard@4ddc782c` with restricted access to branch protection data.

## What Changes

- In conformance mode, run probes tolerantly: a failed probe is recorded once
  with its reason and the other probes still run. Raw data is still collected
  once per check, so a degraded run never repeats the same failing fetch.
- Each mapped requirement whose probes include a failed probe is evaluated as
  `UNKNOWN`, with an explanation naming the probe and the error. Findings the
  probe did produce before failing are kept as evidence. Requirements whose
  probes all completed are evaluated exactly as before; missing evidence is
  never turned into `FAIL`.
- The conformance JSON gains a top-level `probeFailures` list, present only in
  a degraded run, naming each failed probe and reason. The normal-path output
  shape is unchanged.
- The Gemara Evaluation Log's top-level `result` is `Unknown` when a probe
  failed unless a requirement genuinely `Failed` (which still rolls up as
  `Failed`), and `metadata.description` names the failed probes and states the
  run is incomplete.
- Native probe mode (`--probes` without `--catalog`) keeps its strict
  behavior: it still ends the run on a probe error. Only conformance mode
  opts into tolerant execution.

## Capabilities

- `conformance-evaluation`: tolerant probe execution and UNKNOWN degradation.
- `result-formats`: the `probeFailures` JSON field and the Gemara result and
  description changes.

## Impact

- Public Go API: adds an unexported execution option and a `ProbeError` value
  carried on the internal conformance report and the `pkg/scorecard.Result`.
  No existing exported signature changes behavior for existing callers.
- Output: conformance JSON adds an optional field; conformance Gemara changes
  only in degraded runs. The probe, native JSON, SARIF, raw, in-toto, and text
  outputs are unchanged, and the conformance golden files stay byte-identical.
- No score, reason, finding, remediation, flag, environment variable, or
  feature gate changes.
