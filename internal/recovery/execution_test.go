// Copyright 2026 Orisema contributors
// SPDX-License-Identifier: Apache-2.0

package recovery

import (
	"strings"
	"sync"
	"testing"
	"time"
)

func preparedExecution(t *testing.T) (PrepareRequest, Preparation) {
	t.Helper()
	_, request := prepareFixture(t)
	prepared, err := Prepare(request)
	if err != nil {
		t.Fatal(err)
	}
	return request, prepared
}

func TestExecutionLifecycleConsumesTokenAndRecordsResult(t *testing.T) {
	request, prepared := preparedExecution(t)
	beginAt := request.Now.Add(10 * time.Second)
	err := BeginExecution(BeginRequest{HistoryPath: request.HistoryPath, Manifest: request.Manifest, Token: prepared.Token, Report: request.Report, Policy: request.Policy, Now: beginAt})
	if err != nil {
		t.Fatal(err)
	}
	if err := BeginExecution(BeginRequest{HistoryPath: request.HistoryPath, Manifest: request.Manifest, Token: prepared.Token, Report: request.Report, Policy: request.Policy, Now: beginAt}); err == nil || !strings.Contains(err.Error(), "consumed") {
		t.Fatalf("expected consumed token error, got %v", err)
	}
	if err := CompleteExecution(request.HistoryPath, prepared.Token, true, "podman start completed", beginAt.Add(time.Second)); err != nil {
		t.Fatal(err)
	}
	if err := CompleteExecution(request.HistoryPath, prepared.Token, true, "duplicate", beginAt.Add(2*time.Second)); err == nil {
		t.Fatal("expected duplicate result to fail")
	}
	attempts, err := LoadHistory(request.HistoryPath, request.Manifest)
	if err != nil {
		t.Fatal(err)
	}
	if len(attempts) != 2 {
		t.Fatalf("expected seed plus begun attempt, got %#v", attempts)
	}
}

func TestBeginExecutionRevalidatesReservation(t *testing.T) {
	tests := []struct {
		name   string
		change func(*PrepareRequest, Preparation)
		want   string
	}{
		{"expired", func(r *PrepareRequest, p Preparation) { r.Now = p.ExpiresAt }, "expired"},
		{"observation changed", func(r *PrepareRequest, _ Preparation) { r.Report.Execution = "running" }, "observation changed"},
		{"policy changed", func(r *PrepareRequest, _ Preparation) { r.Policy.MinInterval = "6m" }, "policy changed"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			request, prepared := preparedExecution(t)
			request.Now = request.Now.Add(time.Second)
			tc.change(&request, prepared)
			err := BeginExecution(BeginRequest{HistoryPath: request.HistoryPath, Manifest: request.Manifest, Token: prepared.Token, Report: request.Report, Policy: request.Policy, Now: request.Now})
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("expected %q error, got %v", tc.want, err)
			}
		})
	}
}

func TestBeginExecutionIsAtomicAcrossConcurrentCallers(t *testing.T) {
	request, prepared := preparedExecution(t)
	start := make(chan struct{})
	errs := make(chan error, 2)
	var wg sync.WaitGroup
	for i := 0; i < 2; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-start
			errs <- BeginExecution(BeginRequest{HistoryPath: request.HistoryPath, Manifest: request.Manifest, Token: prepared.Token, Report: request.Report, Policy: request.Policy, Now: request.Now.Add(time.Second)})
		}()
	}
	close(start)
	wg.Wait()
	close(errs)
	var succeeded, consumed int
	for err := range errs {
		if err == nil {
			succeeded++
		} else if strings.Contains(err.Error(), "consumed") {
			consumed++
		} else {
			t.Fatalf("unexpected error: %v", err)
		}
	}
	if succeeded != 1 || consumed != 1 {
		t.Fatalf("succeeded=%d consumed=%d", succeeded, consumed)
	}
}

func TestConsumedReservationDoesNotBlockNextPreparation(t *testing.T) {
	_, request := prepareFixture(t)
	request.Policy.MinInterval = "1s"
	prepared, err := Prepare(request)
	if err != nil {
		t.Fatal(err)
	}
	beginAt := request.Now.Add(time.Second)
	if err := BeginExecution(BeginRequest{HistoryPath: request.HistoryPath, Manifest: request.Manifest, Token: prepared.Token, Report: request.Report, Policy: request.Policy, Now: beginAt}); err != nil {
		t.Fatal(err)
	}
	if err := CompleteExecution(request.HistoryPath, prepared.Token, false, "start failed", beginAt.Add(time.Second)); err != nil {
		t.Fatal(err)
	}
	request.Now = beginAt.Add(2 * time.Second)
	if _, err := Prepare(request); err != nil {
		t.Fatalf("consumed reservation should not block after minInterval: %v", err)
	}
}
