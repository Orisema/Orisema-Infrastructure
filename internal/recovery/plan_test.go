// Copyright 2026 Orisema contributors
// SPDX-License-Identifier: Apache-2.0

package recovery

import (
	"testing"

	"github.com/Orisema/Orisema-Infrastructure/internal/diagnostic"
)

func TestBuildTypedRecoveryPlan(t *testing.T) {
	reports := []diagnostic.Report{
		{Name: "healthy", Action: diagnostic.ActionNone, Failure: "none detected"},
		{Name: "unhealthy", Action: diagnostic.ActionInvestigate, Failure: "application unhealthy"},
		{Name: "stopped", Action: diagnostic.ActionStart, Failure: "container is not running"},
		{Name: "missing", Action: diagnostic.ActionRedeployReview, Failure: "container does not exist"},
		{Name: "runtime", Action: diagnostic.ActionRetryRuntime, Failure: "runtime unavailable"},
	}
	steps := Build(reports)
	if len(steps) != len(reports) {
		t.Fatalf("got %d steps, want %d", len(steps), len(reports))
	}
	for i, step := range steps {
		if step.Action != reports[i].Action || step.Reason != reports[i].Failure || len(step.Preconditions) == 0 || step.Risk == "" {
			t.Fatalf("step %d lost typed evidence: %+v", i, step)
		}
	}
	if steps[0].RequiresConfirmation || steps[1].RequiresConfirmation || steps[4].RequiresConfirmation {
		t.Fatal("read-only recommendations must not require mutation confirmation")
	}
	if !steps[2].RequiresConfirmation || !steps[3].RequiresConfirmation {
		t.Fatal("START and REDEPLOY_REVIEW must require explicit confirmation")
	}
}

func TestBuildUnknownActionFallsBackToInvestigation(t *testing.T) {
	steps := Build([]diagnostic.Report{{Name: "service", Action: diagnostic.Action("FUTURE"), Failure: "unknown"}})
	if steps[0].Action != diagnostic.ActionInvestigate || steps[0].RequiresConfirmation {
		t.Fatalf("unexpected fallback: %+v", steps[0])
	}
}
