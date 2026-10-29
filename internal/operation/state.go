// Copyright 2026 Orisema contributors
// SPDX-License-Identifier: Apache-2.0

// Package operation models durable, multi-step operations. It deliberately
// contains no runtime mutation code.
package operation

import "fmt"

type State string

const (
	Pending          State = "PENDING"
	Preparing        State = "PREPARING"
	Applying         State = "APPLYING"
	Verifying        State = "VERIFYING"
	Completed        State = "COMPLETED"
	Failed           State = "FAILED"
	Interrupted      State = "INTERRUPTED"
	RecoveryRequired State = "RECOVERY_REQUIRED"
	RollingBack      State = "ROLLING_BACK"
	RolledBack       State = "ROLLED_BACK"
)

var transitions = map[State]map[State]bool{
	Pending:          {Preparing: true, Failed: true},
	Preparing:        {Applying: true, Failed: true, Interrupted: true},
	Applying:         {Verifying: true, Failed: true, Interrupted: true, RecoveryRequired: true},
	Verifying:        {Completed: true, Failed: true, Interrupted: true, RecoveryRequired: true},
	Failed:           {RecoveryRequired: true},
	Interrupted:      {RecoveryRequired: true},
	RecoveryRequired: {RollingBack: true},
	RollingBack:      {RolledBack: true, Failed: true, Interrupted: true, RecoveryRequired: true},
}

func ValidState(state State) bool {
	_, ok := transitions[state]
	return ok || state == Completed || state == RolledBack
}

func ValidateTransition(from, to State) error {
	if !ValidState(from) || !ValidState(to) {
		return fmt.Errorf("unsupported operation state transition %q -> %q", from, to)
	}
	if !transitions[from][to] {
		return fmt.Errorf("invalid operation state transition %q -> %q", from, to)
	}
	return nil
}

func (state State) Terminal() bool { return state == Completed || state == RolledBack }
