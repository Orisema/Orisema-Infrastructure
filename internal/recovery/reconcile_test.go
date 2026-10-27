// Copyright 2026 Orisema contributors
// SPDX-License-Identifier: Apache-2.0

package recovery

import (
	"strings"
	"sync"
	"testing"
	"time"
)

func interruptedExecution(t *testing.T) (PrepareRequest, Preparation, time.Time) {
	t.Helper()
	request, prepared := preparedExecution(t)
	beginAt := request.Now.Add(time.Second)
	if err := BeginExecution(BeginRequest{HistoryPath: request.HistoryPath, Manifest: request.Manifest, Token: prepared.Token, Report: request.Report, Policy: request.Policy, Now: beginAt}); err != nil {
		t.Fatal(err)
	}
	return request, prepared, beginAt
}

func TestReconcileExecutionRecordsEvidence(t *testing.T) {
	request, prepared, beginAt := interruptedExecution(t)
	records, err := LoadJournal(request.HistoryPath, request.Manifest)
	if err != nil {
		t.Fatal(err)
	}
	if got := UnresolvedExecutions(records); len(got) != 1 || got[0].Token != prepared.Token {
		t.Fatalf("unexpected unresolved executions: %#v", got)
	}
	err = ReconcileExecution(ReconcileRequest{
		HistoryPath: request.HistoryPath,
		Manifest:    request.Manifest,
		Service:     request.Report.Service,
		Token:       prepared.Token,
		Outcome:     "succeeded",
		Operator:    "alice",
		Reason:      "verified application evidence",
		Report:      request.Report,
		Now:         beginAt.Add(time.Minute),
	})
	if err != nil {
		t.Fatal(err)
	}
	records, err = LoadJournal(request.HistoryPath, request.Manifest)
	if err != nil {
		t.Fatal(err)
	}
	if got := UnresolvedExecutions(records); len(got) != 0 {
		t.Fatalf("expected resolution, got %#v", got)
	}
	result := records[len(records)-1]
	if result.Status != "reconciled-command-succeeded" || result.Operator != "alice" || result.Execution == "" {
		t.Fatalf("unexpected result: %#v", result)
	}
}

func TestReconcileExecutionIsSingleWriter(t *testing.T) {
	request, prepared, beginAt := interruptedExecution(t)
	base := ReconcileRequest{HistoryPath: request.HistoryPath, Manifest: request.Manifest, Service: request.Report.Service, Token: prepared.Token, Outcome: "failed", Operator: "alice", Reason: "verified failure", Report: request.Report, Now: beginAt.Add(time.Minute)}
	start := make(chan struct{})
	errs := make(chan error, 2)
	var wg sync.WaitGroup
	for i := 0; i < 2; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-start
			errs <- ReconcileExecution(base)
		}()
	}
	close(start)
	wg.Wait()
	close(errs)
	var succeeded, duplicate int
	for err := range errs {
		if err == nil {
			succeeded++
		} else if strings.Contains(err.Error(), "already") {
			duplicate++
		} else {
			t.Fatalf("unexpected error: %v", err)
		}
	}
	if succeeded != 1 || duplicate != 1 {
		t.Fatalf("succeeded=%d duplicate=%d", succeeded, duplicate)
	}
}

func TestReconcileExecutionRejectsWrongScope(t *testing.T) {
	request, prepared, beginAt := interruptedExecution(t)
	err := ReconcileExecution(ReconcileRequest{HistoryPath: request.HistoryPath, Manifest: "other", Service: request.Report.Service, Token: prepared.Token, Outcome: "failed", Operator: "alice", Reason: "test", Report: request.Report, Now: beginAt.Add(time.Minute)})
	if err == nil || !strings.Contains(err.Error(), "does not match") {
		t.Fatalf("expected scope error, got %v", err)
	}
}
