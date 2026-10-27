// Copyright 2026 Orisema contributors
// SPDX-License-Identifier: Apache-2.0

package recovery

import (
	"strings"
	"testing"
	"time"

	"github.com/Orisema/Orisema-Infrastructure/internal/diagnostic"
)

func completedExecution(t *testing.T) (PrepareRequest, Preparation, time.Time) {
	t.Helper()
	request, prepared, beginAt := interruptedExecution(t)
	resultAt := beginAt.Add(time.Second)
	if err := CompleteExecution(request.HistoryPath, prepared.Token, true, "podman start completed", resultAt); err != nil {
		t.Fatal(err)
	}
	return request, prepared, resultAt
}

func TestVerifyExecutionRecordsObservationWithoutClaimingCausality(t *testing.T) {
	request, prepared, resultAt := completedExecution(t)
	report := diagnostic.Report{Name: request.Report.Name, Service: "web", Configuration: "matches declared fields", Execution: "running", Health: "healthy", Failure: "none detected", Action: diagnostic.ActionNone}
	status, err := VerifyExecution(VerifyRequest{HistoryPath: request.HistoryPath, Manifest: request.Manifest, Service: "web", Token: prepared.Token, Report: report, Now: resultAt.Add(time.Minute)})
	if err != nil {
		t.Fatal(err)
	}
	if status != "healthy" {
		t.Fatalf("status=%q", status)
	}
	records, err := LoadJournal(request.HistoryPath, request.Manifest)
	if err != nil {
		t.Fatal(err)
	}
	got := records[len(records)-1]
	if got.Kind != "verification" || got.Status != "healthy" || !strings.Contains(got.Detail, "causality not inferred") {
		t.Fatalf("unexpected verification: %#v", got)
	}
}

func TestVerifyExecutionRequiresTerminalCommandResult(t *testing.T) {
	request, prepared, beginAt := interruptedExecution(t)
	_, err := VerifyExecution(VerifyRequest{HistoryPath: request.HistoryPath, Manifest: request.Manifest, Service: "web", Token: prepared.Token, Report: request.Report, Now: beginAt.Add(time.Minute)})
	if err == nil || !strings.Contains(err.Error(), "completed START command") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestClassifyVerification(t *testing.T) {
	tests := []struct {
		name   string
		report diagnostic.Report
		want   string
	}{
		{"healthy", diagnostic.Report{Configuration: "matches declared fields", Execution: "running", Health: "healthy"}, "healthy"},
		{"starting", diagnostic.Report{Configuration: "matches declared fields", Execution: "running", Health: "starting"}, "starting"},
		{"no healthcheck", diagnostic.Report{Configuration: "matches declared fields", Execution: "running", Health: "not configured"}, "running-unverified"},
		{"unhealthy", diagnostic.Report{Configuration: "matches declared fields", Execution: "running", Health: "unhealthy"}, "unhealthy"},
		{"stopped", diagnostic.Report{Configuration: "matches declared fields", Execution: "exited"}, "not-running"},
		{"drift", diagnostic.Report{Configuration: "drift: image", Execution: "running"}, "configuration-changed"},
		{"unknown", diagnostic.Report{Configuration: "matches declared fields", Execution: "unavailable"}, "unknown"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := classifyVerification(tc.report); got != tc.want {
				t.Fatalf("got=%q want=%q", got, tc.want)
			}
		})
	}
}
