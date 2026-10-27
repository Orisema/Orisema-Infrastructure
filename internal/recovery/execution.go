// Copyright 2026 Orisema contributors
// SPDX-License-Identifier: Apache-2.0

package recovery

import (
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"strings"
	"syscall"
	"time"

	"github.com/Orisema/Orisema-Infrastructure/internal/diagnostic"
	"github.com/Orisema/Orisema-Infrastructure/internal/manifest"
)

type BeginRequest struct {
	HistoryPath string
	Manifest    string
	Token       string
	Report      diagnostic.Report
	Policy      *manifest.Recovery
	Now         time.Time
}

// BeginExecution consumes a live reservation and durably records an attempt
// before the caller mutates the runtime. A begun token cannot be reused.
func BeginExecution(request BeginRequest) error {
	if !validReservationToken(request.Token) {
		return errors.New("reservation token must be 32 hexadecimal characters")
	}
	if request.Manifest == "" || request.Report.Service == "" || request.Report.Action != diagnostic.ActionStart {
		return errors.New("a revalidated START report is required")
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

	var reservation *HistoryRecord
	attempts := make([]Attempt, 0, len(records))
	for i := range records {
		record := records[i]
		if record.Token == request.Token && (record.Kind == "execution" || record.Kind == "result") {
			return errors.New("reservation token has already been consumed")
		}
		if record.Kind == "reservation" && record.Token == request.Token {
			copy := record
			reservation = &copy
		}
		if record.Manifest == request.Manifest && record.Service == request.Report.Service && record.Action == diagnostic.ActionStart {
			if record.Kind == "" || record.Kind == "attempt" || (record.Kind == "execution" && record.Status == "started") {
				attempts = append(attempts, Attempt{Service: record.Service, Action: record.Action, Timestamp: record.Timestamp, Succeeded: record.Succeeded})
			}
		}
	}
	if reservation == nil {
		return errors.New("reservation token was not found")
	}
	if reservation.Manifest != request.Manifest || reservation.Service != request.Report.Service || reservation.Action != diagnostic.ActionStart {
		return errors.New("reservation does not match manifest, service and START action")
	}
	if !reservation.ExpiresAt.After(request.Now) {
		return errors.New("reservation token has expired")
	}
	currentPolicy := recoveryPolicySnapshot(request.Policy)
	if reservation.Policy == "" || reservation.Policy != currentPolicy {
		return errors.New("recovery policy changed or was not captured by the reservation")
	}
	if reservation.Container != request.Report.Name || reservation.Configuration != request.Report.Configuration || reservation.Execution != request.Report.Execution || reservation.Failure != request.Report.Failure {
		return errors.New("runtime observation changed after preparation")
	}
	decision := Evaluate(request.Now, request.Report.Service, diagnostic.ActionStart, request.Policy, attempts)
	if !decision.Allowed {
		return fmt.Errorf("recovery guardrails blocked execution: %s", decision.Reason)
	}
	record := HistoryRecord{
		Version:       historyVersion,
		Kind:          "execution",
		Manifest:      request.Manifest,
		Service:       request.Report.Service,
		Action:        diagnostic.ActionStart,
		Timestamp:     request.Now,
		Token:         request.Token,
		Operator:      reservation.Operator,
		Reason:        reservation.Reason,
		Container:     request.Report.Name,
		Configuration: request.Report.Configuration,
		Execution:     request.Report.Execution,
		Failure:       request.Report.Failure,
		Policy:        currentPolicy,
		Status:        "started",
	}
	return appendLockedRecord(f, info.Size(), record)
}

// CompleteExecution appends the observed result of a begun attempt. It cannot
// create an attempt or complete the same token twice.
func CompleteExecution(path, token string, succeeded bool, detail string, now time.Time) error {
	if !validReservationToken(token) {
		return errors.New("reservation token must be 32 hexadecimal characters")
	}
	detail = strings.TrimSpace(detail)
	if len(detail) > 512 {
		return errors.New("execution result detail must be at most 512 bytes")
	}
	if now.IsZero() {
		now = time.Now().UTC()
	} else {
		now = now.UTC()
	}
	f, records, info, err := openLockedHistory(path)
	if err != nil {
		return err
	}
	defer f.Close()
	defer syscall.Flock(int(f.Fd()), syscall.LOCK_UN) //nolint:errcheck
	var execution *HistoryRecord
	for i := range records {
		record := records[i]
		if record.Token != token {
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
	if now.Before(execution.Timestamp) {
		return errors.New("execution result timestamp precedes the begun attempt")
	}
	status := "start-command-failed"
	if succeeded {
		status = "start-command-succeeded"
	}
	record := HistoryRecord{
		Version:   historyVersion,
		Kind:      "result",
		Manifest:  execution.Manifest,
		Service:   execution.Service,
		Action:    execution.Action,
		Timestamp: now,
		Token:     token,
		Status:    status,
		Detail:    detail,
	}
	return appendLockedRecord(f, info.Size(), record)
}

func openLockedHistory(path string) (*os.File, []HistoryRecord, os.FileInfo, error) {
	pathInfo, err := os.Lstat(path)
	if err != nil {
		return nil, nil, nil, fmt.Errorf("inspect recovery history path: %w", err)
	}
	if pathInfo.Mode().Perm()&0o077 != 0 {
		return nil, nil, nil, errors.New("recovery history permissions must not grant group or other access")
	}
	f, err := openHistory(path, os.O_RDWR|os.O_APPEND, 0)
	if err != nil {
		return nil, nil, nil, err
	}
	if err := syscall.Flock(int(f.Fd()), syscall.LOCK_EX); err != nil {
		f.Close()
		return nil, nil, nil, fmt.Errorf("lock recovery history: %w", err)
	}
	info, err := f.Stat()
	if err != nil {
		syscall.Flock(int(f.Fd()), syscall.LOCK_UN) //nolint:errcheck
		f.Close()
		return nil, nil, nil, fmt.Errorf("inspect recovery history: %w", err)
	}
	if info.Size() > maxHistorySize {
		syscall.Flock(int(f.Fd()), syscall.LOCK_UN) //nolint:errcheck
		f.Close()
		return nil, nil, nil, fmt.Errorf("recovery history exceeds %d bytes", maxHistorySize)
	}
	records, err := readHistoryRecords(f)
	if err != nil {
		syscall.Flock(int(f.Fd()), syscall.LOCK_UN) //nolint:errcheck
		f.Close()
		return nil, nil, nil, err
	}
	return f, records, info, nil
}

func appendLockedRecord(f *os.File, currentSize int64, record HistoryRecord) error {
	if err := validateRecord(record); err != nil {
		return err
	}
	encoded, err := json.Marshal(record)
	if err != nil {
		return fmt.Errorf("encode recovery history: %w", err)
	}
	encoded = append(encoded, '\n')
	if currentSize+int64(len(encoded)) > maxHistorySize {
		return fmt.Errorf("recovery history would exceed %d bytes", maxHistorySize)
	}
	if _, err := f.Write(encoded); err != nil {
		return fmt.Errorf("append recovery history: %w", err)
	}
	if err := f.Sync(); err != nil {
		return fmt.Errorf("sync recovery history: %w", err)
	}
	return nil
}

func validReservationToken(token string) bool {
	if len(token) != 32 {
		return false
	}
	_, err := hex.DecodeString(token)
	return err == nil
}
