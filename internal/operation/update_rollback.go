// Copyright 2026 Orisema contributors
// SPDX-License-Identifier: Apache-2.0

package operation

import (
	"errors"
	"fmt"
	"time"
)

type UpdateRollback struct {
	StartedAt        time.Time `json:"startedAt"`
	FailedName       string    `json:"failedName"`
	PendingStep      string    `json:"pendingStep,omitempty"`
	CandidateStopped bool      `json:"candidateStopped,omitempty"`
	CandidateRenamed bool      `json:"candidateRenamed,omitempty"`
	PreviousRenamed  bool      `json:"previousRenamed,omitempty"`
	PreviousStarted  bool      `json:"previousStarted,omitempty"`
	HealthObservedAt time.Time `json:"healthObservedAt,omitempty"`
	HealthStatus     string    `json:"healthStatus,omitempty"`
}

func rollbackResources(op Operation, restored bool) []Resource {
	candidateName, previousName := op.IntentResourceName(), op.Execution.BackupName
	if restored {
		candidateName, previousName = op.Rollback.FailedName, op.IntentResourceName()
	}
	return []Resource{
		{Kind: "container", Name: candidateName, ID: op.Execution.CandidateID, Ownership: OwnershipManaged, Application: op.Application, Service: op.Intent.Service},
		{Kind: "container", Name: previousName, ID: op.Execution.PreviousID, Ownership: OwnershipManaged, Application: op.Application, Service: op.Intent.Service},
	}
}

func (s *Store) BeginUpdateRollback(id, failedName string, observed []Resource, at time.Time) (Operation, error) {
	var result Operation
	err := s.withLock("."+id+".lock", func() error {
		op, err := s.Load(id)
		if err != nil {
			return err
		}
		if op.State != RecoveryRequired || op.Decision == nil || op.Decision.Action != VerificationRecoveryRequired || op.Execution == nil || op.Rollback != nil {
			return errors.New("update operation is not eligible for rollback")
		}
		if failedName == "" || !safeID.MatchString(failedName) {
			return errors.New("rollback failed-candidate name is invalid")
		}
		if evidence := RevalidateResources(rollbackResources(op, false), observed); !evidence.Safe {
			return fmt.Errorf("rollback resource evidence changed: %s", firstResourceIssue(evidence))
		}
		if at.IsZero() {
			at = time.Now().UTC()
		}
		at = at.UTC()
		if at.Before(op.UpdatedAt) {
			return errors.New("rollback timestamp precedes the operation")
		}
		op.Rollback = &UpdateRollback{StartedAt: at, FailedName: failedName, PendingStep: "stop-candidate"}
		op.Events = append(op.Events, Event{At: at, From: RecoveryRequired, To: RollingBack, Step: "rollback-started", Detail: "fresh evidence persisted before mutation"})
		op.State, op.UpdatedAt = RollingBack, at
		if err := validate(op); err != nil {
			return err
		}
		path, _ := s.path(id)
		if err := s.atomicWrite(path, op); err != nil {
			return err
		}
		result = op
		return nil
	})
	return result, err
}

func (s *Store) PrepareUpdateRollbackStep(id, step string, at time.Time) (Operation, error) {
	var result Operation
	err := s.withLock("."+id+".lock", func() error {
		op, err := s.Load(id)
		if err != nil {
			return err
		}
		if op.State != RollingBack || op.Rollback == nil || op.Rollback.PendingStep != "" {
			return errors.New("rollback cannot prepare another step")
		}
		expected := ""
		switch {
		case op.Rollback.CandidateStopped && !op.Rollback.CandidateRenamed:
			expected = "rename-candidate"
		case op.Rollback.CandidateRenamed && !op.Rollback.PreviousRenamed:
			expected = "rename-previous"
		case op.Rollback.PreviousRenamed && !op.Rollback.PreviousStarted:
			expected = "start-previous"
		}
		if step != expected || expected == "" {
			return errors.New("rollback step preparation is out of order")
		}
		if at.IsZero() {
			at = time.Now().UTC()
		}
		at = at.UTC()
		if at.Before(op.UpdatedAt) {
			return errors.New("rollback step timestamp regressed")
		}
		op.Rollback.PendingStep = step
		op.Events = append(op.Events, Event{At: at, From: RollingBack, To: RollingBack, Step: step + "-prepared", Detail: "persisted before runtime mutation"})
		op.UpdatedAt = at
		if err := validate(op); err != nil {
			return err
		}
		path, _ := s.path(id)
		if err := s.atomicWrite(path, op); err != nil {
			return err
		}
		result = op
		return nil
	})
	return result, err
}

func (s *Store) RecordUpdateRollbackStep(id, step string, at time.Time) (Operation, error) {
	var result Operation
	err := s.withLock("."+id+".lock", func() error {
		op, err := s.Load(id)
		if err != nil {
			return err
		}
		if op.State != RollingBack || op.Rollback == nil {
			return errors.New("rollback is not in progress")
		}
		switch step {
		case "candidate-stopped":
			if op.Rollback.PendingStep != "stop-candidate" || op.Rollback.CandidateStopped {
				return errors.New("candidate stop evidence is invalid")
			}
			op.Rollback.CandidateStopped = true
		case "candidate-renamed":
			if op.Rollback.PendingStep != "rename-candidate" || !op.Rollback.CandidateStopped || op.Rollback.CandidateRenamed {
				return errors.New("candidate rename evidence is invalid")
			}
			op.Rollback.CandidateRenamed = true
		case "previous-renamed":
			if op.Rollback.PendingStep != "rename-previous" || !op.Rollback.CandidateRenamed || op.Rollback.PreviousRenamed {
				return errors.New("previous rename evidence is invalid")
			}
			op.Rollback.PreviousRenamed = true
		case "previous-started":
			if op.Rollback.PendingStep != "start-previous" || !op.Rollback.PreviousRenamed || op.Rollback.PreviousStarted {
				return errors.New("previous start evidence is invalid")
			}
			op.Rollback.PreviousStarted = true
		default:
			return errors.New("unsupported rollback step")
		}
		if at.IsZero() {
			at = time.Now().UTC()
		}
		at = at.UTC()
		if at.Before(op.UpdatedAt) {
			return errors.New("rollback step timestamp regressed")
		}
		op.Rollback.PendingStep = ""
		op.Events = append(op.Events, Event{At: at, From: RollingBack, To: RollingBack, Step: step})
		op.UpdatedAt = at
		if err := validate(op); err != nil {
			return err
		}
		path, _ := s.path(id)
		if err := s.atomicWrite(path, op); err != nil {
			return err
		}
		result = op
		return nil
	})
	return result, err
}

func (s *Store) RecordUpdateRollbackHealth(id string, observed []Resource, status string, at time.Time) (Operation, error) {
	var result Operation
	err := s.withLock("."+id+".lock", func() error {
		op, err := s.Load(id)
		if err != nil {
			return err
		}
		if op.State != RollingBack || op.Rollback == nil || !op.Rollback.PreviousStarted || op.Rollback.PendingStep != "" {
			return errors.New("rollback is not ready for health verification")
		}
		if evidence := RevalidateResources(rollbackResources(op, true), observed); !evidence.Safe {
			return fmt.Errorf("rollback verification evidence changed: %s", firstResourceIssue(evidence))
		}
		if status != "healthy" && status != "starting" && status != "unhealthy" && status != "unknown" && status != "not-running" && status != "no-healthcheck" {
			return errors.New("unsupported rollback health status")
		}
		if at.IsZero() {
			at = time.Now().UTC()
		}
		at = at.UTC()
		if at.Before(op.UpdatedAt) {
			return errors.New("rollback health timestamp regressed")
		}
		op.Rollback.HealthObservedAt, op.Rollback.HealthStatus = at, status
		to := RollingBack
		if status == "healthy" {
			to = RolledBack
		}
		op.Events = append(op.Events, Event{At: at, From: RollingBack, To: to, Step: "rollback-health", Detail: status})
		op.State, op.UpdatedAt = to, at
		if err := validate(op); err != nil {
			return err
		}
		path, _ := s.path(id)
		if err := s.atomicWrite(path, op); err != nil {
			return err
		}
		result = op
		return nil
	})
	return result, err
}

func validateUpdateRollback(op Operation) error {
	if op.Rollback == nil {
		return nil
	}
	r := op.Rollback
	if op.Execution == nil || op.Decision == nil || r.StartedAt.Before(op.Decision.At) || r.FailedName == "" || !safeID.MatchString(r.FailedName) {
		return errors.New("persisted rollback boundary is invalid")
	}
	if r.CandidateRenamed && !r.CandidateStopped || r.PreviousRenamed && !r.CandidateRenamed || r.PreviousStarted && !r.PreviousRenamed {
		return errors.New("persisted rollback steps are out of order")
	}
	validPending := map[string]bool{"": true, "stop-candidate": true, "rename-candidate": true, "rename-previous": true, "start-previous": true}
	if !validPending[r.PendingStep] {
		return errors.New("persisted rollback pending step is invalid")
	}
	expectedPending := ""
	switch {
	case !r.CandidateStopped:
		expectedPending = "stop-candidate"
	case !r.CandidateRenamed:
		expectedPending = "rename-candidate"
	case !r.PreviousRenamed:
		expectedPending = "rename-previous"
	case !r.PreviousStarted:
		expectedPending = "start-previous"
	}
	if r.PendingStep != "" && r.PendingStep != expectedPending {
		return errors.New("persisted rollback boundary is inconsistent")
	}
	if op.State != RollingBack && op.State != RolledBack {
		return errors.New("persisted rollback has an invalid operation state")
	}
	if !r.HealthObservedAt.IsZero() && (!r.PreviousStarted || r.HealthStatus == "") {
		return errors.New("persisted rollback health evidence is invalid")
	}
	if op.State == RolledBack && (!r.PreviousStarted || r.HealthStatus != "healthy" || r.HealthObservedAt.IsZero()) {
		return errors.New("rolled back operation lacks healthy restored evidence")
	}
	return nil
}
