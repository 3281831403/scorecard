## ADDED Requirements

### Requirement: Conformance JSON Probe Failures

When one or more probes fail during a conformance run, the conformance JSON
SHALL include a top-level `probeFailures` list. Each entry SHALL contain the
failed `probe` name and a `message` with the reason. When every probe
completes, the field SHALL be absent, so the normal-path output shape is
unchanged. The per-requirement `results` and `notEvaluated` lists SHALL still
be written in a degraded run.

#### Scenario: Degraded run names the failed probe

- **GIVEN** the `blocksDeleteOnBranches` probe failed
- **WHEN** the run is written with `--format json`
- **THEN** the output has a `probeFailures` entry naming
  `blocksDeleteOnBranches` and its error
- **AND** every mapped requirement still appears in `results`

#### Scenario: Successful run omits the field

- **WHEN** every probe completes and the run is written with `--format json`
- **THEN** the output has no `probeFailures` field

### Requirement: Gemara Degraded Log Result

When one or more probes fail during a conformance run, the Gemara Evaluation
Log's top-level `result` SHALL be `Unknown`, unless a requirement genuinely
evaluated as `Failed`, in which case it remains `Failed`. The
`metadata.description` SHALL name each failed probe with its reason and state
that the run is incomplete. When every probe completes, the result and
description SHALL be computed as before.

#### Scenario: Probe failure makes the log Unknown

- **GIVEN** every evaluated requirement passed but one probe failed
- **WHEN** the run is written with `--format gemara`
- **THEN** the log's top-level `result` is `Unknown`
- **AND** `metadata.description` names the failed probe and its reason

#### Scenario: A real failure still rolls up as Failed

- **GIVEN** one requirement evaluated as `Failed` and one probe also failed
- **WHEN** the run is written with `--format gemara`
- **THEN** the log's top-level `result` is `Failed`
- **AND** the description still names the failed probe
