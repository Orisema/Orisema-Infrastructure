// Copyright 2026 Orisema contributors
// SPDX-License-Identifier: Apache-2.0

package recovery

import (
	"crypto/rand"
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

const (
	defaultReservationTTL = 5 * time.Minute
	maxReservationTTL     = 15 * time.Minute
)

type PrepareRequest struct {
	HistoryPath string
	Manifest    string
	Report      diagnostic.Report
	Policy      *manifest.Recovery
	Operator    string
	Reason      string
	Now         time.Time
	TTL         time.Duration
}

type Preparation struct {
	Token     string
	ExpiresAt time.Time
	Decision  Decision
}

// Prepare atomically evaluates guardrails and reserves one future START. It
// only writes audit state; it never calls a container runtime.
func Prepare(request PrepareRequest) (Preparation, error) {
	request.Operator = strings.TrimSpace(request.Operator)
	request.Reason = strings.TrimSpace(request.Reason)
	if request.Manifest == "" || request.Operator == "" || request.Reason == "" {
		return Preparation{}, errors.New("manifest, operator and reason are required")
	}
	if len(request.Operator) > 128 || len(request.Reason) > 512 {
		return Preparation{}, errors.New("operator must be at most 128 bytes and reason at most 512 bytes")
	}
	if request.Report.Service == "" || request.Report.Action != diagnostic.ActionStart {
		return Preparation{}, errors.New("only a revalidated START recommendation can be prepared")
	}
	if request.Now.IsZero() {
		request.Now = time.Now().UTC()
	} else {
		request.Now = request.Now.UTC()
	}
	if request.TTL == 0 {
		request.TTL = defaultReservationTTL
	}
	if request.TTL < time.Minute || request.TTL > maxReservationTTL {
		return Preparation{}, fmt.Errorf("reservation TTL must be between 1m and %s", maxReservationTTL)
	}

	pathInfo, err := os.Lstat(request.HistoryPath)
	if err != nil {
		return Preparation{}, fmt.Errorf("inspect recovery history path: %w", err)
	}
	if pathInfo.Mode().Perm()&0o077 != 0 {
		return Preparation{}, errors.New("recovery history permissions must not grant group or other access")
	}
	f, err := openHistory(request.HistoryPath, os.O_RDWR|os.O_APPEND, 0)
	if err != nil {
		return Preparation{}, err
	}
	defer f.Close()
	if err := syscall.Flock(int(f.Fd()), syscall.LOCK_EX); err != nil {
		return Preparation{}, fmt.Errorf("lock recovery history: %w", err)
	}
	defer syscall.Flock(int(f.Fd()), syscall.LOCK_UN) //nolint:errcheck
	info, err := f.Stat()
	if err != nil {
		return Preparation{}, fmt.Errorf("inspect recovery history: %w", err)
	}
	if info.Size() > maxHistorySize {
		return Preparation{}, fmt.Errorf("recovery history exceeds %d bytes", maxHistorySize)
	}
	records, err := readHistoryRecords(f)
	if err != nil {
		return Preparation{}, err
	}

	attempts := make([]Attempt, 0, len(records))
	consumed := make(map[string]bool)
	for _, record := range records {
		if record.Token != "" && (record.Kind == "execution" || record.Kind == "result") {
			consumed[record.Token] = true
		}
	}
	for _, record := range records {
		if record.Manifest != request.Manifest || record.Service != request.Report.Service || record.Action != request.Report.Action {
			continue
		}
		switch record.Kind {
		case "", "attempt":
			attempts = append(attempts, Attempt{Service: record.Service, Action: record.Action, Timestamp: record.Timestamp, Succeeded: record.Succeeded})
		case "execution":
			if record.Status == "started" {
				attempts = append(attempts, Attempt{Service: record.Service, Action: record.Action, Timestamp: record.Timestamp})
			}
		case "reservation":
			if consumed[record.Token] {
				continue
			}
			if record.Timestamp.After(request.Now) {
				return Preparation{}, errors.New("recovery history contains a future reservation")
			}
			if record.ExpiresAt.After(request.Now) {
				return Preparation{}, fmt.Errorf("an active reservation already exists until %s", record.ExpiresAt.Format(time.RFC3339))
			}
		}
	}
	decision := Evaluate(request.Now, request.Report.Service, request.Report.Action, request.Policy, attempts)
	if !decision.Allowed {
		return Preparation{Decision: decision}, fmt.Errorf("recovery guardrails blocked preparation: %s", decision.Reason)
	}
	token, err := newReservationToken()
	if err != nil {
		return Preparation{}, err
	}
	expiresAt := request.Now.Add(request.TTL)
	record := HistoryRecord{
		Version:       historyVersion,
		Kind:          "reservation",
		Manifest:      request.Manifest,
		Service:       request.Report.Service,
		Action:        request.Report.Action,
		Timestamp:     request.Now,
		Token:         token,
		ExpiresAt:     expiresAt,
		Operator:      request.Operator,
		Reason:        request.Reason,
		Container:     request.Report.Name,
		Configuration: request.Report.Configuration,
		Execution:     request.Report.Execution,
		Failure:       request.Report.Failure,
		Policy:        recoveryPolicySnapshot(request.Policy),
	}
	if err := validateRecord(record); err != nil {
		return Preparation{}, err
	}
	encoded, err := json.Marshal(record)
	if err != nil {
		return Preparation{}, fmt.Errorf("encode recovery reservation: %w", err)
	}
	encoded = append(encoded, '\n')
	if info.Size()+int64(len(encoded)) > maxHistorySize {
		return Preparation{}, fmt.Errorf("recovery history would exceed %d bytes", maxHistorySize)
	}
	if _, err := f.Write(encoded); err != nil {
		return Preparation{}, fmt.Errorf("append recovery reservation: %w", err)
	}
	if err := f.Sync(); err != nil {
		return Preparation{}, fmt.Errorf("sync recovery reservation: %w", err)
	}
	return Preparation{Token: token, ExpiresAt: expiresAt, Decision: decision}, nil
}

func recoveryPolicySnapshot(policy *manifest.Recovery) string {
	if policy == nil {
		return ""
	}
	return fmt.Sprintf("maxAttempts=%d;minInterval=%s;cooldown=%s", policy.MaxAttempts, policy.MinInterval, policy.Cooldown)
}

func newReservationToken() (string, error) {
	raw := make([]byte, 16)
	if _, err := rand.Read(raw); err != nil {
		return "", fmt.Errorf("generate recovery reservation token: %w", err)
	}
	return hex.EncodeToString(raw), nil
}
