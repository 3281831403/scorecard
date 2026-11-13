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

package conformance

import (
	"bytes"
	"encoding/json"
	"errors"
	"flag"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/google/go-cmp/cmp"

	"github.com/ossf/scorecard/v5/finding"
)

// Regenerate the golden files with:
//
//	go test ./internal/conformance -run TestGolden -update
//
// testdata/evaluation-log.json is also checked against the Gemara schema
// by `make validate-gemara`.
var update = flag.Bool("update", false, "update golden files")

func testReport() *Report {
	rel := testRelease()
	results, notEvaluated := Evaluate(rel, "level-1", Input{})
	return &Report{
		Date:             time.Date(2026, time.November, 8, 12, 0, 0, 0, time.UTC),
		Release:          rel,
		Repo:             "github.com/owner/repo",
		Commit:           "0123456789abcdef0123456789abcdef01234567",
		ScorecardVersion: "v5.7.0",
		ScorecardCommit:  "fedcba9876543210fedcba9876543210fedcba98",
		Profile:          "level-1",
		Results:          results,
		NotEvaluated:     notEvaluated,
	}
}

func TestGolden(t *testing.T) {
	t.Parallel()
	tests := []struct {
		write  func(*Report, *bytes.Buffer) error
		name   string
		golden string
	}{
		{
			name:   "conformance JSON",
			golden: "conformance.json",
			write:  func(r *Report, b *bytes.Buffer) error { return r.AsJSON(b) },
		},
		{
			name:   "Gemara Evaluation Log",
			golden: "evaluation-log.json",
			write:  func(r *Report, b *bytes.Buffer) error { return r.AsGemara(b) },
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			var got bytes.Buffer
			if err := tt.write(testReport(), &got); err != nil {
				t.Fatalf("write: %v", err)
			}
			path := filepath.Join("testdata", tt.golden)
			if *update {
				if err := os.WriteFile(path, got.Bytes(), 0o600); err != nil {
					t.Fatalf("update golden file: %v", err)
				}
			}
			want, err := os.ReadFile(path)
			if err != nil {
				t.Fatalf("read golden file: %v", err)
			}
			if diff := cmp.Diff(string(want), got.String()); diff != "" {
				t.Errorf("%s (-want +got):\n%s", path, diff)
			}
		})
	}
}

func TestAsGemaraNoResults(t *testing.T) {
	t.Parallel()
	r := testReport()
	r.Results = nil
	var b bytes.Buffer
	if err := r.AsGemara(&b); !errors.Is(err, errNoResults) {
		t.Errorf("AsGemara() error = %v, want %v", err, errNoResults)
	}
}

func TestAsJSONEmptyLists(t *testing.T) {
	t.Parallel()
	r := testReport()
	r.Results = nil
	r.NotEvaluated = nil
	var b bytes.Buffer
	if err := r.AsJSON(&b); err != nil {
		t.Fatalf("AsJSON() error = %v", err)
	}
	for _, want := range []string{`"results": []`, `"notEvaluated": []`} {
		if !bytes.Contains(b.Bytes(), []byte(want)) {
			t.Errorf("AsJSON() output lacks %s:\n%s", want, b.String())
		}
	}
}

// degradedReport is a test report whose run had a failed probe.
func degradedReport() *Report {
	r := testReport()
	r.FailedProbes = []ProbeError{{
		Probe:   "probeA",
		Message: "internal error: collect failed",
	}}
	return r
}

// TestAsJSONProbeFailures pins the degraded JSON shape: the failed probe and
// its reason appear at the top level, and the normal-path keys stay.
func TestAsJSONProbeFailures(t *testing.T) {
	t.Parallel()
	var b bytes.Buffer
	if err := degradedReport().AsJSON(&b); err != nil {
		t.Fatalf("AsJSON() error = %v", err)
	}
	var out map[string]any
	if err := json.Unmarshal(b.Bytes(), &out); err != nil {
		t.Fatalf("output is not JSON: %v", err)
	}
	failures, ok := out["probeFailures"].([]any)
	if !ok || len(failures) != 1 {
		t.Fatalf("probeFailures = %v, want one entry", out["probeFailures"])
	}
	entry, ok := failures[0].(map[string]any)
	if !ok {
		t.Fatalf("probe failure entry = %v, want an object", failures[0])
	}
	if entry["probe"] != "probeA" || entry["message"] != "internal error: collect failed" {
		t.Errorf("probeFailures entry = %+v", entry)
	}
	// Results still carry the per-requirement Unknown explanation.
	results, ok := out["results"].([]any)
	if !ok {
		t.Fatalf("results = %v, want a list", out["results"])
	}
	if len(results) == 0 {
		t.Fatal("degraded report has no results")
	}
}

// TestAsJSONNoProbeFailuresKeyOnNormalRun pins that a run without failures
// keeps the normal-path output shape, including the golden files.
func TestAsJSONNoProbeFailuresKeyOnNormalRun(t *testing.T) {
	t.Parallel()
	var b bytes.Buffer
	if err := testReport().AsJSON(&b); err != nil {
		t.Fatalf("AsJSON() error = %v", err)
	}
	if bytes.Contains(b.Bytes(), []byte("probeFailures")) {
		t.Errorf("normal run output must not carry probeFailures:\n%s", b.String())
	}
}

// TestAsGemaraProbeFailures pins the degraded Gemara shape: the top-level
// result is Unknown and the description names the failed probe, even when
// every evaluated requirement passed.
func TestAsGemaraProbeFailures(t *testing.T) {
	t.Parallel()
	r := degradedReport()
	// Make every evaluated requirement pass, so the only thing keeping the
	// log from "Passed" is the failed probe.
	for i := range r.Results {
		if r.Results[i].Result != NotApplicable {
			r.Results[i].Result = Pass
		}
	}
	var b bytes.Buffer
	if err := r.AsGemara(&b); err != nil {
		t.Fatalf("AsGemara() error = %v", err)
	}
	var out struct {
		Result   string `json:"result"`
		Metadata struct {
			Description string `json:"description"`
		} `json:"metadata"`
	}
	if err := json.Unmarshal(b.Bytes(), &out); err != nil {
		t.Fatalf("output is not JSON: %v", err)
	}
	if out.Result != gemaraUnknown {
		t.Errorf("top-level result = %q, want %q because a probe failed", out.Result, gemaraUnknown)
	}
	for _, want := range []string{"probeA", "internal error: collect failed", "Unknown"} {
		if !strings.Contains(out.Metadata.Description, want) {
			t.Errorf("description %q does not contain %q", out.Metadata.Description, want)
		}
	}
}

// TestAsGemaraFailedOutranksProbeFailure pins that a genuine failure still
// rolls up as Failed: probe failures cap the result at Unknown but do not
// hide a requirement that actually failed.
func TestAsGemaraFailedOutranksProbeFailure(t *testing.T) {
	t.Parallel()
	r := degradedReport()
	for i := range r.Results {
		if r.Results[i].Requirement == "TEST-01.02" {
			r.Results[i].Result = Fail
		}
	}
	var b bytes.Buffer
	if err := r.AsGemara(&b); err != nil {
		t.Fatalf("AsGemara() error = %v", err)
	}
	var out struct {
		Result string `json:"result"`
	}
	if err := json.Unmarshal(b.Bytes(), &out); err != nil {
		t.Fatalf("output is not JSON: %v", err)
	}
	if out.Result != gemaraFailed {
		t.Errorf("top-level result = %q, want %q", out.Result, gemaraFailed)
	}
}

// TestDescriptionProbeFailures pins the description wording so a partial run
// cannot be read as conformance with the whole catalog.
func TestDescriptionProbeFailures(t *testing.T) {
	t.Parallel()
	r := degradedReport()
	got := r.description()
	if !strings.Contains(got, "1 probe(s) failed") || !strings.Contains(got, "probeA (internal error: collect failed)") {
		t.Errorf("description = %q", got)
	}
	if !strings.HasSuffix(got, "could not be decided.") {
		t.Errorf("description does not end with the Unknown explanation: %q", got)
	}
	// The normal description is unchanged.
	normal := testReport().description()
	if strings.Contains(normal, "probe") && strings.Contains(normal, "failed") {
		t.Errorf("normal description mentions probe failures: %q", normal)
	}
}

func TestGemaraResult(t *testing.T) {
	t.Parallel()
	want := map[Result]string{
		Pass:          "Passed",
		Fail:          "Failed",
		Unknown:       "Unknown",
		NotApplicable: "Not Applicable",
		NeedsReview:   "Needs Review",
	}
	for r, w := range want {
		if got := gemaraResult(r); got != w {
			t.Errorf("gemaraResult(%s) = %q, want %q", r, got, w)
		}
	}
}

func TestAggregate(t *testing.T) {
	t.Parallel()
	tests := []struct {
		want    string
		results []string
	}{
		{want: "Not Applicable", results: []string{"Not Applicable"}},
		{want: "Passed", results: []string{"Not Applicable", "Passed"}},
		{want: "Needs Review", results: []string{"Passed", "Needs Review"}},
		{want: "Unknown", results: []string{"Needs Review", "Unknown", "Passed"}},
		{want: "Failed", results: []string{"Unknown", "Failed", "Needs Review"}},
	}
	for _, tt := range tests {
		if got := aggregate(tt.results); got != tt.want {
			t.Errorf("aggregate(%v) = %q, want %q", tt.results, got, tt.want)
		}
	}
}

func TestScorecardCommitURI(t *testing.T) {
	t.Parallel()
	tests := map[string]string{
		"fedcba9876543210fedcba9876543210fedcba98": "https://github.com/ossf/scorecard/commit/fedcba9876543210fedcba9876543210fedcba98",
		"":        "https://github.com/ossf/scorecard",
		"unknown": "https://github.com/ossf/scorecard",
	}
	for in, want := range tests {
		if got := scorecardCommitURI(in); got != want {
			t.Errorf("scorecardCommitURI(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestRepoURI(t *testing.T) {
	t.Parallel()
	tests := map[string]string{
		"github.com/owner/repo": "https://github.com/owner/repo",
		"file:///tmp/repo":      "file:///tmp/repo",
	}
	for in, want := range tests {
		if got := repoURI(in); got != want {
			t.Errorf("repoURI(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestEvidenceID(t *testing.T) {
	t.Parallel()
	tests := []struct {
		values map[string]string
		want   string
	}{
		{want: "someProbe"},
		{values: map[string]string{"branchName": "main"}, want: "someProbe/main"},
		{values: map[string]string{"z": "2", "a": "1"}, want: "someProbe/1/2"},
	}
	for _, tt := range tests {
		f := &finding.Finding{Probe: "someProbe", Values: tt.values}
		if got := evidenceID(f); got != tt.want {
			t.Errorf("evidenceID(%v) = %q, want %q", tt.values, got, tt.want)
		}
	}
}
