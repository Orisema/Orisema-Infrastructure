// Copyright 2026 Orisema contributors
// SPDX-License-Identifier: Apache-2.0

package planner

import (
 "testing"
 "github.com/Orisema/Orisema-Infrastructure/internal/manifest"
 "github.com/Orisema/Orisema-Infrastructure/internal/runtime"
)

func TestBuildConservative(t *testing.T) {
 m := &manifest.Manifest{Name:"demo", Services:[]manifest.Service{{Name:"web",Image:"nginx:1.27"},{Name:"db",Image:"postgres:17"}}}
 existing := []runtime.Container{{Names:[]string{"orisema-demo-web"},State:"running",Image:"nginx:1.27"}}
 steps := Build(m, existing)
 if len(steps)!=2 || steps[0].Action!=Review || steps[1].Action!=Create {
  t.Fatalf("unexpected steps: %#v",steps)
 }
}
