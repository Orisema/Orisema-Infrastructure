// Copyright 2026 Orisema contributors
// SPDX-License-Identifier: Apache-2.0

package runtime

import "testing"

func TestFindExactName(t *testing.T) {
	items := []Container{{Names: []string{"orisema-demo-web"}, State: "running"}}
	if _, ok := Find(items, "orisema-demo"); ok {
		t.Fatal("partial match must not match")
	}
	got, ok := Find(items, "orisema-demo-web")
	if !ok || got.State != "running" {
		t.Fatal("expected exact container match")
	}
}
