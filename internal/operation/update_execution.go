// Copyright 2026 Orisema contributors
// SPDX-License-Identifier: Apache-2.0

package operation

import (
	"errors"
	"fmt"
	"time"
)

type UpdateExecution struct {
	StartedAt          time.Time `json:"startedAt"`
	BackupName         string    `json:"backupName"`
	PreviousID         string    `json:"previousId"`
	PendingStep        string    `json:"pendingStep,omitempty"`
	PreviousStopped    bool      `json:"previousStopped,omitempty"`
	PreviousRenamed    bool      `json:"previousRenamed,omitempty"`
	CandidateID        string    `json:"candidateId,omitempty"`
	BindingsPrepared   bool      `json:"bindingsPrepared,omitempty"`
	TransientSecretIDs []string  `json:"transientSecretIds,omitempty"`
}

func (s *Store) BeginUpdateExecution(id, planFingerprint, backupName string, observed []Resource, at time.Time) (Operation, error) {
	var result Operation
	err := s.withLock("."+id+".lock", func() error {
		op, err := s.Load(id)
		if err != nil {
			return err
		}
		if op.State != Preparing || op.Intent == nil || op.Authorization == nil || op.Execution != nil {
			return errors.New("update intent is not ready for execution")
		}
		if op.Intent.Contract == updateIntentV3PublicConfig {
			return errors.New("public configuration intent is review-only and cannot be executed")
		}
		if at.IsZero() {
			at = time.Now().UTC()
		}
		at = at.UTC()
		if at.Before(op.UpdatedAt) || at.After(op.Authorization.At.Add(UpdateAuthorizationTTL)) {
			return errors.New("update execution authorization is stale")
		}
		if planFingerprint != op.Intent.PlanFingerprint || backupName == "" || !safeID.MatchString(backupName) {
			return errors.New("update execution plan or backup name is invalid")
		}
		evidence := RevalidateResources(op.Resources, observed)
		if !evidence.Safe {
			return fmt.Errorf("update execution resource evidence changed: %s", firstResourceIssue(evidence))
		}
		pendingStep := "stop-previous"
		if op.Intent.Contract == updateIntentV2 {
			pendingStep = "prepare-bindings"
		}
		op.Execution = &UpdateExecution{StartedAt: at, BackupName: backupName, PreviousID: op.Resources[0].ID, PendingStep: pendingStep}
		op.Events = append(op.Events, Event{At: at, From: Preparing, To: Applying, Step: "update-execution-started", Detail: "fresh evidence persisted before mutation"})
		op.State, op.UpdatedAt = Applying, at
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

// RecordUpdateBindingsPrepared persists metadata-only evidence after transient
// bindings are ready and before the previous container is stopped.
func (s *Store) RecordUpdateBindingsPrepared(id string, transientSecretIDs []string, at time.Time) (Operation, error) {
	var result Operation
	err := s.withLock("."+id+".lock", func() error {
		op, err := s.Load(id)
		if err != nil {
			return err
		}
		if op.State != Applying || op.Execution == nil || op.Intent == nil || op.Intent.Contract != updateIntentV2 || op.Execution.PendingStep != "prepare-bindings" || op.Execution.BindingsPrepared {
			return errors.New("update bindings are not pending preparation")
		}
		seen := map[string]bool{}
		for _, identity := range transientSecretIDs {
			if !safeID.MatchString(identity) || seen[identity] {
				return errors.New("transient secret evidence is invalid")
			}
			seen[identity] = true
		}
		if at.IsZero() {
			at = time.Now().UTC()
		}
		at = at.UTC()
		if at.Before(op.UpdatedAt) {
			return errors.New("update binding preparation timestamp regressed")
		}
		op.Execution.BindingsPrepared = true
		op.Execution.TransientSecretIDs = append([]string(nil), transientSecretIDs...)
		op.Execution.PendingStep = "stop-previous"
		op.Events = append(op.Events, Event{At: at, From: Applying, To: Applying, Step: "bindings-prepared", Detail: fmt.Sprintf("%d transient secret identities persisted", len(transientSecretIDs))})
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

func (s *Store) PrepareUpdateExecutionStep(id, step string, at time.Time) (Operation, error) {
	var result Operation
	err := s.withLock("."+id+".lock", func() error {
		op, err := s.Load(id)
		if err != nil {
			return err
		}
		if op.State != Applying || op.Execution == nil || op.Execution.PendingStep != "" {
			return errors.New("update execution cannot prepare another step")
		}
		expected := ""
		switch {
		case op.Execution.PreviousStopped && !op.Execution.PreviousRenamed:
			expected = "rename-previous"
		case op.Execution.PreviousRenamed && op.Execution.CandidateID == "":
			expected = "create-candidate"
		}
		if step != expected || expected == "" {
			return errors.New("update execution step preparation is out of order")
		}
		if at.IsZero() {
			at = time.Now().UTC()
		}
		at = at.UTC()
		if at.Before(op.UpdatedAt) {
			return errors.New("update execution step timestamp regressed")
		}
		op.Execution.PendingStep = step
		op.Events = append(op.Events, Event{At: at, From: Applying, To: Applying, Step: step + "-prepared", Detail: "persisted before runtime mutation"})
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

func firstResourceIssue(evidence ResourceEvidence) string {
	if len(evidence.Issues) == 0 {
		return "unknown mismatch"
	}
	return evidence.Issues[0]
}

func (s *Store) RecordUpdateExecutionStep(id, step, candidateID string, at time.Time) (Operation, error) {
	var result Operation
	err := s.withLock("."+id+".lock", func() error {
		op, err := s.Load(id)
		if err != nil {
			return err
		}
		if op.State != Applying || op.Execution == nil {
			return errors.New("update execution is not APPLYING")
		}
		if at.IsZero() {
			at = time.Now().UTC()
		}
		at = at.UTC()
		if at.Before(op.UpdatedAt) {
			return errors.New("update execution step timestamp regressed")
		}
		switch step {
		case "previous-stopped":
			if op.Execution.PendingStep != "stop-previous" || op.Execution.PreviousStopped {
				return errors.New("previous container stop is already recorded")
			}
			op.Execution.PreviousStopped = true
		case "previous-renamed":
			if op.Execution.PendingStep != "rename-previous" || !op.Execution.PreviousStopped || op.Execution.PreviousRenamed {
				return errors.New("previous container rename is out of order")
			}
			op.Execution.PreviousRenamed = true
		case "candidate-created":
			if op.Execution.PendingStep != "create-candidate" || !op.Execution.PreviousRenamed || op.Execution.CandidateID != "" || candidateID == "" || candidateID == op.Execution.PreviousID {
				return errors.New("candidate creation evidence is invalid or out of order")
			}
			op.Execution.CandidateID = candidateID
		default:
			return errors.New("unsupported update execution step")
		}
		op.Execution.PendingStep = ""
		op.Events = append(op.Events, Event{At: at, From: Applying, To: Applying, Step: step, Detail: candidateID})
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

func validateUpdateExecution(op Operation) error {
	if op.Execution == nil {
		return nil
	}
	e := op.Execution
	if op.Intent == nil || op.Authorization == nil || op.State == Pending || op.State == Preparing || e.StartedAt.Before(op.Authorization.At) || e.BackupName == "" || !safeID.MatchString(e.BackupName) || e.PreviousID == "" {
		return errors.New("persisted update execution boundary is invalid")
	}
	if e.PreviousRenamed && !e.PreviousStopped {
		return errors.New("persisted update execution rename is out of order")
	}
	if e.CandidateID != "" && (!e.PreviousRenamed || e.CandidateID == e.PreviousID) {
		return errors.New("persisted update candidate evidence is invalid")
	}
	if op.Intent.Contract == updateIntentV2 && !e.BindingsPrepared && e.PendingStep != "prepare-bindings" {
		return errors.New("persisted update binding preparation is incomplete")
	}
	if op.Intent.Contract != updateIntentV2 && (e.BindingsPrepared || len(e.TransientSecretIDs) != 0) {
		return errors.New("legacy update execution contains binding evidence")
	}
	validPending := e.PendingStep == "" || e.PendingStep == "prepare-bindings" || e.PendingStep == "stop-previous" || e.PendingStep == "rename-previous" || e.PendingStep == "create-candidate"
	if !validPending || (e.PendingStep == "stop-previous" && e.PreviousStopped) || (e.PendingStep == "rename-previous" && (!e.PreviousStopped || e.PreviousRenamed)) || (e.PendingStep == "create-candidate" && (!e.PreviousRenamed || e.CandidateID != "")) {
		return errors.New("persisted update execution pending step is invalid")
	}
	return nil
}
