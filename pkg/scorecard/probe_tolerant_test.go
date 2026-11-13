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

package scorecard

import (
	"testing"

	"go.uber.org/mock/gomock"

	"github.com/ossf/scorecard/v5/checker"
	"github.com/ossf/scorecard/v5/clients"
	mockrepo "github.com/ossf/scorecard/v5/clients/mockclients"
	"github.com/ossf/scorecard/v5/probes/blocksDeleteOnBranches"
	"github.com/ossf/scorecard/v5/probes/branchProtectionAppliesToAdmins"
	"github.com/ossf/scorecard/v5/probes/branchesAreProtected"
	"github.com/ossf/scorecard/v5/probes/fuzzed"
	"github.com/ossf/scorecard/v5/probes/requiresPRsToChangeCode"
)

// branchProtectionProbes are the probes the OSPS conformance mappings run.
// They all need the Branch-Protection raw data, collected once.
var branchProtectionProbes = []string{
	blocksDeleteOnBranches.Probe,
	branchProtectionAppliesToAdmins.Probe,
	branchesAreProtected.Probe,
	requiresPRsToChangeCode.Probe,
}

func tolerantCheckRequest(m *mockrepo.MockRepoClient, ret *Result) *checker.CheckRequest {
	return &checker.CheckRequest{
		RepoClient: m,
		RawResults: &ret.RawResults,
	}
}

// TestRunEnabledProbesTolerant_CollectionFailure pins the degraded path the
// OSPS conformance run relies on: when the branch protection data cannot be
// fetched, every probe that needs it is recorded as failed once, the
// collector is called only once, and probes backed by other data still run.
func TestRunEnabledProbesTolerant_CollectionFailure(t *testing.T) {
	t.Parallel()
	ctrl := gomock.NewController(t)
	mockRepoClient := mockrepo.NewMockRepoClient(ctrl)
	// One failed collection: all four branch protection probes share it, so
	// it must not be retried per probe.
	mockRepoClient.EXPECT().GetDefaultBranch().Return(nil, errCollect).Times(1)
	// The fuzzing probe uses different raw data and must still complete.
	mockRepoClient.EXPECT().ListFiles(gomock.Any()).Return(nil, nil).Times(1)
	mockRepoClient.EXPECT().ListProgrammingLanguages().Return(nil, nil).Times(1)

	var ret Result
	req := tolerantCheckRequest(mockRepoClient, &ret)
	probes := append(append([]string{}, branchProtectionProbes...), fuzzed.Probe)
	runEnabledProbesTolerant(req, probes, &ret)

	if len(ret.ProbeErrors) != len(branchProtectionProbes) {
		t.Fatalf("got %d probe errors %v, want one per branch protection probe (%d)",
			len(ret.ProbeErrors), ret.ProbeErrors, len(branchProtectionProbes))
	}
	failed := map[string]string{}
	for _, pe := range ret.ProbeErrors {
		if _, dup := failed[pe.Probe]; dup {
			t.Errorf("probe %s recorded more than once: %v", pe.Probe, ret.ProbeErrors)
		}
		if pe.Message == "" {
			t.Errorf("probe %s has an empty error message", pe.Probe)
		}
		failed[pe.Probe] = pe.Message
	}
	for _, name := range branchProtectionProbes {
		if _, ok := failed[name]; !ok {
			t.Errorf("probe %s not recorded as failed", name)
		}
	}
	if _, bad := failed[fuzzed.Probe]; bad {
		t.Errorf("the fuzzing probe must not be marked failed by an unrelated collection error")
	}
	// The surviving probe's findings are still reported.
	if len(ret.Findings) != 1 || ret.Findings[0].Probe != fuzzed.Probe {
		t.Errorf("findings = %v, want only the fuzzing probe's finding", ret.Findings)
	}
}

// TestRunEnabledProbesTolerant_UnknownProbe covers a probe name that is not
// registered: the failure is recorded and the other probes still run.
func TestRunEnabledProbesTolerant_UnknownProbe(t *testing.T) {
	t.Parallel()
	ctrl := gomock.NewController(t)
	mockRepoClient := mockrepo.NewMockRepoClient(ctrl)
	mockRepoClient.EXPECT().ListFiles(gomock.Any()).Return(nil, nil).Times(1)
	mockRepoClient.EXPECT().ListProgrammingLanguages().Return(nil, nil).Times(1)

	var ret Result
	req := tolerantCheckRequest(mockRepoClient, &ret)
	runEnabledProbesTolerant(req, []string{"noProbeCalledThis", fuzzed.Probe}, &ret)

	if len(ret.ProbeErrors) != 1 || ret.ProbeErrors[0].Probe != "noProbeCalledThis" {
		t.Fatalf("ProbeErrors = %v, want only the unknown probe", ret.ProbeErrors)
	}
	if ret.ProbeErrors[0].Message == "" {
		t.Error("the unknown probe's error message is empty")
	}
	if len(ret.Findings) != 1 || ret.Findings[0].Probe != fuzzed.Probe {
		t.Errorf("findings = %v, want the fuzzing probe's finding", ret.Findings)
	}
}

// TestRunEnabledProbesTolerant_Success pins that a run without failures has
// no probe errors: the normal path must look exactly like the strict path.
func TestRunEnabledProbesTolerant_Success(t *testing.T) {
	t.Parallel()
	ctrl := gomock.NewController(t)
	mockRepoClient := mockrepo.NewMockRepoClient(ctrl)
	mockRepoClient.EXPECT().GetDefaultBranch().
		Return(&clients.BranchRef{Name: asPointer("main")}, nil).Times(1)
	mockRepoClient.EXPECT().ListReleases().Return(nil, nil).Times(1)
	mockRepoClient.EXPECT().ListFiles(gomock.Any()).Return(nil, nil).Times(1)

	var ret Result
	req := tolerantCheckRequest(mockRepoClient, &ret)
	runEnabledProbesTolerant(req, branchProtectionProbes, &ret)

	if len(ret.ProbeErrors) != 0 {
		t.Fatalf("ProbeErrors = %v, want none on a successful run", ret.ProbeErrors)
	}
	if len(ret.Findings) == 0 {
		t.Fatal("expected findings from every probe, got none")
	}
	seen := map[string]bool{}
	for _, f := range ret.Findings {
		seen[f.Probe] = true
	}
	for _, name := range branchProtectionProbes {
		if !seen[name] {
			t.Errorf("missing finding from %s", name)
		}
	}
}

// TestRunEnabledProbesTolerant_SharedRawDataCollectedOnce pins that probes
// sharing a check's raw data trigger exactly one collection, so the degraded
// path does not duplicate API calls.
func TestRunEnabledProbesTolerant_SharedRawDataCollectedOnce(t *testing.T) {
	t.Parallel()
	ctrl := gomock.NewController(t)
	mockRepoClient := mockrepo.NewMockRepoClient(ctrl)
	// Times(1) fails the test if the four probes cause four collections.
	mockRepoClient.EXPECT().GetDefaultBranch().
		Return(&clients.BranchRef{Name: asPointer("main")}, nil).Times(1)
	mockRepoClient.EXPECT().ListReleases().Return(nil, nil).Times(1)
	mockRepoClient.EXPECT().ListFiles(gomock.Any()).Return(nil, nil).Times(1)

	var ret Result
	req := tolerantCheckRequest(mockRepoClient, &ret)
	runEnabledProbesTolerant(req, branchProtectionProbes, &ret)
}
