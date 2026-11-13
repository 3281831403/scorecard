// Copyright 2026 OpenSSF Scorecard Authors
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//      http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package cmd

import (
	"bytes"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/ossf/scorecard/v5/finding"
	"github.com/ossf/scorecard/v5/internal/conformance"
	"github.com/ossf/scorecard/v5/internal/conformance/catalog"
	"github.com/ossf/scorecard/v5/options"
	"github.com/ossf/scorecard/v5/pkg/scorecard"
)

func scanResult(defaultBranch string, findings ...finding.Finding) *scorecard.Result {
	r := &scorecard.Result{
		Repo:      scorecard.RepoInfo{Name: "github.com/owner/repo", CommitSHA: "0123456789abcdef"},
		Scorecard: scorecard.ScorecardInfo{Version: "v5.7.0", CommitSHA: "fedcba9876543210"},
		Date:      time.Date(2026, time.November, 8, 12, 0, 0, 0, time.UTC),
		Findings:  findings,
	}
	r.RawResults.Metadata.Metadata = map[string]string{"repository.defaultBranch": defaultBranch}
	return r
}

func onBranch(probe, branch string, outcome finding.Outcome) finding.Finding {
	return finding.Finding{Probe: probe, Outcome: outcome, Values: map[string]string{"branchName": branch}}
}

func TestConformanceReport(t *testing.T) {
	t.Parallel()
	release, err := catalog.Lookup("osps-baseline")
	if err != nil {
		t.Fatal(err)
	}
	result := scanResult("main",
		onBranch("requiresPRsToChangeCode", "main", finding.OutcomeTrue),
		onBranch("branchProtectionAppliesToAdmins", "main", finding.OutcomeFalse),
		onBranch("blocksDeleteOnBranches", "main", finding.OutcomeTrue),
		onBranch("blocksDeleteOnBranches", "release/v1", finding.OutcomeFalse),
	)
	report := conformanceReport(result, release, "maturity-1")

	want := map[string]conformance.Result{
		"OSPS-AC-03.01": conformance.NeedsReview,
		"OSPS-AC-03.02": conformance.Pass,
	}
	if len(report.Results) != len(want) {
		t.Fatalf("got %d results, want %d: %v", len(report.Results), len(want), report.Results)
	}
	for _, r := range report.Results {
		if r.Result != want[r.Requirement] {
			t.Errorf("%s = %s, want %s", r.Requirement, r.Result, want[r.Requirement])
		}
	}
	if got := len(report.NotEvaluated); got != 23 {
		t.Errorf("got %d requirements not evaluated, want 23", got)
	}
	if report.Repo != "github.com/owner/repo" || report.Commit != "0123456789abcdef" ||
		report.ScorecardVersion != "v5.7.0" || report.Profile != "maturity-1" {
		t.Errorf("report identity = %+v", report)
	}
}

// TestConformanceReportUnknownDefaultBranch covers a platform that can't
// report the default branch: findings for a branch named "unknown" must not
// be read as the default branch's.
func TestConformanceReportUnknownDefaultBranch(t *testing.T) {
	t.Parallel()
	release, err := catalog.Lookup("osps-baseline")
	if err != nil {
		t.Fatal(err)
	}
	result := scanResult("unknown",
		onBranch("requiresPRsToChangeCode", "unknown", finding.OutcomeTrue),
		onBranch("branchProtectionAppliesToAdmins", "unknown", finding.OutcomeTrue),
		onBranch("blocksDeleteOnBranches", "unknown", finding.OutcomeTrue),
	)
	report := conformanceReport(result, release, "maturity-1")
	for _, r := range report.Results {
		if r.Result != conformance.Unknown || r.Explanation != "The default branch is unknown." {
			t.Errorf("%s = %s (%s), want UNKNOWN because the default branch is unknown",
				r.Requirement, r.Result, r.Explanation)
		}
	}
}

// TestConformanceReportEmptyRepository covers a scan that returns before
// recording a default branch, as Scorecard does for an empty repository.
func TestConformanceReportEmptyRepository(t *testing.T) {
	t.Parallel()
	release, err := catalog.Lookup("osps-baseline")
	if err != nil {
		t.Fatal(err)
	}
	result := &scorecard.Result{Repo: scorecard.RepoInfo{Name: "github.com/owner/empty"}}
	report := conformanceReport(result, release, "maturity-1")
	for _, r := range report.Results {
		if r.Result != conformance.Unknown {
			t.Errorf("%s = %s, want UNKNOWN", r.Requirement, r.Result)
		}
	}
}

// TestConformanceReportProbeFailure covers a degraded scan: the branch
// protection data could not be fetched, so every mapped probe failed. The
// report is still produced, every mapped requirement is UNKNOWN with the
// probe and reason named, and the failures are recorded on the report.
func TestConformanceReportProbeFailure(t *testing.T) {
	t.Parallel()
	release, err := catalog.Lookup("osps-baseline")
	if err != nil {
		t.Fatal(err)
	}
	failedProbes := []scorecard.ProbeError{
		{Probe: "blocksDeleteOnBranches", Message: "internal error: collect failed"},
		{Probe: "branchProtectionAppliesToAdmins", Message: "internal error: collect failed"},
		{Probe: "branchesAreProtected", Message: "internal error: collect failed"},
		{Probe: "requiresPRsToChangeCode", Message: "internal error: collect failed"},
	}
	result := scanResult("main")
	result.ProbeErrors = failedProbes
	report := conformanceReport(result, release, "maturity-1")

	if len(report.FailedProbes) != len(failedProbes) {
		t.Fatalf("report records %d failed probes, want %d: %v",
			len(report.FailedProbes), len(failedProbes), report.FailedProbes)
	}
	if len(report.Results) != 2 {
		t.Fatalf("got %d results, want the two mapped requirements", len(report.Results))
	}
	for _, r := range report.Results {
		if r.Result != conformance.Unknown {
			t.Errorf("%s = %s, want UNKNOWN because its probes failed", r.Requirement, r.Result)
		}
		if !strings.Contains(r.Explanation, "failed") ||
			!strings.Contains(r.Explanation, "internal error: collect failed") {
			t.Errorf("%s explanation does not name the probe failure: %q", r.Requirement, r.Explanation)
		}
	}
	// The other profile requirements are still not evaluated, not failed.
	if got := len(report.NotEvaluated); got != 23 {
		t.Errorf("got %d requirements not evaluated, want 23", got)
	}
}

// TestConformanceReportProbeFailureOutputs pins the degraded output for both
// formats: the JSON names the failed probe at the top level, and the Gemara
// log's overall result is Unknown rather than Passed.
func TestConformanceReportProbeFailureOutputs(t *testing.T) {
	t.Parallel()
	release, err := catalog.Lookup("osps-baseline")
	if err != nil {
		t.Fatal(err)
	}
	result := scanResult("main")
	result.ProbeErrors = []scorecard.ProbeError{{
		Probe:   "blocksDeleteOnBranches",
		Message: "internal error: collect failed",
	}}
	report := conformanceReport(result, release, "maturity-1")

	var jsonOut bytes.Buffer
	if err := writeConformance(&jsonOut, options.FormatJSON, report); err != nil {
		t.Fatalf("writeConformance(json) = %v", err)
	}
	var decoded map[string]any
	if err := json.Unmarshal(jsonOut.Bytes(), &decoded); err != nil {
		t.Fatalf("JSON output is not valid: %v", err)
	}
	failures, ok := decoded["probeFailures"].([]any)
	if !ok || len(failures) != 1 {
		t.Fatalf("JSON probeFailures = %v, want one entry", decoded["probeFailures"])
	}
	entry, ok := failures[0].(map[string]any)
	if !ok {
		t.Fatalf("JSON probe failure entry = %v, want an object", failures[0])
	}
	if entry["probe"] != "blocksDeleteOnBranches" ||
		entry["message"] != "internal error: collect failed" {
		t.Errorf("JSON probe failure entry = %+v", entry)
	}

	var gemaraOut bytes.Buffer
	if err := writeConformance(&gemaraOut, options.FormatGemara, report); err != nil {
		t.Fatalf("writeConformance(gemara) = %v", err)
	}
	var gemara struct {
		Result   string `json:"result"`
		Metadata struct {
			Description string `json:"description"`
		} `json:"metadata"`
	}
	if err := json.Unmarshal(gemaraOut.Bytes(), &gemara); err != nil {
		t.Fatalf("Gemara output is not valid: %v", err)
	}
	if gemara.Result != "Unknown" {
		t.Errorf("Gemara result = %q, want Unknown", gemara.Result)
	}
	if !strings.Contains(gemara.Metadata.Description, "blocksDeleteOnBranches") {
		t.Errorf("Gemara description does not name the failed probe: %q",
			gemara.Metadata.Description)
	}
}

// TestConformanceReportPartialProbeFailure pins that requirements whose
// probes completed are still evaluated when another probe failed.
func TestConformanceReportPartialProbeFailure(t *testing.T) {
	t.Parallel()
	release, err := catalog.Lookup("osps-baseline")
	if err != nil {
		t.Fatal(err)
	}
	result := scanResult("main",
		onBranch("blocksDeleteOnBranches", "main", finding.OutcomeTrue),
	)
	result.ProbeErrors = []scorecard.ProbeError{{
		Probe:   "requiresPRsToChangeCode",
		Message: "internal error: collect failed",
	}}
	report := conformanceReport(result, release, "maturity-1")

	byReq := map[string]conformance.RequirementResult{}
	for _, r := range report.Results {
		byReq[r.Requirement] = r
	}
	// AC-03.02's only probe succeeded, so it still passes.
	if r := byReq["OSPS-AC-03.02"]; r.Result != conformance.Pass {
		t.Errorf("OSPS-AC-03.02 = %s, want PASS: its probe did not fail", r.Result)
	}
	// AC-03.01 needs the failed probe, so it is Unknown and never FAIL.
	r := byReq["OSPS-AC-03.01"]
	if r.Result != conformance.Unknown {
		t.Errorf("OSPS-AC-03.01 = %s, want UNKNOWN", r.Result)
	}
	if !strings.Contains(r.Explanation, "requiresPRsToChangeCode") {
		t.Errorf("OSPS-AC-03.01 explanation does not name the failed probe: %q", r.Explanation)
	}
}

func TestWriteConformanceJSON(t *testing.T) {
	t.Parallel()
	release, err := catalog.Lookup("osps-baseline")
	if err != nil {
		t.Fatal(err)
	}
	report := conformanceReport(scanResult("main"), release, "maturity-1")
	var b bytes.Buffer
	if err := writeConformance(&b, options.FormatJSON, report); err != nil {
		t.Fatalf("writeConformance() = %v", err)
	}
	var out map[string]any
	if err := json.Unmarshal(b.Bytes(), &out); err != nil {
		t.Fatalf("output is not JSON: %v", err)
	}
	for _, key := range []string{"experimental", "date", "repo", "scorecard", "catalog", "profile", "results", "notEvaluated"} {
		if _, ok := out[key]; !ok {
			t.Errorf("output lacks %q", key)
		}
	}
	for _, key := range []string{"checks", "score", "aggregate_score"} {
		if _, ok := out[key]; ok {
			t.Errorf("output has check score key %q", key)
		}
	}
	if out["experimental"] != true {
		t.Errorf("experimental = %v, want true", out["experimental"])
	}
}

func TestWriteConformanceGemara(t *testing.T) {
	t.Parallel()
	release, err := catalog.Lookup("osps-baseline@v2026.08.28")
	if err != nil {
		t.Fatal(err)
	}
	report := conformanceReport(scanResult("main"), release, "maturity-1")
	var b bytes.Buffer
	if err := writeConformance(&b, options.FormatGemara, report); err != nil {
		t.Fatalf("writeConformance() = %v", err)
	}
	var out struct {
		Metadata struct {
			MappingReferences []struct {
				Version string `json:"version"`
			} `json:"mapping-references"`
		} `json:"metadata"`
	}
	if err := json.Unmarshal(b.Bytes(), &out); err != nil {
		t.Fatalf("output is not JSON: %v", err)
	}
	refs := out.Metadata.MappingReferences
	if len(refs) != 1 || refs[0].Version != "v2026.08.28" {
		t.Errorf("mapping-references = %+v, want one entry with version v2026.08.28", refs)
	}
}

func TestWriteConformanceRejectsOtherFormats(t *testing.T) {
	t.Parallel()
	release, err := catalog.Lookup("osps-baseline")
	if err != nil {
		t.Fatal(err)
	}
	report := conformanceReport(scanResult("main"), release, "maturity-1")
	var b bytes.Buffer
	if err := writeConformance(&b, options.FormatDefault, report); !errors.Is(err, errCatalogFormat) {
		t.Errorf("writeConformance(default) = %v, want %v", err, errCatalogFormat)
	}
}
