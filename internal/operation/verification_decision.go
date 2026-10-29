// Copyright 2026 Orisema contributors
// SPDX-License-Identifier: Apache-2.0

package operation

import "fmt"

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
	if op.State != Verifying || op.Health == nil {
		return blocked("operation is not in a persisted VERIFYING state", ResourceEvidence{})
	}
	if !op.Health.Evaluation.Terminal {
		return VerificationDecision{Action: VerificationWait, Reason: "health verification has not reached a terminal evaluation"}
	}
	complete, _, _ := ResourceEvidenceComplete(op.Resources)
	if !complete {
		return blocked("recorded resource ownership evidence is incomplete or ambiguous", ResourceEvidence{})
	}
	evidence := RevalidateResources(op.Resources, observedResources)
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
