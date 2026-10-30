// Copyright 2026 Orisema contributors
// SPDX-License-Identifier: Apache-2.0

package operation

import (
	"errors"
	"fmt"
	"reflect"
	"time"
)

type HealthVerification struct {
	StartedAt             time.Time           `json:"startedAt"`
	Policy                HealthPolicy        `json:"policy"`
	VerificationResources []Resource          `json:"verificationResources"`
	Observations          []HealthObservation `json:"observations,omitempty"`
	Evaluation            HealthEvaluation    `json:"evaluation"`
}

// StartHealthVerification atomically records intent and moves APPLYING to
// VERIFYING. It does not inspect a runtime or execute a probe.
func (s *Store) StartHealthVerification(id string, policy HealthPolicy, at time.Time) (Operation, error) {
	return s.startHealthVerification(id, policy, nil, false, at)
}

// StartUpdateHealthVerification binds post-update health evidence to the exact
// candidate and preserved previous container before entering VERIFYING.
func (s *Store) StartUpdateHealthVerification(id string, policy HealthPolicy, observed []Resource, at time.Time) (Operation, error) {
	return s.startHealthVerification(id, policy, observed, true, at)
}

func (s *Store) startHealthVerification(id string, policy HealthPolicy, observed []Resource, update bool, at time.Time) (Operation, error) {
	var result Operation
	err := s.withLock("."+id+".lock", func() error {
		op, err := s.Load(id)
		if err != nil {
			return err
		}
		if op.Health != nil {
			return errors.New("health verification has already been started")
		}
		var verificationResources []Resource
		if update {
			if op.Type != "UPDATE" || op.Execution == nil || op.Intent == nil || op.Execution.PendingStep != "" || !op.Execution.PreviousStopped || !op.Execution.PreviousRenamed || op.Execution.CandidateID == "" {
				return errors.New("update execution is not ready for health verification")
			}
			verificationResources = []Resource{
				{Kind: "container", Name: op.IntentResourceName(), ID: op.Execution.CandidateID, Ownership: OwnershipManaged, Application: op.Application, Service: op.Intent.Service},
				{Kind: "container", Name: op.Execution.BackupName, ID: op.Execution.PreviousID, Ownership: OwnershipManaged, Application: op.Application, Service: op.Intent.Service},
			}
			evidence := RevalidateResources(verificationResources, observed)
			if !evidence.Safe {
				return fmt.Errorf("update verification resource evidence changed: %s", firstResourceIssue(evidence))
			}
		}
		if err := ValidateTransition(op.State, Verifying); err != nil {
			return err
		}
		if at.IsZero() {
			at = time.Now().UTC()
		}
		at = at.UTC()
		if at.Before(op.UpdatedAt) {
			return errors.New("health verification timestamp precedes current operation state")
		}
		evaluation, err := EvaluateHealth(at, at, policy, nil)
		if err != nil {
			return err
		}
		op.Events = append(op.Events, Event{At: at, From: op.State, To: Verifying, Step: "health-verification", Detail: "verification policy persisted"})
		op.State, op.UpdatedAt = Verifying, at
		op.Health = &HealthVerification{StartedAt: at, Policy: policy, VerificationResources: verificationResources, Evaluation: evaluation}
		path, _ := s.path(id)
		if err := s.atomicWrite(path, op); err != nil {
			return err
		}
		result = op
		return nil
	})
	return result, err
}

// RecordHealthObservation durably appends one supplied observation and its
// deterministic evaluation. It does not collect the observation itself.
func (s *Store) RecordHealthObservation(id string, observation HealthObservation, now time.Time) (Operation, error) {
	return s.updateHealthVerification(id, &observation, now)
}

// RefreshHealthVerification persists a time-based reevaluation, allowing a
// timeout to be recorded without inventing an observation.
func (s *Store) RefreshHealthVerification(id string, now time.Time) (Operation, error) {
	return s.updateHealthVerification(id, nil, now)
}

func (s *Store) updateHealthVerification(id string, observation *HealthObservation, now time.Time) (Operation, error) {
	var result Operation
	err := s.withLock("."+id+".lock", func() error {
		op, err := s.Load(id)
		if err != nil {
			return err
		}
		if op.State != Verifying || op.Health == nil {
			return errors.New("operation is not in a persisted VERIFYING state")
		}
		if op.Health.Evaluation.Terminal {
			return errors.New("health verification already has a terminal evaluation")
		}
		if now.IsZero() {
			now = time.Now().UTC()
		}
		now = now.UTC()
		if now.Before(op.UpdatedAt) {
			return errors.New("health evaluation timestamp precedes current operation state")
		}
		observations := append([]HealthObservation(nil), op.Health.Observations...)
		if observation != nil {
			observation.At = observation.At.UTC()
			observations = append(observations, *observation)
		}
		evaluation, err := EvaluateHealth(op.Health.StartedAt, now, op.Health.Policy, observations)
		if err != nil {
			return err
		}
		op.Health.Observations = observations
		op.Health.Evaluation = evaluation
		op.UpdatedAt = now
		detail := fmt.Sprintf("health evaluation %s", evaluation.Status)
		op.Events = append(op.Events, Event{At: now, From: Verifying, To: Verifying, Step: "health-observation", Detail: detail})
		path, _ := s.path(id)
		if err := s.atomicWrite(path, op); err != nil {
			return err
		}
		result = op
		return nil
	})
	return result, err
}

func validateHealthVerification(op Operation) error {
	if op.Health == nil {
		return nil
	}
	switch op.State {
	case Verifying, Completed, Failed, Interrupted, RecoveryRequired, RollingBack, RolledBack:
	default:
		return errors.New("health verification is present before the operation reached VERIFYING")
	}
	reachedVerifying := false
	for _, event := range op.Events {
		if event.To == Verifying {
			reachedVerifying = true
			break
		}
	}
	if !reachedVerifying {
		return errors.New("health verification has no VERIFYING transition event")
	}
	if op.Health.StartedAt.IsZero() || op.Health.StartedAt.Before(op.CreatedAt) {
		return errors.New("health verification start is invalid")
	}
	if op.Execution != nil {
		complete, _, _ := ResourceEvidenceComplete(op.Health.VerificationResources)
		if !complete {
			return errors.New("health verification resources are incomplete or ambiguous")
		}
		want := []Resource{
			{Kind: "container", Name: op.IntentResourceName(), ID: op.Execution.CandidateID, Ownership: OwnershipManaged, Application: op.Application, Service: op.Intent.Service},
			{Kind: "container", Name: op.Execution.BackupName, ID: op.Execution.PreviousID, Ownership: OwnershipManaged, Application: op.Application, Service: op.Intent.Service},
		}
		if evidence := RevalidateResources(want, op.Health.VerificationResources); !evidence.Safe {
			return errors.New("persisted update verification resources do not match execution evidence")
		}
	}
	if op.Health.Evaluation.EvaluatedAt.IsZero() || op.Health.Evaluation.EvaluatedAt.Before(op.Health.StartedAt) {
		return errors.New("health verification evaluation timestamp is invalid")
	}
	recomputed, err := EvaluateHealth(op.Health.StartedAt, op.Health.Evaluation.EvaluatedAt, op.Health.Policy, op.Health.Observations)
	if err != nil {
		return fmt.Errorf("invalid persisted health verification: %w", err)
	}
	if !reflect.DeepEqual(recomputed, op.Health.Evaluation) {
		return errors.New("persisted health evaluation does not match its policy and observations")
	}
	return nil
}
