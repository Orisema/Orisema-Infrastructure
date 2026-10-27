// Copyright 2026 Orisema contributors
// SPDX-License-Identifier: Apache-2.0

package recovery

import (
	"errors"
	"fmt"
	"strings"
	"syscall"
	"time"

	"github.com/Orisema/Orisema-Infrastructure/internal/diagnostic"
)

type ReconcileRequest struct {
	HistoryPath string
	Manifest    string
	Service     string
	Token       string
	Outcome     string
	Operator    string
	Reason      string
	Report      diagnostic.Report
	Now         time.Time
}

// UnresolvedExecutions returns begun attempts without a matching result.
func UnresolvedExecutions(records []HistoryRecord) []HistoryRecord {
	resolved := make(map[string]bool)
	for _, record := range records {
		if record.Kind == "result" {
			resolved[record.Token] = true
		}
	}
	var unresolved []HistoryRecord
	for _, record := range records {
		if record.Kind == "execution" && record.Status == "started" && !resolved[record.Token] {
			unresolved = append(unresolved, record)
		}
	}
	return unresolved
}

// ReconcileExecution records an operator assertion for a begun attempt whose
// automatic result was never persisted. It observes but never mutates runtime.
func ReconcileExecution(request ReconcileRequest) error {
	request.Operator = strings.TrimSpace(request.Operator)
	request.Reason = strings.TrimSpace(request.Reason)
	if request.Manifest == "" || request.Service == "" || request.Operator == "" || request.Reason == "" {
		return errors.New("manifest, service, operator and reason are required")
	}
	if len(request.Operator) > 128 || len(request.Reason) > 512 {
		return errors.New("operator must be at most 128 bytes and reason at most 512 bytes")
	}
	if request.Outcome != "succeeded" && request.Outcome != "failed" {
		return errors.New("reconciliation outcome must be succeeded or failed")
	}
	if !validReservationToken(request.Token) {
		return errors.New("reservation token must be 32 hexadecimal characters")
	}
	if request.Report.Service != request.Service || request.Report.Name == "" {
		return errors.New("a current diagnostic report for the service is required")
	}
	if request.Now.IsZero() {
		request.Now = time.Now().UTC()
	} else {
		request.Now = request.Now.UTC()
	}

	f, records, info, err := openLockedHistory(request.HistoryPath)
	if err != nil {
		return err
	}
	defer f.Close()
	defer syscall.Flock(int(f.Fd()), syscall.LOCK_UN) //nolint:errcheck
	var execution *HistoryRecord
	for i := range records {
		record := records[i]
		if record.Token != request.Token {
			continue
		}
		if record.Kind == "result" {
			return errors.New("execution result has already been recorded")
		}
		if record.Kind == "execution" {
			copy := record
			execution = &copy
		}
	}
	if execution == nil {
		return errors.New("begun execution was not found")
	}
	if execution.Manifest != request.Manifest || execution.Service != request.Service {
		return errors.New("execution does not match manifest and service")
	}
	if request.Now.Before(execution.Timestamp) {
		return errors.New("reconciliation timestamp precedes the begun attempt")
	}
	record := HistoryRecord{
		Version:       historyVersion,
		Kind:          "result",
		Manifest:      request.Manifest,
		Service:       request.Service,
		Action:        execution.Action,
		Timestamp:     request.Now,
		Token:         request.Token,
		Status:        "reconciled-command-" + request.Outcome,
		Operator:      request.Operator,
		Reason:        request.Reason,
		Container:     request.Report.Name,
		Configuration: request.Report.Configuration,
		Execution:     request.Report.Execution,
		Failure:       request.Report.Failure,
		Detail:        fmt.Sprintf("operator reconciled interrupted START command as %s", request.Outcome),
	}
	return appendLockedRecord(f, info.Size(), record)
}
