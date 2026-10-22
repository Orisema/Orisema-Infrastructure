// Copyright 2026 Orisema contributors
// SPDX-License-Identifier: Apache-2.0

package planner

import (
 "context"
 "fmt"
 "strconv"
 "strings"

 "github.com/Orisema/Orisema-Infrastructure/internal/manifest"
 "github.com/Orisema/Orisema-Infrastructure/internal/runtime"
)

const (
 Noop Action = "NOOP"
 Drift Action = "DRIFT"
)

type Inspector interface {
 Inspect(context.Context, string) (runtime.Inspection, error)
}

// BuildInspected checks declared managed fields only. It never mutates Podman.
// Inspection failures are REVIEW, not evidence of safe equivalence.
func BuildInspected(ctx context.Context, m *manifest.Manifest, existing []runtime.Container, inspector Inspector) []Step {
 steps := make([]Step, 0, len(m.Services))
 for _, svc := range m.Services {
  name := "orisema-" + m.Name + "-" + svc.Name
  _, exists := runtime.Find(existing, name)
  if !exists {
   steps = append(steps, Step{Name:name, Image:svc.Image, Action:Create, Reason:"container not found"})
   continue
  }
  inspected, err := inspector.Inspect(ctx, name)
  if err != nil {
   steps = append(steps, Step{Name:name, Image:svc.Image, Action:Review, Reason:fmt.Sprintf("inspect failed: %v",err)})
   continue
  }
  labels := inspected.Config.Labels
  if labels["dev.orisema.managed"]!="true" || labels["dev.orisema.app"]!=m.Name || labels["dev.orisema.service"]!=svc.Name {
   steps = append(steps, Step{Name:name, Image:svc.Image, Action:Review, Reason:"ownership labels missing or mismatched"})
   continue
  }
  reasons := []string{}
  if inspected.Config.Image != svc.Image { reasons=append(reasons,"configured image differs") }
  expected := make(map[string]string,len(svc.Ports))
  for _, p := range svc.Ports { expected[strconv.Itoa(p.Container)+"/tcp"]=strconv.Itoa(p.Host) }
  actual := inspected.HostConfig.PortBindings
  if len(actual)!=len(expected) { reasons=append(reasons,"published port count differs") } else {
   for containerPort, hostPort := range expected {
    bindings, ok := actual[containerPort]
    if !ok || len(bindings)!=1 || bindings[0].HostPort!=hostPort || (bindings[0].HostIP!="" && bindings[0].HostIP!="0.0.0.0") {
     reasons=append(reasons,"published port binding differs");break
    }
   }
  }
  if len(reasons)>0 {
   steps=append(steps,Step{Name:name,Image:svc.Image,Action:Drift,Reason:strings.Join(reasons,", ")})
  } else {
   steps=append(steps,Step{Name:name,Image:svc.Image,Action:Noop,Reason:"managed image reference and port bindings match; runtime health and other settings not verified"})
  }
 }
 return steps
}
