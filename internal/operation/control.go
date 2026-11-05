// Copyright 2026 Orisema contributors
// SPDX-License-Identifier: Apache-2.0

package operation

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"strings"
)

const ControlContract = "local-control-v1"

type ControlIntent struct {
	Contract        string `json:"contract"`
	Action          string `json:"action"`
	ResourceName    string `json:"resourceName"`
	ResourceID      string `json:"resourceId"`
	InitialRunning  bool   `json:"initialRunning"`
	ExpectedRunning bool   `json:"expectedRunning"`
	PlanFingerprint string `json:"planFingerprint"`
}

func ControlPlanFingerprint(application, service, action, resourceName, resourceID string, initial, expected bool) string {
	value := strings.Join([]string{ControlContract, application, service, action, resourceName, resourceID}, "\x00")
	if initial {
		value += "\x001"
	} else {
		value += "\x000"
	}
	if expected {
		value += "\x001"
	} else {
		value += "\x000"
	}
	sum := sha256.Sum256([]byte(value))
	return hex.EncodeToString(sum[:])
}

func validateControlIntent(op Operation) error {
	if op.Control == nil {
		return nil
	}
	intent := op.Control
	if op.Type != intent.Action || (intent.Action != "START" && intent.Action != "STOP") || len(op.Services) != 1 {
		return errors.New("control operation type, action or service is invalid")
	}
	if intent.Contract != ControlContract || intent.ResourceName == "" || intent.ResourceID == "" || intent.InitialRunning == intent.ExpectedRunning {
		return errors.New("control operation intent is incomplete")
	}
	want := ControlPlanFingerprint(op.Application, op.Services[0], intent.Action, intent.ResourceName, intent.ResourceID, intent.InitialRunning, intent.ExpectedRunning)
	if intent.PlanFingerprint != want {
		return errors.New("control operation plan fingerprint mismatch")
	}
	if len(op.Resources) != 1 || op.Resources[0].Kind != "container" || op.Resources[0].Name != intent.ResourceName || op.Resources[0].ID != intent.ResourceID || op.Resources[0].Ownership != OwnershipManaged {
		return errors.New("control operation resource evidence does not match intent")
	}
	return nil
}
