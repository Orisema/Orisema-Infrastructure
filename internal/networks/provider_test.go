// Copyright 2026 Orisema contributors
// SPDX-License-Identifier: Apache-2.0

package networks

import (
	"reflect"
	"testing"
)

func TestMetadataContractContainsIdentityFieldsOnly(t *testing.T) {
	typeOfMetadata := reflect.TypeOf(Metadata{})
	want := []string{"Provider", "Locator", "Identity", "Driver"}
	if typeOfMetadata.NumField() != len(want) {
		t.Fatalf("Metadata has %d fields; want %d identity fields", typeOfMetadata.NumField(), len(want))
	}
	for i, name := range want {
		if typeOfMetadata.Field(i).Name != name {
			t.Fatalf("Metadata field %d = %q; want %q", i, typeOfMetadata.Field(i).Name, name)
		}
	}
	for _, forbidden := range []string{"Subnet", "Gateway", "IPAM", "Create", "Remove", "Connect", "Disconnect"} {
		if _, ok := typeOfMetadata.FieldByName(forbidden); ok {
			t.Fatalf("Metadata unexpectedly exposes %s", forbidden)
		}
	}
}
