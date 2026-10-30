// Copyright 2026 Orisema contributors
// SPDX-License-Identifier: Apache-2.0

package operation

import (
	"errors"
	"fmt"
	"reflect"
	"strings"
	"time"
)

type VerificationAction string

const (
	VerificationWait             VerificationAction = "WAIT"
	VerificationComplete         VerificationAction = "COMPLETE"
	VerificationRecoveryRequired VerificationAction = "RECOVERY_REQUIRED"
	VerificationBlocked          VerificationAction = "BLOCKED"
)

type VerificationDecision struct {
	Action               VerificationAction
	TargetState          State
	TransitionAllowed    bool
	InterventionRequired bool
	ResourceEvidence     ResourceEvidence
	Reason               string
}

// PersistedVerificationDecision is the auditable result of committing a
// terminal verification decision. Observations are supplied by the caller;
// the operation package never queries or mutates a runtime.
type PersistedVerificationDecision struct {
	At                time.Time          `json:"at"`
	Action            VerificationAction `json:"action"`
	TargetState       State              `json:"targetState"`
	ObservedResources []Resource         `json:"observedResources"`
	ResourceEvidence  ResourceEvidence   `json:"resourceEvidence"`
	Reason            string             `json:"reason"`
}

// DecideAfterVerification combines a persisted health evaluation with a fresh
// supplied resource observation. It is pure: it neither persists a transition
// nor queries or mutates a runtime.
func DecideAfterVerification(op Operation, observedResources []Resource) VerificationDecision {
	blocked := func(reason string, evidence ResourceEvidence) VerificationDecision {
		return VerificationDecision{Action: VerificationBlocked, InterventionRequired: true, ResourceEvidence: evidence, Reason: reason}
	}
	if err := validate(op); err != nil {
		return blocked("operation journal is inconsistent: "+err.Error(), ResourceEvidence{})
	}
	return decideAfterValidatedVerification(op, observedResources)
}

func decideAfterValidatedVerification(op Operation, observedResources []Resource) VerificationDecision {
	blocked := func(reason string, evidence ResourceEvidence) VerificationDecision {
		return VerificationDecision{Action: VerificationBlocked, InterventionRequired: true, ResourceEvidence: evidence, Reason: reason}
	}
	if op.State != Verifying || op.Health == nil {
		return blocked("operation is not in a persisted VERIFYING state", ResourceEvidence{})
	}
	if !op.Health.Evaluation.Terminal {
		return VerificationDecision{Action: VerificationWait, Reason: "health verification has not reached a terminal evaluation"}
	}
	verificationResources := op.Resources
	if op.Execution != nil {
		verificationResources = op.Health.VerificationResources
	}
	complete, _, _ := ResourceEvidenceComplete(verificationResources)
	if !complete {
		return blocked("recorded resource ownership evidence is incomplete or ambiguous", ResourceEvidence{})
	}
	evidence := RevalidateResources(verificationResources, observedResources)
	if !evidence.Safe {
		reason := "fresh resource evidence does not match the journal"
		if len(evidence.Issues) != 0 {
			reason += ": " + evidence.Issues[0]
		}
		return blocked(reason, evidence)
	}
	evaluation := op.Health.Evaluation
	if evaluation.Status == HealthHealthy && evaluation.Succeeded {
		return VerificationDecision{
			Action:            VerificationComplete,
			TargetState:       Completed,
			TransitionAllowed: true,
			ResourceEvidence:  evidence,
			Reason:            "configured healthcheck succeeded and resource ownership was freshly revalidated",
		}
	}
	if evaluation.Succeeded {
		return blocked(fmt.Sprintf("terminal health status %s is inconsistent with success", evaluation.Status), evidence)
	}
	return VerificationDecision{
		Action:               VerificationRecoveryRequired,
		TargetState:          RecoveryRequired,
		TransitionAllowed:    true,
		InterventionRequired: true,
		ResourceEvidence:     evidence,
		Reason:               fmt.Sprintf("terminal health evaluation %s did not establish application health", evaluation.Status),
	}
}

// CommitVerificationDecision reloads the operation while holding its lock,
// revalidates caller-supplied runtime evidence and atomically persists only an
// eligible terminal decision. WAIT and BLOCKED decisions leave no journal
// change behind.
func (s *Store) CommitVerificationDecision(id string, observedResources []Resource, at time.Time) (Operation, error) {
	var result Operation
	err := s.withLock("."+id+".lock", func() error {
		op, err := s.Load(id)
		if err != nil {
			return err
		}
		if op.Decision != nil {
			return errors.New("verification decision has already been committed")
		}
		decision := DecideAfterVerification(op, observedResources)
		if !decision.TransitionAllowed || (decision.Action != VerificationComplete && decision.Action != VerificationRecoveryRequired) {
			return fmt.Errorf("verification decision %s cannot be committed: %s", decision.Action, decision.Reason)
		}
		if at.IsZero() {
			at = time.Now().UTC()
		}
		at = at.UTC()
		if at.Before(op.UpdatedAt) {
			return errors.New("verification decision timestamp precedes current operation state")
		}
		observed := append([]Resource(nil), observedResources...)
		op.Decision = &PersistedVerificationDecision{
			At: at, Action: decision.Action, TargetState: decision.TargetState,
			ObservedResources: observed, ResourceEvidence: decision.ResourceEvidence, Reason: decision.Reason,
		}
		op.Events = append(op.Events, Event{At: at, From: Verifying, To: decision.TargetState, Step: "verification-decision", Detail: decision.Reason})
		op.State, op.UpdatedAt = decision.TargetState, at
		if decision.TargetState == RecoveryRequired {
			op.Failure = decision.Reason
		}
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

func validatePersistedVerificationDecision(op Operation) error {
	if op.Decision == nil {
		return nil
	}
	d := op.Decision
	if d.At.IsZero() || d.At.Before(op.CreatedAt) || d.At != op.UpdatedAt {
		return errors.New("persisted verification decision timestamp is invalid")
	}
	if d.Action != VerificationComplete && d.Action != VerificationRecoveryRequired {
		return errors.New("persisted verification decision action is not terminal")
	}
	if d.TargetState != op.State || len(op.Events) < 2 {
		return errors.New("persisted verification decision does not describe the current state")
	}
	last := op.Events[len(op.Events)-1]
	if last.At != d.At || last.From != Verifying || last.To != d.TargetState || last.Step != "verification-decision" || strings.TrimSpace(last.Detail) != d.Reason {
		return errors.New("persisted verification decision has no matching transition event")
	}
	base := op
	base.Decision = nil
	base.State = Verifying
	base.Events = append([]Event(nil), op.Events[:len(op.Events)-1]...)
	base.UpdatedAt = base.Events[len(base.Events)-1].At
	decision := decideAfterValidatedVerification(base, d.ObservedResources)
	if !decision.TransitionAllowed || decision.Action != d.Action || decision.TargetState != d.TargetState || decision.Reason != d.Reason || !reflect.DeepEqual(decision.ResourceEvidence, d.ResourceEvidence) {
		return errors.New("persisted verification decision does not match health and resource evidence")
	}
	return nil
}
