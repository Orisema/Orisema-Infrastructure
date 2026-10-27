// Copyright 2026 Orisema contributors
// SPDX-License-Identifier: Apache-2.0

package recovery

import (
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/Orisema/Orisema-Infrastructure/internal/diagnostic"
	"github.com/Orisema/Orisema-Infrastructure/internal/manifest"
)

func prepareFixture(t *testing.T) (string, PrepareRequest) {
	t.Helper()
	path := filepath.Join(t.TempDir(), "history.jsonl")
	now := time.Date(2026, 10, 27, 12, 0, 0, 0, time.UTC)
	if err := AppendHistory(path, HistoryRecord{Manifest: "demo", Service: "web", Action: diagnostic.ActionStart, Timestamp: now.Add(-time.Hour)}); err != nil {
		t.Fatal(err)
	}
	request := PrepareRequest{
		HistoryPath: path,
		Manifest:    "demo",
		Report: diagnostic.Report{
			Name:          "orisema-demo-web",
			Service:       "web",
			Configuration: "matches declared fields",
			Execution:     "exited",
			Failure:       "container is not running",
			Action:        diagnostic.ActionStart,
		},
		Policy:   &manifest.Recovery{MaxAttempts: 3, MinInterval: "5m", Cooldown: "30m"},
		Operator: "alice",
		Reason:   "incident review approved",
		Now:      now,
	}
	return path, request
}

func TestPreparePersistsAuditableReservation(t *testing.T) {
	path, request := prepareFixture(t)
	result, err := Prepare(request)
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Token) != 32 || !result.ExpiresAt.Equal(request.Now.Add(5*time.Minute)) || !result.Decision.Allowed {
		t.Fatalf("unexpected preparation: %#v", result)
	}
	f, err := os.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	records, err := readHistoryRecords(f)
	if err != nil {
		t.Fatal(err)
	}
	got := records[len(records)-1]
	if got.Kind != "reservation" || got.Token != result.Token || got.Operator != "alice" || got.Container != "orisema-demo-web" {
		t.Fatalf("unexpected record: %#v", got)
	}
}

func TestPrepareIsAtomicAcrossConcurrentCallers(t *testing.T) {
	_, request := prepareFixture(t)
	start := make(chan struct{})
	errs := make(chan error, 2)
	var wg sync.WaitGroup
	for i := 0; i < 2; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-start
			_, err := Prepare(request)
			errs <- err
		}()
	}
	close(start)
	wg.Wait()
	close(errs)
	var succeeded, blocked int
	for err := range errs {
		if err == nil {
			succeeded++
		} else if strings.Contains(err.Error(), "active reservation") {
			blocked++
		} else {
			t.Fatalf("unexpected error: %v", err)
		}
	}
	if succeeded != 1 || blocked != 1 {
		t.Fatalf("succeeded=%d blocked=%d", succeeded, blocked)
	}
}

func TestPrepareFailsClosedWhenGuardrailBlocks(t *testing.T) {
	path, request := prepareFixture(t)
	for i := 1; i <= 3; i++ {
		if err := AppendHistory(path, HistoryRecord{Manifest: "demo", Service: "web", Action: diagnostic.ActionStart, Timestamp: request.Now.Add(-time.Duration(i) * 6 * time.Minute)}); err != nil {
			t.Fatal(err)
		}
	}
	result, err := Prepare(request)
	if err == nil || !strings.Contains(err.Error(), "attempt limit") || result.Decision.Allowed {
		t.Fatalf("result=%#v err=%v", result, err)
	}
}

func TestPrepareAllowsReplacementAfterReservationExpires(t *testing.T) {
	_, request := prepareFixture(t)
	first, err := Prepare(request)
	if err != nil {
		t.Fatal(err)
	}
	request.Now = first.ExpiresAt.Add(time.Second)
	second, err := Prepare(request)
	if err != nil {
		t.Fatal(err)
	}
	if second.Token == first.Token {
		t.Fatal("expected a new token after expiry")
	}
}

func TestPrepareRequiresRevalidatedStartAndExistingHistory(t *testing.T) {
	_, request := prepareFixture(t)
	request.Report.Action = diagnostic.ActionInvestigate
	if _, err := Prepare(request); err == nil {
		t.Fatal("expected non-START recommendation to fail")
	}
	request.Report.Action = diagnostic.ActionStart
	request.HistoryPath = filepath.Join(t.TempDir(), "missing")
	if _, err := Prepare(request); err == nil {
		t.Fatal("expected missing history to fail")
	}
}
