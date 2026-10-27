// Copyright 2026 Orisema contributors
// SPDX-License-Identifier: Apache-2.0

package recovery

import (
	"errors"
	"syscall"
	"time"

	"github.com/Orisema/Orisema-Infrastructure/internal/diagnostic"
)

type VerifyRequest struct {
	HistoryPath string
	Manifest    string
	Service     string
	Token       string
	Report      diagnostic.Report
	Now         time.Time
}

// VerifyExecution appends a read-only post-command observation. It does not
// claim causality and it never mutates the runtime.
func VerifyExecution(request VerifyRequest) (string, error) {
	if request.Manifest == "" || request.Service == "" || request.Report.Service != request.Service || request.Report.Name == "" {
		return "", errors.New("manifest, service and current diagnostic report are required")
	}
	if !validReservationToken(request.Token) {
		return "", errors.New("reservation token must be 32 hexadecimal characters")
	}
	if request.Now.IsZero() {
		request.Now = time.Now().UTC()
	} else {
		request.Now = request.Now.UTC()
	}
	f, records, info, err := openLockedHistory(request.HistoryPath)
	if err != nil {
		return "", err
	}
	defer f.Close()
	defer syscall.Flock(int(f.Fd()), syscall.LOCK_UN) //nolint:errcheck
	var execution *HistoryRecord
	var result *HistoryRecord
	for i := range records {
		record := records[i]
		if record.Token != request.Token {
			continue
		}
		if record.Kind == "execution" {
			copy := record
			execution = &copy
		}
		if record.Kind == "result" {
			copy := record
			result = &copy
		}
	}
	if execution == nil || result == nil {
		return "", errors.New("a completed START command was not found")
	}
	if execution.Manifest != request.Manifest || execution.Service != request.Service {
		return "", errors.New("execution does not match manifest and service")
	}
	if request.Now.Before(result.Timestamp) {
		return "", errors.New("verification timestamp precedes the command result")
	}
	status := classifyVerification(request.Report)
	record := HistoryRecord{
		Version:       historyVersion,
		Kind:          "verification",
		Manifest:      request.Manifest,
		Service:       request.Service,
		Action:        diagnostic.ActionStart,
		Timestamp:     request.Now,
		Token:         request.Token,
		Status:        status,
		Container:     request.Report.Name,
		Configuration: request.Report.Configuration,
		Execution:     request.Report.Execution,
		Failure:       request.Report.Failure,
		Detail:        "read-only post-command observation; causality not inferred",
	}
	if err := appendLockedRecord(f, info.Size(), record); err != nil {
		return "", err
	}
	return status, nil
}

func classifyVerification(report diagnostic.Report) string {
	if report.Configuration != "matches declared fields" {
		return "configuration-changed"
	}
	if report.Execution != "running" {
		if report.Execution == "unavailable" || report.Execution == "unknown" {
			return "unknown"
		}
		return "not-running"
	}
	switch report.Health {
	case "healthy":
		return "healthy"
	case "starting":
		return "starting"
	case "unhealthy":
		return "unhealthy"
	case "not configured":
		return "running-unverified"
	default:
		return "unknown"
	}
}
