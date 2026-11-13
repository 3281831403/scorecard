## ADDED Requirements

### Requirement: Probe Failure Degradation

In conformance mode, when a probe cannot collect its raw data or cannot run,
the system SHALL record that probe once, with the error, and continue running
the remaining probes. It SHALL NOT end the run or discard the findings the
other probes produce. Raw data for a check SHALL be collected at most once,
so a failed collection is never retried on behalf of another probe that needs
the same data.

Native probe mode without `--catalog` SHALL keep ending the run on a probe
error; tolerant execution applies only to conformance mode.

#### Scenario: One probe's data cannot be fetched

- **GIVEN** the branch protection raw data collection fails while the other
  mapped probes use different data
- **WHEN** a conformance run executes
- **THEN** every probe that needed the failed data is recorded as failed with
  the error
- **AND** the failing collection is attempted once
- **AND** the probes using other data still produce findings and the run
  completes with a report

#### Scenario: Native probe mode stays strict

- **WHEN** a user runs `scorecard --probes=...` without `--catalog` and a
  probe errors
- **THEN** the run ends with an error as it did before, with no conformance
  report

### Requirement: UNKNOWN Result for a Failed Probe

A mapped requirement whose mapping names a probe that failed SHALL be
evaluated as `UNKNOWN`. Its explanation SHALL name the failed probe and the
reason it could not run, and it SHALL keep any findings that probe produced
before failing as evidence. A requirement whose probes all completed SHALL be
evaluated by its mapping unchanged. A failed probe MUST NOT produce a `FAIL`
result: missing evidence is not evidence that a control is unconfigured.

#### Scenario: Requirement needs a failed probe

- **GIVEN** `requiresPRsToChangeCode` failed with a collection error
- **WHEN** OSPS-AC-03.01 is evaluated
- **THEN** its result is `UNKNOWN`
- **AND** its explanation names `requiresPRsToChangeCode` and the error
- **AND** it is not `FAIL`

#### Scenario: Requirement's probes all completed

- **GIVEN** `blocksDeleteOnBranches` completed and reported `True` while an
  unrelated probe failed
- **WHEN** OSPS-AC-03.02 is evaluated
- **THEN** its result is still evaluated by its mapping and is `PASS`

#### Scenario: Requirements outside the profile are unaffected

- **GIVEN** a probe failed during the run
- **WHEN** a requirement that does not apply to the selected profile is
  recorded
- **THEN** its result remains `NOT_APPLICABLE`
