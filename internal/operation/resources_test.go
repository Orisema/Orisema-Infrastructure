// Copyright 2026 Orisema contributors
// SPDX-License-Identifier: Apache-2.0

package operation

import (
	"strings"
	"testing"
)

func resourceFixture() []Resource {
	return []Resource{
		{Kind: "container", Name: "orisema-demo-web", ID: "managed-id", Ownership: OwnershipManaged, Application: "demo", Service: "web"},
		{Kind: "volume", Name: "customer-data", ID: "external-id", Ownership: OwnershipExternal},
	}
}

func TestRevalidateResourcesAcceptsExactManagedAndExternalEvidence(t *testing.T) {
	resources := resourceFixture()
	result := RevalidateResources(resources, append([]Resource(nil), resources...))
	if !result.Safe || result.Managed != 1 || result.External != 1 || len(result.Issues) != 0 {
		t.Fatalf("unexpected evidence: %#v", result)
	}
	complete, managed, external := ResourceEvidenceComplete(resources)
	if !complete || managed != 1 || external != 1 {
		t.Fatalf("complete=%t managed=%d external=%d", complete, managed, external)
	}
}

func TestRevalidateResourcesFailsClosed(t *testing.T) {
	tests := []struct {
		name     string
		mutate   func([]Resource) []Resource
		contains string
	}{
		{"missing", func(r []Resource) []Resource { return r[:1] }, "missing"},
		{"identity", func(r []Resource) []Resource { r[0].ID = "other"; return r }, "identity"},
		{"ownership", func(r []Resource) []Resource { r[0].Ownership = OwnershipUnknown; return r }, "ownership changed"},
		{"scope", func(r []Resource) []Resource { r[0].Service = "other"; return r }, "scope changed"},
		{"duplicate", func(r []Resource) []Resource { return append(r, r[0]) }, "duplicate"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			expected := resourceFixture()
			observed := tc.mutate(append([]Resource(nil), resourceFixture()...))
			result := RevalidateResources(expected, observed)
			if result.Safe || !strings.Contains(strings.Join(result.Issues, "; "), tc.contains) {
				t.Fatalf("unexpected evidence: %#v", result)
			}
		})
	}
}

func TestUnknownOwnershipBlocksRollbackAssessment(t *testing.T) {
	op := Operation{State: RecoveryRequired, Previous: "old", RecoveryData: "old-id", Resources: []Resource{{Kind: "container", Name: "mystery", Ownership: OwnershipUnknown}}}
	a := Assess(op)
	if a.OwnershipComplete || a.RollbackCandidate || !a.Intervention || !strings.Contains(a.Reason, "ambiguous") {
		t.Fatalf("unsafe assessment: %#v", a)
	}
}

func TestJournalRejectsManagedResourceOutsideOperationScope(t *testing.T) {
	store, op, _ := fixture(t)
	op.Resources = []Resource{{Kind: "container", Name: "foreign", ID: "id", Ownership: OwnershipManaged, Application: "other", Service: "web"}}
	if err := store.Create(op); err == nil || !strings.Contains(err.Error(), "outside") {
		t.Fatalf("expected out-of-scope resource rejection, got %v", err)
	}
}
